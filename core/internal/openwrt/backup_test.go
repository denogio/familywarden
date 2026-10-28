package openwrt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/usign"
)

func TestBackupSnapshotLocksAndKeyContinuity(t *testing.T) {
	dir := t.TempDir()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := usign.SecretKey{Key: priv, KeyID: [8]byte{1}}
	raw := make([]byte, 104)
	copy(raw, []byte("EdBK"))
	sum := sha512.Sum512(priv)
	copy(raw[24:32], sum[:8])
	copy(raw[32:40], key.KeyID[:])
	copy(raw[40:], priv)
	secret := []byte("untrusted comment: fictional\n" + base64.StdEncoding.EncodeToString(raw) + "\n")
	if err := os.WriteFile(filepath.Join(dir, "mast.key"), secret, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mast.pub"), []byte("untrusted comment: fictional\n"+key.Public()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(dir, "adapter")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n[ \"$*\" = 'call version' ] || exit 1\necho '{\"backend\":\"fixture-r1\",\"frontend\":\"fixture-r1\",\"configuration_owner\":\"core\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a := Adapter{KeyFile: filepath.Join(dir, "mast.key"), Shell: shell, LockFile: filepath.Join(dir, "state.lock"), DNSLockFile: filepath.Join(dir, "dns.lock"), BackupConfigFile: filepath.Join(dir, "config")}
	called := false
	err = a.Backup(func(s backup.Source) error {
		called = true
		if s.Platform != "openwrt" || s.Key.Public() != key.Public() || s.Roots["household"] != dir || s.Roots["platform-config"] != a.BackupConfigFile || s.Versions["configuration_owner"] != "" {
			t.Fatal("wrong snapshot source metadata")
		}
		for _, path := range []string{a.LockFile + ".ticket", a.LockFile, a.DNSLockFile} {
			f, err := os.OpenFile(path, os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			f.Close()
			if !errors.Is(err, syscall.EWOULDBLOCK) {
				t.Fatal("snapshot did not hold native lock", path, err)
			}
		}
		return nil
	})
	if err != nil || !called {
		t.Fatal(called, err)
	}
	for _, path := range []string{a.LockFile + ".ticket", a.LockFile, a.DNSLockFile} {
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		f.Close()
		if err != nil {
			t.Fatal("backup leaked native lock", err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "mast.key")); err != nil {
		t.Fatal(err)
	}
	if err := a.Backup(func(backup.Source) error { t.Fatal("missing key reached callback"); return nil }); err == nil {
		t.Fatal("missing signing key accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "mast.key")); !os.IsNotExist(err) {
		t.Fatal("backup regenerated the identity", err)
	}
}
