package openwrt

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/denogio/familywarden/core/internal/platform"
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

// NetworkTransaction keeps RAM persistence, native firewall application and its
// acknowledgement under the same locks as all participating legacy writers.
func (a Adapter) NetworkTransaction(profile string, fn func(platform.Context, platform.NetworkOverride, func(platform.NetworkOverride) error) error) error {
	if profile == "" || strings.ContainsAny(profile, ".=\r\n") {
		return fmt.Errorf("invalid network profile")
	}
	return a.withStateLocks(func(state, ticket *os.File) error {
		path := a.StateFile
		if path == "" {
			path = "/tmp/familywarden.state"
		}
		raw, err := os.ReadFile(path)
		absent := os.IsNotExist(err)
		if err != nil && !absent {
			return err
		}
		if len(raw) > maxOutput {
			return fmt.Errorf("network state too large")
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
				var restore error
				if absent {
					restore = os.Remove(path)
					if os.IsNotExist(restore) {
						restore = nil
					}
				} else {
					restore = replaceFile(path, raw)
				}
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
