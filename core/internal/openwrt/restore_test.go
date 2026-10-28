package openwrt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/usign"
)

func writeRouterKey(t *testing.T, dir string) usign.SecretKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := usign.SecretKey{Key: priv, KeyID: [8]byte{2}}
	raw := make([]byte, 104)
	copy(raw, "EdBK")
	sum := sha512.Sum512(priv)
	copy(raw[24:32], sum[:8])
	copy(raw[32:40], key.KeyID[:])
	copy(raw[40:], priv)
	if err := os.WriteFile(filepath.Join(dir, "mast.key"), []byte("untrusted comment: fictional\n"+base64.StdEncoding.EncodeToString(raw)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mast.pub"), []byte("untrusted comment: fictional\n"+key.Public()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return key
}

type restoreFixture struct {
	t                   *testing.T
	target, archive     string
	key                 usign.SecretKey
	adapter             Adapter
	legacyConfiguration string
}

func newRestoreFixture(t *testing.T, owned bool, signer *usign.SecretKey) *restoreFixture {
	t.Helper()
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "core"), 0700); err != nil {
		t.Fatal(err)
	}
	key := writeRouterKey(t, source)
	docs := map[string]string{"core/configuration.json": `{"version":1}`, "core/devices.json": `{"version":1,"devices":[{"id":"fw-alex"}]}`, "pact": "fictional"}
	if owned {
		docs["core/authority.json"] = `{"version":1,"owner":"core"}`
	}
	for name, data := range docs {
		if err := os.WriteFile(filepath.Join(source, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sign := key
	if signer != nil {
		sign = *signer
	}
	archive := filepath.Join(t.TempDir(), "household.tar.gz")
	if _, err := backup.Create(archive, backup.Source{Platform: "openwrt", Versions: map[string]string{"core": "test"}, Roots: map[string]string{"household": source}, Key: sign}); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "familywarden")
	if err := os.MkdirAll(filepath.Join(target, "core", "usage"), 0700); err != nil {
		t.Fatal(err)
	}
	writeRouterKey(t, target) // fresh, unpaired identity of the replacement router
	for name, data := range map[string]string{"core/.writer.lock": "", "core/devices.json": `{"version":1,"devices":[]}`, "core/devices.json.present": `{"version":1}`,
		"core/generation.json": `{"version":1,"epoch":"` + strings.Repeat("c", 32) + `","counter":1,"lineage":[]}`, "core/generation.json.present": `{"version":1}`} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f := &restoreFixture{t: t, target: target, archive: archive, key: key,
		legacyConfiguration: `{"version":1,"current":{"profiles":[],"legacy_phones":[]}}`}
	f.adapter = Adapter{KeyFile: filepath.Join(target, "mast.key"), LockFile: filepath.Join(parent, "state.lock"), DNSLockFile: filepath.Join(parent, "dns.lock")}
	return f
}

func (f *restoreFixture) restore(trusted usign.SecretKey, admit func(string) error) (string, error) {
	f.t.Helper()
	shell := filepath.Join(f.t.TempDir(), "adapter")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n[ \"$*\" = 'core-configuration configuration-v1 held-v1' ] || exit 1\necho '"+f.legacyConfiguration+"'\n"), 0700); err != nil {
		f.t.Fatal(err)
	}
	f.adapter.Shell = shell
	pub, _ := usign.ParsePublicKey(trusted.Public())
	_, previous, err := f.adapter.Restore(f.archive, pub, admit)
	return previous, err
}

func accept(string) error { return nil }

func TestRestorePlacesVerifiedHouseholdOnClearedRouter(t *testing.T) {
	f := newRestoreFixture(t, true, nil)
	freshKey, _ := os.ReadFile(filepath.Join(f.target, "mast.key"))
	var admitted string
	previous, err := f.restore(f.key, func(core string) error { admitted = core; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(admitted) != "core" {
		t.Fatal("admission did not inspect the staged core", admitted)
	}
	data, err := os.ReadFile(filepath.Join(f.target, "core", "devices.json"))
	if err != nil || string(data) != `{"version":1,"devices":[{"id":"fw-alex"}]}` {
		t.Fatal("restored registry missing", string(data), err)
	}
	restoredKey, _ := os.ReadFile(filepath.Join(f.target, "mast.key"))
	kept, _ := os.ReadFile(filepath.Join(previous, "mast.key"))
	if string(kept) != string(freshKey) || string(restoredKey) == string(freshKey) {
		t.Fatal("fresh identity was not preserved aside or restored identity not placed")
	}
	if info, err := os.Stat(filepath.Join(f.target, "core", "configuration.json")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("restored file not private", info, err)
	}
	staging, _ := filepath.Glob(filepath.Join(filepath.Dir(f.target), ".familywarden-restore-*"))
	if len(staging) != 0 {
		t.Fatal("staging left behind", staging)
	}
}

func TestRestoreRefusesUnsafeTargetsAndArchives(t *testing.T) {
	other := writeRouterKey(t, t.TempDir())
	for _, kind := range []string{"pact-file", "approved-phone", "legacy-profiles", "wrong-pin", "legacy-backup", "admission", "foreign-signer", "link"} {
		t.Run(kind, func(t *testing.T) {
			var signer *usign.SecretKey
			if kind == "foreign-signer" {
				signer = &other
			}
			f := newRestoreFixture(t, kind != "legacy-backup", signer)
			trusted, admit := f.key, accept
			switch kind {
			case "pact-file":
				os.WriteFile(filepath.Join(f.target, "pact"), []byte("x"), 0600)
			case "approved-phone":
				os.WriteFile(filepath.Join(f.target, "core", "devices.json"), []byte(`{"version":1,"devices":[{"id":"fw-other"}]}`), 0600)
			case "legacy-profiles":
				f.legacyConfiguration = `{"version":1,"current":{"profiles":[{"id":"robin"}]}}`
			case "wrong-pin":
				trusted = other
			case "admission":
				admit = func(string) error { return errors.New("future schema") }
			case "foreign-signer":
				trusted = other
			case "link":
				os.Symlink("/etc/passwd", filepath.Join(f.target, "core", "usage", "alias"))
			}
			before, _ := os.ReadFile(filepath.Join(f.target, "mast.key"))
			want := map[string]string{"pact-file": "not cleared: pact", "approved-phone": "approved phones",
				"legacy-profiles": "profiles, phones or a pact", "wrong-pin": "signature invalid",
				"legacy-backup": "core-owned", "admission": "future schema",
				"foreign-signer": "not the restored router identity", "link": "links"}[kind]
			if _, err := f.restore(trusted, admit); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("unsafe restore not refused for its reason: want %q, got %v", want, err)
			}
			after, _ := os.ReadFile(filepath.Join(f.target, "mast.key"))
			if string(before) != string(after) {
				t.Fatal("refused restore changed the target identity")
			}
			if _, err := os.Stat(filepath.Join(f.target, "core", "configuration.json")); !os.IsNotExist(err) {
				t.Fatal("refused restore activated household state", err)
			}
			if matches, _ := filepath.Glob(f.target + ".pre-restore-*"); len(matches) != 0 {
				t.Fatal("refused restore moved the target aside")
			}
		})
	}
}
