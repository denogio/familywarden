package openwrt

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
