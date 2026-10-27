package configuration

import (
	"testing"
)

func TestProtectedReleaseOwnsPhraseAndCoreStatus(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	okResult(t, a.API("configuration_pact", []byte(`{"until":1800100000}`)))
	before, _, _ := a.load()
	if r := a.Recovery("request", []byte(`{"answer":"wrong"}`)); r["error"] == nil {
		t.Fatal(r)
	}
	after, _, _ := a.load()
	if before.Configuration.Revision != after.Configuration.Revision {
		t.Fatal("wrong phrase mutated pact")
	}
	r := a.Recovery("request", []byte(`{"answer":"  PRESS ON!  "}`))
	if r["error"] != nil || r["pact"] != true || r["captain"] != "alex" || r["release_at"].(int64) <= a.Now().Unix() {
		t.Fatal(r)
	}
	deadline := r["release_at"]
	r = a.Recovery("request", []byte(`{"answer":"press on"}`))
	if r["release_at"] != deadline {
		t.Fatal("retry changed cooling-off deadline", r)
	}
	r = a.Recovery("cancel", []byte(`{}`))
	if r["error"] != nil || r["release_at"] != int64(0) {
		t.Fatal(r)
	}
	if r := a.Recovery("request", []byte(`{"answer":"press on","answer":"wrong"}`)); r["error"] == nil {
		t.Fatal("duplicate answer accepted")
	}
}
