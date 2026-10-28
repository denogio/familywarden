package configuration

import (
	"bytes"
	"errors"
	"testing"
)

func TestOwnershipRequiresNativeGuardAndAdoptsExistingHousehold(t *testing.T) {
	a, b, s := bonusSetup(t, "")
	b.facts.AuthorityFormat = ""
	if r := a.API("configuration_import", request(map[string]any{"revision": b.current.Revision})); r["error"] == nil {
		t.Fatal("old adapter activated core ownership", r)
	}
	if len(s.data) != 0 || len(s.authority) != 0 || b.applies != 0 {
		t.Fatal("incompatible import changed state")
	}
	b.facts.AuthorityFormat = OwnershipFormat
	activate(t, a, b.fakeBackend)
	before := append([]byte{}, s.data...)
	revision := b.current.Revision
	// An existing v1 household from r18 has no separate ownership floor yet.
	s.authority = nil
	b.facts.AuthorityFormat = ""
	applies := b.applies
	if _, err := New(b, b, s); err == nil {
		t.Fatal("old adapter adopted the ownership floor")
	}
	if len(s.authority) != 0 || b.applies != applies || !bytes.Equal(s.data, before) {
		t.Fatal("incompatible adoption changed household/native state")
	}
	b.facts.AuthorityFormat = OwnershipFormat
	restarted, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if owned, err := restarted.ownership(); !owned || err != nil || b.current.Revision != revision {
		t.Fatal("existing household was not adopted without revision changes", owned, err)
	}
}

type failedOwnershipReadback struct{ memory }

func (s *failedOwnershipReadback) ReadDocument(name string, v any) (bool, error) {
	if name == OwnershipDocument && len(s.authority) > 0 {
		return false, errors.New("ownership readback unavailable")
	}
	return s.memory.ReadDocument(name, v)
}

func TestOwnershipReadbackMustSucceedBeforeNativeApply(t *testing.T) {
	b := &fakeBackend{current: sample()}
	if err := b.current.Seal(); err != nil {
		t.Fatal(err)
	}
	st := &failedOwnershipReadback{}
	a, err := New(b, b, st)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if r := a.API("configuration_import", request(map[string]any{"revision": b.current.Revision})); r["error"] == nil || b.applies != 0 {
		t.Fatal("unverified ownership floor reached native apply", r, b.applies)
	}
	if len(st.data) == 0 || len(st.authority) == 0 {
		t.Fatal("test did not reach ownership readback")
	}
	// Removing only the injected read failure completes the durable exact journal.
	recovered, err := New(b, b, &st.memory)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if r := recovered.API("configuration_import", []byte(`{}`)); r["already_imported"] != true {
		t.Fatal("interrupted ownership adoption was not idempotent", r)
	}
}
