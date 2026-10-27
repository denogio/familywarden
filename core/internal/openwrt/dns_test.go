package openwrt

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
)

func TestNativeDNSSecretAndImportTransaction(t *testing.T) {
	dir := t.TempDir()
	a := Adapter{KeyFile: filepath.Join(dir, "mast.key"), DNSLockFile: filepath.Join(dir, "dns.lock")}
	if err := a.SetDNSCredentials("parent", "test-password"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dns.ack"), []byte("alex fw-alex dns.example.test - -\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.DNSTransaction(func(v platform.DNSRuntime) error {
		probeStateLock(t, a.DNSLockFile, true)
		if v.Auth != base64.StdEncoding.EncodeToString([]byte("parent:test-password")) || len(v.LegacyAcks) != 1 {
			t.Fatal(v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	probeStateLock(t, a.DNSLockFile, false)
	stat, err := os.Stat(filepath.Join(dir, "agh.auth"))
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal(stat, err)
	}
	if err := a.SetDNSCredentials("bad:name", "pass"); err == nil {
		t.Fatal("invalid native secret accepted")
	}
}
