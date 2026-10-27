package openwrt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
)

func networkAdapter(t *testing.T) Adapter {
	t.Helper()
	dir := t.TempDir()
	a := Adapter{Shell: filepath.Join(dir, "adapter"), LockFile: filepath.Join(dir, "state.lock"), StateFile: filepath.Join(dir, "state")}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
case "$1" in
core-network-facts) echo '{"network_format":"network-actions-v1","timezone":"UTC"}' ;;
core-apply-network)
 [ /proc/self/fd/3 -ef %[1]q ] && [ /proc/self/fd/4 -ef %[2]q ]
 flock -n -x 3 && flock -n -x 4
 n=0; [ ! -f %[3]q ] || n=$(head -1 %[3]q)
 n=$((n+1)); echo "$n" > %[3]q
 if [ -f %[4]q ]; then cp %[4]q %[5]q.$n; else echo absent > %[5]q.$n; fi
 if [ -f %[6]q ]; then rm %[6]q; echo '{"error":"injected native failure"}'; exit 1; fi
 echo '{"status":"ok"}' ;;
*) exit 1 ;;
esac
`, a.LockFile, a.LockFile+".ticket", filepath.Join(dir, "count"), a.StateFile, filepath.Join(dir, "observed"), filepath.Join(dir, "fail-once"))
	if err := os.WriteFile(a.Shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNetworkTransactionUsesInheritedLocksAndPreservesOtherState(t *testing.T) {
	a := networkAdapter(t)
	raw := "alex.override=allow\nalex.override_expires=12345\ntaylor.override=block\nother=value\n"
	if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.NetworkTransaction("alex", func(facts platform.Context, current platform.NetworkOverride, apply func(platform.NetworkOverride) error) error {
		probeStateLock(t, a.LockFile, true)
		probeStateLock(t, a.LockFile+".ticket", true)
		if facts.NetworkFormat != "network-actions-v1" || current.Mode != "allow" || current.Expires != 12345 {
			t.Fatal(facts, current)
		}
		return apply(platform.NetworkOverride{Mode: "block"})
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(a.StateFile)
	if err != nil || string(got) != "taylor.override=block\nother=value\nalex.override=block\nalex.override_expires=\n" {
		t.Fatal(string(got), err)
	}
	probeStateLock(t, a.LockFile, false)
	probeStateLock(t, a.LockFile+".ticket", false)
}

func TestNetworkTransactionRestoresRAMAndPriorEnforcementOnFailure(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(fmt.Sprint("absent=", absent), func(t *testing.T) {
			a := networkAdapter(t)
			dir := filepath.Dir(a.StateFile)
			raw := "alex.override=block\nalex.override_expires=\ntaylor.override=allow\n"
			if !absent {
				if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "fail-once"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			err := a.NetworkTransaction("alex", func(_ platform.Context, _ platform.NetworkOverride, apply func(platform.NetworkOverride) error) error {
				return apply(platform.NetworkOverride{Mode: "allow", Expires: 12345})
			})
			if err == nil {
				t.Fatal("native failure hidden", err)
			}
			got, readErr := os.ReadFile(a.StateFile)
			if absent {
				if !os.IsNotExist(readErr) {
					t.Fatal("failed apply created lasting RAM intent", string(got), readErr)
				}
			} else if readErr != nil || string(got) != raw {
				t.Fatal("previous state not restored", string(got), readErr)
			}
			count, err := os.ReadFile(filepath.Join(dir, "count"))
			if err != nil || string(count) != "2\n" {
				t.Fatal("prior enforcement was not reapplied", string(count), err)
			}
			requested, err := os.ReadFile(filepath.Join(dir, "observed.1"))
			if err != nil || !strings.Contains(string(requested), "alex.override=allow") {
				t.Fatal(string(requested), err)
			}
			restored, err := os.ReadFile(filepath.Join(dir, "observed.2"))
			want := raw
			if absent {
				want = "absent\n"
			}
			if err != nil || string(restored) != want {
				t.Fatal("rollback did not apply prior intent", string(restored), err)
			}
			probeStateLock(t, a.LockFile, false)
			probeStateLock(t, a.LockFile+".ticket", false)
		})
	}
}

func TestNetworkTransactionRefusesMalformedRAMBeforeCallback(t *testing.T) {
	for _, raw := range []string{"alex.override_expires=invalid\n", "alex.override=allow\nalex.override_expires\n", "alex.override=allow\nalex.override=block\n"} {
		a := networkAdapter(t)
		if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		called := false
		err := a.NetworkTransaction("alex", func(platform.Context, platform.NetworkOverride, func(platform.NetworkOverride) error) error {
			called = true
			return nil
		})
		if err == nil || called {
			t.Fatal("corrupt state reached product mutation", called, err)
		}
		probeStateLock(t, a.LockFile, false)
		probeStateLock(t, a.LockFile+".ticket", false)
	}
}
