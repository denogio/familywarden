package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentsAreBoundedAndAtomic(t *testing.T) {
	s, dir := open(t)
	want := map[string]any{"version": float64(1), "status": "approved"}
	if err := s.WriteDocument("requests.json", want); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if ok, err := s.ReadDocument("requests.json", &got); err != nil || !ok || got["status"] != want["status"] {
		t.Fatal(got, err)
	}
	stat, err := os.Stat(filepath.Join(dir, "requests.json"))
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("private ledger permissions", stat, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requests.json"), make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadDocument("requests.json", &got); err == nil {
		t.Fatal("oversized document accepted")
	}
	for _, name := range []string{"../requests.json", "requests/other.json", "requests"} {
		if err := s.WriteDocument(name, want); err != ErrInvalidID {
			t.Fatal("unsafe document name accepted", name, err)
		}
	}
}
