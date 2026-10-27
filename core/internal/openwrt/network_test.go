package openwrt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func networkAdapter(t *testing.T) Adapter {
	t.Helper()
	dir := t.TempDir()
	a := Adapter{Shell: filepath.Join(dir, "adapter"), LockFile: filepath.Join(dir, "state.lock"), StateFile: filepath.Join(dir, "state"), IntentFile: filepath.Join(dir, "intent")}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
case "$1" in
core-network-facts) echo '{"network_format":"network-actions-v1","network_intent_format":"network-intent-v1","timezone":"UTC"}' ;;
core-apply-network)
 [ /proc/self/fd/3 -ef %[1]q ] && [ /proc/self/fd/4 -ef %[2]q ]
 flock -n -x 3 && flock -n -x 4
 n=0; [ ! -f %[3]q ] || n=$(head -1 %[3]q)
 n=$((n+1)); echo "$n" > %[3]q
 if [ -f %[4]q ]; then cp %[4]q %[5]q.$n; else echo absent > %[5]q.$n; fi
 if [ -f %[6]q ]; then rm %[6]q; echo '{"error":"injected native failure"}'; exit 1; fi
 id=$(sed -n '2s/^# intent-id //p' %[7]q)
 printf '{"status":"ok","intent_id":"%%s"}\n' "$id" ;;
*) exit 1 ;;
esac
`, a.LockFile, a.LockFile+".ticket", filepath.Join(dir, "count"), a.StateFile, filepath.Join(dir, "observed"), filepath.Join(dir, "fail-once"), a.IntentFile+".nft")
	if err := os.WriteFile(a.Shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPactPermitProjectionPreservesBlocksAndCanUndoFailedApply(t *testing.T) {
	for _, mode := range []string{"allow", "block", "none"} {
		t.Run(mode, func(t *testing.T) {
			a := networkAdapter(t)
			raw := "alex.override=" + mode + "\nalex.override_expires=2000\ntaylor.override=allow\nother=value\n"
			if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			c := rules.Configuration{Pact: &rules.ConfigurationPact{Captain: "alex", Type: "ongoing"}}
			err := a.withStateLocks(func(_, _ *os.File) error {
				undo, err := a.projectPactPermit(c, time.Unix(1000, 0))
				if err != nil {
					return err
				}
				probeStateLock(t, a.LockFile, true)
				probeStateLock(t, a.LockFile+".ticket", true)
				got, err := os.ReadFile(a.StateFile)
				if err != nil {
					return err
				}
				if mode != "allow" {
					if undo != nil || string(got) != raw {
						t.Fatal("non-allow state changed", string(got))
					}
					return nil
				}
				if undo == nil || string(got) != "taylor.override=allow\nother=value\nalex.override=none\nalex.override_expires=\n" {
					t.Fatal("wrong permit projection", string(got))
				}
				if err := undo(); err != nil {
					return err
				}
				got, err = os.ReadFile(a.StateFile)
				if err != nil || string(got) != raw {
					t.Fatal("failed-apply undo lost original RAM", string(got), err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			probeStateLock(t, a.LockFile, false)
			probeStateLock(t, a.LockFile+".ticket", false)
		})
	}
}

func TestConfigurationTransactionRejectsOldPactBridgeBeforeCallback(t *testing.T) {
	a := networkAdapter(t)
	script := "#!/bin/sh\ncase \"$1\" in\ncore-configuration) echo '{\"version\":1}';;\ncore-configuration-facts) echo '{\"bonus_format\":\"bonus-state-v1\"}';;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(a.Shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	err := a.ConfigurationTransaction(func(rules.Configuration, platform.Context, map[string]platform.NetworkOverride, func(rules.Configuration, platform.NetworkIntent) error) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatal("old bridge accepted before revocation guarantee", called, err)
	}
	probeStateLock(t, a.LockFile, false)
	probeStateLock(t, a.LockFile+".ticket", false)
}

func TestNetworkTransactionUsesInheritedLocksAndPreservesOtherState(t *testing.T) {
	a := networkAdapter(t)
	raw := "alex.override=allow\nalex.override_expires=12345\ntaylor.override=block\nother=value\n"
	if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.NetworkTransaction("alex", func(facts platform.Context, overrides map[string]platform.NetworkOverride, apply func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
		current := overrides["alex"]
		probeStateLock(t, a.LockFile, true)
		probeStateLock(t, a.LockFile+".ticket", true)
		if facts.NetworkFormat != "network-actions-v1" || current.Mode != "allow" || current.Expires != 12345 {
			t.Fatal(facts, current)
		}
		return apply(platform.NetworkOverride{Mode: "block"}, platform.NetworkIntent{Format: platform.NetworkIntentFormat}, platform.NetworkIntent{Format: platform.NetworkIntentFormat})
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
			err := a.NetworkTransaction("alex", func(_ platform.Context, _ map[string]platform.NetworkOverride, apply func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
				return apply(platform.NetworkOverride{Mode: "allow", Expires: 12345}, platform.NetworkIntent{Format: platform.NetworkIntentFormat}, platform.NetworkIntent{Format: platform.NetworkIntentFormat})
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
		err := a.NetworkTransaction("alex", func(platform.Context, map[string]platform.NetworkOverride, func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
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

func TestNetworkSnapshotServesLockedStateAndMastSync(t *testing.T) {
	a := networkAdapter(t)
	a.MastSeenFile = filepath.Join(filepath.Dir(a.StateFile), "mast")
	raw := "alex.override=allow\nalex.override_expires=12345\ntaylor.override=block\nother=value\n"
	if err := os.WriteFile(a.StateFile, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.MastSeenFile, []byte("captain-phone 999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.NetworkSnapshot(func(facts platform.Context, state map[string]platform.NetworkOverride) error {
		probeStateLock(t, a.LockFile, true)
		if facts.NetworkFormat != "network-actions-v1" || facts.TimeZone != "UTC" || facts.MastLastSync != 999 {
			t.Fatal(facts)
		}
		if state["alex"].Mode != "allow" || state["alex"].Expires != 12345 || state["taylor"].Mode != "block" {
			t.Fatal(state)
		}
		if _, has := state["other"]; has {
			t.Fatal("unrelated keys leaked into the snapshot", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	probeStateLock(t, a.LockFile, false)
}

func TestNetworkSnapshotReadsAbsentStateAsEmpty(t *testing.T) {
	a := networkAdapter(t)
	a.MastSeenFile = filepath.Join(filepath.Dir(a.StateFile), "mast")
	err := a.NetworkSnapshot(func(_ platform.Context, state map[string]platform.NetworkOverride) error {
		if len(state) != 0 {
			t.Fatal(state)
		}
		got, err := readAllOverrides(nil)
		if err != nil || len(got) != 0 {
			t.Fatal(got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal("absent state file must read as empty, not fail", err)
	}
	probeStateLock(t, a.LockFile, false)
}

func TestNetworkSnapshotRejectsMalformedRAMOrFacts(t *testing.T) {
	for name, mutate := range map[string]func(Adapter) error{
		"duplicate row": func(a Adapter) error {
			return os.WriteFile(a.StateFile, []byte("alex.override=allow\nalex.override=block\n"), 0600)
		},
		"invalid mode": func(a Adapter) error {
			return os.WriteFile(a.StateFile, []byte("alex.override=teleport\n"), 0600)
		},
		"invalid deadline": func(a Adapter) error {
			return os.WriteFile(a.StateFile, []byte("alex.override=allow\nalex.override_expires=soon\n"), 0600)
		},
		"row without value": func(a Adapter) error {
			return os.WriteFile(a.StateFile, []byte("alex.override\n"), 0600)
		},
		"orphan key": func(a Adapter) error {
			return os.WriteFile(a.StateFile, []byte(".override=allow\n"), 0600)
		},
		"broken facts": func(a Adapter) error {
			return os.WriteFile(a.Shell, []byte("#!/bin/sh\nexit 1\n"), 0700)
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := networkAdapter(t)
			if err := mutate(a); err != nil {
				t.Fatal(err)
			}
			called := false
			err := a.NetworkSnapshot(func(platform.Context, map[string]platform.NetworkOverride) error {
				called = true
				return nil
			})
			if err == nil || called {
				t.Fatal("corrupt snapshot reached the report", called, err)
			}
			probeStateLock(t, a.LockFile, false)
		})
	}
}

func TestMastSeenJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mast")
	if v, err := mastSeen(path); err != nil || v != 0 {
		t.Fatal("absent journal must read zero", v, err)
	}
	for content, want := range map[string]int64{"": 0, "device 42\n": 42} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if v, err := mastSeen(path); err != nil || v != want {
			t.Fatal(content, v, err)
		}
	}
	for _, content := range []string{"device\n", "device -3\n", "device notanumber\n", "a b c\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := mastSeen(path); err == nil {
			t.Fatal("malformed journal accepted:", content)
		}
	}
}
