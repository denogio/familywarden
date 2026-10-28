package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentLinksAndDirectoriesAreNotAbsence(t *testing.T) {
	s, dir := open(t)
	for _, name := range []string{"dangling.json", "linked.json", "directory.json"} {
		path := filepath.Join(dir, name)
		switch name {
		case "dangling.json":
			if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
				t.Fatal(err)
			}
		case "linked.json":
			if err := s.WriteDocument("target.json", map[string]int{"version": 1}); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(dir, "target.json"), path); err != nil {
				t.Fatal(err)
			}
		default:
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
		var got map[string]any
		if _, err := s.ReadDocument(name, &got); err == nil {
			t.Fatal("non-regular document was accepted or treated as absent", name)
		}
	}
}

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
