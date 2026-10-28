package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/openwrt"
	"github.com/denogio/familywarden/core/internal/usign"
)

func TestOfflineBackupRefusesActiveWriterBeforeNativeReads(t *testing.T) {
	dir := t.TempDir()
	writer, err := acquireWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	_, err = exportBackup(openwrt.Adapter{Shell: "/does-not-exist", KeyFile: "/does-not-exist"}, dir, out, "test")
	if err == nil || !strings.Contains(err.Error(), "writer lock") {
		t.Fatal("backup reached native state while daemon owned the store", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("live backup published an artifact", err)
	}
}

func TestBackupInspectionSummaryNeverExportsHouseholdContents(t *testing.T) {
	root := t.TempDir()
	secret := "fictional-sensitive-content"
	if err := os.WriteFile(filepath.Join(root, "secret"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := usign.SecretKey{Key: priv, KeyID: [8]byte{1}}
	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err := backup.Create(out, backup.Source{Platform: "test", Versions: map[string]string{"core": "test"}, Roots: map[string]string{"household": root}, Key: key}); err != nil {
		t.Fatal(err)
	}
	pubFile := filepath.Join(t.TempDir(), "pinned.pub")
	if err := os.WriteFile(pubFile, []byte("untrusted comment: fictional\n"+key.Public()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var summary bytes.Buffer
	if err := inspectBackup(out, pubFile, &summary); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(summary.Bytes(), &m); err != nil || m["restore_authorized"] != false || m["files"] != float64(1) {
		t.Fatal(m, err)
	}
	if strings.Contains(summary.String(), secret) || strings.Contains(summary.String(), "household/secret") || strings.Contains(summary.String(), root) {
		t.Fatal("backup inspection disclosed household data")
	}
	if err := os.WriteFile(pubFile, []byte("not a trusted key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := inspectBackup(out, pubFile, &bytes.Buffer{}); err == nil {
		t.Fatal("invalid external pin accepted")
	}
}
