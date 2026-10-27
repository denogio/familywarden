package openwrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// readOverride parses the existing volatile format, without deciding eligibility
// or expiry. Other profiles and unrelated lines are preserved when writing.
func readOverride(raw []byte, profile string) (platform.NetworkOverride, error) {
	out := platform.NetworkOverride{Mode: "none"}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, "=")
		if key != profile+".override" && key != profile+".override_expires" {
			continue
		}
		if !found {
			return out, errors.New("invalid network state row")
		}
		if seen[key] {
			return out, errors.New("duplicate network state")
		}
		seen[key] = true
		if key == profile+".override" {
			out.Mode = value
		} else if value != "" {
			var err error
			out.Expires, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return out, errors.New("invalid network deadline")
			}
		}
	}
	return out, nil
}

// readAllOverrides parses every profile's volatile rows without deciding
// eligibility or expiry. Unrelated lines are ignored; duplicate or malformed
// rows fail closed. A profile with no mode row reads as "none".
func readAllOverrides(raw []byte) (map[string]platform.NetworkOverride, error) {
	out := map[string]platform.NetworkOverride{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, "=")
		suffix := ""
		switch {
		case strings.HasSuffix(key, ".override"):
			suffix = ".override"
		case strings.HasSuffix(key, ".override_expires"):
			suffix = ".override_expires"
		default:
			continue
		}
		if !found {
			return nil, errors.New("invalid network state row")
		}
		profileID := key[:len(key)-len(suffix)]
		if profileID == "" {
			return nil, errors.New("invalid network state row")
		}
		rowKey := profileID + suffix
		if seen[rowKey] {
			return nil, errors.New("duplicate network state")
		}
		seen[rowKey] = true
		row := out[profileID]
		if suffix == ".override" {
			row.Mode = value
		} else if value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, errors.New("invalid network deadline")
			}
			row.Expires = n
		}
		out[profileID] = row
	}
	for id, row := range out {
		if row.Mode == "" {
			row.Mode = "none"
		}
		if row.Mode != "none" && row.Mode != "allow" && row.Mode != "block" {
			return nil, fmt.Errorf("invalid network state for %s", id)
		}
		out[id] = row
	}
	return out, nil
}

func renderOverride(raw []byte, profile string, next platform.NetworkOverride) []byte {
	var out strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		key, _, _ := strings.Cut(line, "=")
		if line != "" && key != profile+".override" && key != profile+".override_expires" {
			out.WriteString(line + "\n")
		}
	}
	fmt.Fprintf(&out, "%s.override=%s\n%s.override_expires=", profile, next.Mode, profile)
	if next.Expires != 0 {
		fmt.Fprint(&out, next.Expires)
	}
	out.WriteByte('\n')
	return []byte(out.String())
}

func (a Adapter) networkStateFile() string {
	if a.StateFile != "" {
		return a.StateFile
	}
	return "/tmp/familywarden.state"
}

func (a Adapter) mastSeenFile() string {
	if a.MastSeenFile != "" {
		return a.MastSeenFile
	}
	return "/tmp/familywarden.mast"
}

func readNetworkFile(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	absent := os.IsNotExist(err)
	if err != nil && !absent {
		return nil, false, err
	}
	if len(raw) > maxOutput {
		return nil, false, fmt.Errorf("network state too large")
	}
	return raw, absent, nil
}

func restoreNetworkFile(path string, raw []byte, absent bool) error {
	if absent {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return replaceFile(path, raw)
}

// mastSeen reports the router's last signed-payload fetch epoch. An absent or
// empty journal reads as zero; malformed content fails closed.
func mastSeen(path string) (int64, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0, nil
	}
	if len(fields) != 2 {
		return 0, errors.New("invalid mast sync journal")
	}
	epoch, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || epoch < 0 {
		return 0, errors.New("invalid mast sync journal")
	}
	return epoch, nil
}

// NetworkSnapshot takes the shared state lock, reads the volatile overrides and
// the mast journal, and serves one consistent facts snapshot to the callback.
// Platform writers hold the exclusive lock; the callback must not call the
// adapter.
func (a Adapter) NetworkSnapshot(fn func(platform.Context, map[string]platform.NetworkOverride) error) error {
	path := a.LockFile
	if path == "" {
		path = "/var/lock/familywarden.lock"
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := acquireLock(ctx, lock, true); err != nil {
		return fmt.Errorf("network snapshot: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	raw, _, err := readNetworkFile(a.networkStateFile())
	if err != nil {
		return err
	}
	state, err := readAllOverrides(raw)
	if err != nil {
		return err
	}
	synced, err := mastSeen(a.mastSeenFile())
	if err != nil {
		return err
	}
	var facts platform.Context
	if err := a.runLocked(&facts, lock, "core-network-facts"); err != nil {
		return err
	}
	facts.MastLastSync = synced
	return fn(facts, state)
}

// projectPactPermit is called with the configuration transaction's locks already
// held. Core decides revocation; this adapter only reads/writes its RAM format.
// The undo is for failed application, not later successful pact release.
func (a Adapter) projectPactPermit(c rules.Configuration, now time.Time) (func() error, error) {
	if c.Pact == nil {
		return nil, nil
	}
	path := a.networkStateFile()
	raw, absent, err := readNetworkFile(path)
	if err != nil {
		return nil, err
	}
	current, err := readOverride(raw, c.Pact.Captain)
	if err != nil {
		return nil, err
	}
	next, err := configuration.PactNetworkOverride(c, c.Pact.Captain, current, now)
	if err != nil || next == current {
		return nil, err
	}
	undo := func() error { return restoreNetworkFile(path, raw, absent) }
	if err := replaceFile(path, renderOverride(raw, c.Pact.Captain, next)); err != nil {
		if restore := undo(); restore != nil {
			return nil, fmt.Errorf("pact permit projection failed (%v); restore failed: %w", err, restore)
		}
		return nil, err
	}
	return undo, nil
}

// NetworkTransaction keeps RAM persistence, native firewall application and its
// acknowledgement under the same locks as all participating legacy writers.
func (a Adapter) NetworkTransaction(profile string, fn func(platform.Context, platform.NetworkOverride, func(platform.NetworkOverride) error) error) error {
	if profile == "" || strings.ContainsAny(profile, ".=\r\n") {
		return fmt.Errorf("invalid network profile")
	}
	return a.withStateLocks(func(state, ticket *os.File) error {
		path := a.networkStateFile()
		raw, absent, err := readNetworkFile(path)
		if err != nil {
			return err
		}
		current, err := readOverride(raw, profile)
		if err != nil {
			return err
		}
		var facts platform.Context
		if err := a.runLocked(&facts, state, "core-network-facts"); err != nil {
			return err
		}
		compile := func() error {
			var reply struct {
				Status string `json:"status"`
			}
			if err := a.runFiles(&reply, []*os.File{state, ticket}, "core-apply-network"); err != nil {
				return err
			}
			if reply.Status != "ok" {
				return fmt.Errorf("network apply not acknowledged")
			}
			return nil
		}
		apply := func(next platform.NetworkOverride) error {
			// Validate the output representation, not the core's policy decision.
			if (next.Mode != "none" && next.Mode != "allow" && next.Mode != "block") || next.Expires < 0 {
				return fmt.Errorf("invalid network intent")
			}
			err := replaceFile(path, renderOverride(raw, profile, next))
			if err == nil {
				err = compile()
			}
			if err != nil {
				restore := restoreNetworkFile(path, raw, absent)
				if restore == nil {
					restore = compile() // Restore prior enforcement, never retry the requested change.
				}
				if restore != nil {
					return fmt.Errorf("network apply failed (%v); restore failed: %w", err, restore)
				}
				return err
			}
			return nil
		}
		return fn(facts, current, apply)
	})
}
