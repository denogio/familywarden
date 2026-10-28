package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/denogio/familywarden/core/internal/store"
)

// A mixed-version or damaged household must be rejected before startup rolls
// back a pending edit, applies native rules, or imports/removes legacy devices.
func TestStartupChecksStoredSchemasBeforeRecovery(t *testing.T) {
	for _, name := range []string{"configuration.json", "authority.json", "devices.json", "requests.json", "newapps.json", "dns.json"} {
		damages := []string{"future-version", "truncated-json"}
		if name == "configuration.json" || name == "devices.json" {
			damages = append(damages, "missing")
		}
		for _, damage := range damages {
			t.Run(name+"/"+damage, func(t *testing.T) {
				f := newPortableFixture(t)
				c := f.configuration()
				c.Current.Profiles[0].Name = "Alex awaiting confirmation"
				if r := f.api("configuration_apply", map[string]any{"revision": c.Revision, "current": c.Current}); r["status"] != "confirmation_required" {
					t.Fatal(r)
				}
				f.app.Close()
				st, err := store.Open(f.dir)
				if err != nil {
					t.Fatal(err)
				}
				data := []byte(`{"version":999}`)
				if damage == "truncated-json" {
					data = []byte(`{"version":1,`)
				}
				path := filepath.Join(f.dir, name)
				if damage == "missing" {
					err = os.Remove(path)
				} else {
					err = os.WriteFile(path, data, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				before := documentSnapshot(t, f.dir)
				applies := len(f.host.intents)
				app, err := New(f.host, f.host, st, "migration-test")
				if app != nil {
					app.Close()
				}
				if err == nil {
					t.Error("unsupported or malformed document accepted at startup")
				}
				if len(f.host.intents) != applies {
					t.Error("incompatible startup changed native enforcement")
				}
				after := documentSnapshot(t, f.dir)
				if !bytes.Equal(before, after) {
					t.Error("incompatible startup changed durable documents")
				}
			})
		}
	}
}

func TestOwnedHouseholdCannotFallBackAfterConfigurationLoss(t *testing.T) {
	f := newPortableFixture(t)
	captain := f.phone("fw-jordan", "jordan")
	f.api("configuration_pact", map[string]any{"until": 0, "window_day": "sun", "window_start": "10:00", "window_stop": "11:00"})
	path := filepath.Join(f.dir, "configuration.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Allow the fixture to return a permissive legacy projection, so a fallback
	// is observable rather than merely being stopped by the usual tripwire.
	f.host.owned = false
	f.host.current.Pact = nil
	f.host.current.Current.Settings.Enabled = "0"
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	before := documentSnapshot(t, f.dir)
	applies := len(f.host.intents)
	if r := f.call("/api/configuration_export", nil); r["error"] == nil {
		t.Error("lost owned document selected legacy authority", r)
	}
	if code, body := f.exchange("/phone", captain.query()); code != 500 || len(body) != 0 {
		t.Error("lost owned state produced a policy", code, string(body))
	}
	f.app.Close()
	st, err := store.Open(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(f.host, f.host, st, "migration-test")
	if app != nil {
		app.Close()
	}
	if err == nil || f.host.legacyCalls != 0 || len(f.host.intents) != applies || !bytes.Equal(before, documentSnapshot(t, f.dir)) {
		t.Fatal("restart fell back, imported or changed state after owned document loss", err, f.host.legacyCalls)
	}
	// Explicitly repairing the exact journal recovers the same pact and identity;
	// the ownership floor is not an instruction to invent a replacement document.
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if f.configuration().Pact == nil || objectField(t, f.sync(captain)["pact"])["active"] != true {
		t.Fatal("repair lost the pact or device binding")
	}
}

func documentSnapshot(t *testing.T, dir string) []byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents[filepath.Base(path)] = string(data)
	}
	data, err := json.Marshal(contents)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
