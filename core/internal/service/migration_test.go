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
	for _, name := range []string{"configuration.json", "devices.json", "requests.json", "newapps.json", "dns.json"} {
		for _, damage := range []string{"future-version", "truncated-json"} {
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
				if err := os.WriteFile(filepath.Join(f.dir, name), data, 0600); err != nil {
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
