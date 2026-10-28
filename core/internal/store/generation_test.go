package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerationAdvancesBeforeHouseholdWritesOnly(t *testing.T) {
	s, dir := open(t)
	if g, err := s.Generation(); err != nil || g.Counter != 0 || g.Epoch != "" {
		t.Fatal("first use is not an empty generation", g, err)
	}
	if err := s.WriteDocument("devices.json", map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Generation()
	if err != nil || first.Counter != 1 || !validEpoch.MatchString(first.Epoch) {
		t.Fatal(first, err)
	}
	if err := s.WriteDocument("requests.json", map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	r := report(phoneKey, 1, 1, nil)
	r.Day = time.Now().UTC().Format(time.DateOnly)
	if _, err := s.ApplyUsage("alex", r); err != nil {
		t.Fatal(err)
	}
	second, _ := s.Generation()
	if second.Epoch != first.Epoch || second.Counter != 2 {
		t.Fatal("generation did not advance once per household document, excluding usage", second)
	}
	if err := s.WriteDocument(GenerationDocument, second); err != ErrInvalidID {
		t.Fatal("product code could overwrite the generation", err)
	}
	reopened, _ := Open(dir)
	if g, err := reopened.Generation(); err != nil || g.Counter != 2 || g.Epoch != first.Epoch {
		t.Fatal("generation not durable", g, err)
	}
}

func TestForkKeepsBoundedDistinctLineage(t *testing.T) {
	s, _ := open(t)
	if err := s.WriteDocument("devices.json", map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Generation()
	if err := s.Fork(); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Generation()
	if after.Epoch == before.Epoch || after.Counter != 0 || len(after.Lineage) != 1 ||
		after.Lineage[0] != (Ancestor{before.Epoch, before.Counter}) {
		t.Fatal("fork lost the restored fork point", after)
	}
	for i := 0; i < 2*maxLineage; i++ {
		if err := s.Fork(); err != nil {
			t.Fatal(err)
		}
	}
	g, err := s.Generation()
	if err != nil || len(g.Lineage) != maxLineage || g.valid() != nil {
		t.Fatal("lineage not bounded/valid", len(g.Lineage), err)
	}
	fresh, _ := open(t)
	if err := fresh.Fork(); err != nil {
		t.Fatal(err)
	}
	if g, _ := fresh.Generation(); len(g.Lineage) != 0 || !validEpoch.MatchString(g.Epoch) {
		t.Fatal("pre-generation backup must fork without ancestry", g)
	}
}

func TestGenerationLossAndDamageRefuse(t *testing.T) {
	for _, damage := range []string{"missing", "future", "duplicate-epoch"} {
		t.Run(damage, func(t *testing.T) {
			s, dir := open(t)
			if err := s.WriteDocument("devices.json", map[string]int{"version": 1}); err != nil {
				t.Fatal(err)
			}
			g, _ := s.Generation()
			path := filepath.Join(dir, GenerationDocument)
			var err error
			switch damage {
			case "missing":
				err = os.Remove(path)
			case "future":
				err = os.WriteFile(path, []byte(`{"version":2,"epoch":"`+g.Epoch+`","counter":1,"lineage":[]}`), 0600)
			default:
				err = os.WriteFile(path, []byte(`{"version":1,"epoch":"`+g.Epoch+`","counter":1,"lineage":[{"epoch":"`+g.Epoch+`","counter":0}]}`), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			reopened, _ := Open(dir)
			if reopened.CheckGeneration() == nil && reopened.CheckPresence() == nil {
				t.Fatal("damaged generation accepted")
			}
			if err := reopened.WriteDocument("devices.json", map[string]int{"version": 1}); err == nil {
				t.Fatal("write proceeded over a damaged generation")
			}
		})
	}
}
