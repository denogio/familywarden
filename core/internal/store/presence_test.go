package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPresenceAdoptsExistingDataWithoutRewritingIt(t *testing.T) {
	s, dir := open(t)
	path := filepath.Join(dir, "requests.json")
	original := `{"version":1, "last_time":1, "requests":[]}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.AdoptPresence(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatal("adoption rewrote the existing ledger", err)
	}
	stat, err := os.Stat(path + PresenceSuffix)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("presence witness is not private", stat, err)
	}
	if err := s.AdoptPresence(); err != nil {
		t.Fatal("adoption is not idempotent", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckPresence(); err == nil {
		t.Fatal("lost adopted ledger not detected")
	}
}

func TestPresenceVersionAndMissingWitnessRemainDistinct(t *testing.T) {
	s, dir := open(t)
	var got any
	if found, err := s.ReadDocument("never-created.json", &got); err != nil || found {
		t.Fatal("genuine first use refused", found, err)
	}
	if err := s.WriteDocument("requests.json", map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{"version":999}`, `{`, `null`} {
		if err := os.WriteFile(filepath.Join(dir, "requests.json"+PresenceSuffix), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReadDocument("requests.json", &got); err == nil {
			t.Fatal("unsupported presence metadata ignored", data)
		}
		if err := s.CheckPresence(); err == nil {
			t.Fatal("unsupported presence metadata passed startup", data)
		}
	}
}

func TestPresenceAdoptionValidatesBeforeWriting(t *testing.T) {
	s, dir := open(t)
	for name, data := range map[string]string{"a.json": `{}`, "z.json": `{`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AdoptPresence(); err == nil {
		t.Fatal("malformed existing state adopted")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"+PresenceSuffix))
	if len(files) != 0 {
		t.Fatal("adoption modified metadata before validating all input", files)
	}
}

type obstructPresence struct{ path string }

func (v obstructPresence) MarshalJSON() ([]byte, error) {
	// Inject a marker-write failure after presence validation but before data
	// persistence, without adding a production filesystem abstraction/test hook.
	return []byte(`{"version":1}`), os.Mkdir(v.path+PresenceSuffix, 0700)
}

func TestFailedWitnessWriteCannotBeAcknowledgedOrFollowedByMoreWrites(t *testing.T) {
	s, dir := open(t)
	path := filepath.Join(dir, "requests.json")
	if err := s.WriteDocument("requests.json", obstructPresence{path}); err == nil {
		t.Fatal("witness failure reported success")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("test did not reach the data-before-witness interruption", err)
	}
	if err := s.WriteDocument("other.json", map[string]int{"version": 1}); err == nil {
		t.Fatal("uncertain writer continued")
	}
	var data any
	if _, err := s.ReadDocument("requests.json", &data); err == nil {
		t.Fatal("uncertain data became a successful read")
	}
	if err := os.Remove(path + PresenceSuffix); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.AdoptPresence(); err != nil {
		t.Fatal("reopen did not finish interrupted presence adoption", err)
	}
	if found, err := reopened.ReadDocument("requests.json", &data); err != nil || !found {
		t.Fatal(found, err)
	}
}

type removeParentDuringMarshal struct{ dir string }

func (v removeParentDuringMarshal) MarshalJSON() ([]byte, error) {
	return []byte(`{}`), os.RemoveAll(v.dir)
}

func TestUncertainNotExistErrorCannotMasqueradeAsFirstUse(t *testing.T) {
	s, dir := open(t)
	if err := s.WriteDocument("requests.json", removeParentDuringMarshal{dir}); err == nil {
		t.Fatal("interrupted write reported success")
	}
	var value any
	if found, err := s.ReadDocument("configuration.json", &value); err == nil {
		t.Fatal("poisoned ENOENT became a legacy selector", found)
	}
}

func TestLostUsageCannotBecomeZeroOrRestartCounters(t *testing.T) {
	s, dir := open(t)
	r := report(phoneKey, 1, 600, nil)
	r.Day = time.Now().UTC().Format(time.DateOnly)
	if _, err := s.ApplyUsage("alex", r); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "usage", "alex", r.Day+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UsageDay("alex", r.Day); err == nil {
		t.Fatal("lost accounting became measured zero")
	}
	r.Seq, r.Total = 2, 0
	if _, err := s.ApplyUsage("alex", r); err == nil {
		t.Fatal("higher sequence restarted lost counters")
	}
}

func TestPreviouslyWrittenDocumentCannotBecomeFirstUse(t *testing.T) {
	s, dir := open(t)
	if err := s.WriteDocument("requests.json", map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "requests.json")); err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if found, err := s.ReadDocument("requests.json", &got); err == nil {
		t.Fatal("lost ledger became a first-use empty ledger", found)
	}
	if err := s.WriteDocument("requests.json", map[string]int{"version": 1}); err == nil {
		t.Fatal("cached writer silently recreated a lost ledger")
	}
}
