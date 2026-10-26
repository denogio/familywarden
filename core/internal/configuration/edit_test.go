package configuration

import (
	"fmt"
	"strings"
	"testing"
)

func TestExplicitAmendmentCommitsWithoutAnAutomaticPactRollback(t *testing.T) {
	a, b, s := setup(t)
	activate(t, a, b)
	day := strings.ToLower(a.Now().Weekday().String()[:3])
	okResult(t, a.API("configuration_pact", []byte(fmt.Sprintf(`{"until":0,"window_day":"%s","window_start":"00:00","window_stop":"24:00"}`, day))))
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Profiles[0].ScreenMinutes = "5"
	req := request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})
	if a.API("configuration_apply", req)["error"] == nil {
		t.Fatal("ordinary confirmation could undo a pact amendment")
	}
	okResult(t, a.API("configuration_amend", req))
	d, _, _ = a.load()
	if d.Pending != nil || d.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "5" {
		t.Fatal("amendment is not a committed frozen policy", d)
	}
	recovered, e := New(b, b, s)
	if e != nil {
		t.Fatal(e)
	}
	d, _, _ = recovered.load()
	if d.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "5" {
		t.Fatal("restart undid a committed pact")
	}
}

func TestClientIDAllocationLivesInTheCoreTransaction(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Settings.DNSDomain = "dns.example.invalid"
	next.Current.Profiles[1].DNS = "1"
	next.Current.Profiles[1].ClientID = ""
	r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))
	okResult(t, r)
	d, _, _ = a.load()
	id := d.Configuration.Current.Profiles[1].ClientID
	if !strings.HasPrefix(id, "fw") || len(id) != 34 {
		t.Fatal("missing portable ClientID allocation", id)
	}
	okResult(t, a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": r["token"]})))
	d, _, _ = a.load()
	if d.Configuration.Current.Profiles[1].ClientID != id {
		t.Fatal("confirmation regenerated identity")
	}
}

func TestMalformedAndDuplicateEditsDoNotBecomeAuthority(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	for _, raw := range []string{`{"revision":"a","revision":"b","current":{}}`, `{} {}`, `{"revision":"a","unknown":true}`} {
		if a.API("configuration_apply", []byte(raw))["error"] == nil {
			t.Fatal("invalid edit accepted", raw)
		}
	}
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Profiles[1].ScreenMinutes = "not-a-limit"
	if a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))["error"] == nil {
		t.Fatal("invalid limit silently normalized")
	}
	if _, e := a.Context(); e != nil {
		t.Fatal("validation error blocked healthy authority", e)
	}
}
