package configuration

import (
	"github.com/denogio/familywarden/core/internal/platform"
	"strings"
	"testing"
	"time"
)

func TestAppRuleJournalPreflightRejectsCorruption(t *testing.T) {
	for _, mutate := range []func(*document){
		func(d *document) { d.RuleCommits[0].Digest = "not-a-hash" },
		func(d *document) { d.RuleCommits = append(d.RuleCommits, d.RuleCommits[0]) },
		func(d *document) { d.RuleTime = -1 },
		func(d *document) { d.RuleCommits[0].Expires = 0 },
	} {
		a, b, st := setup(t)
		activate(t, a, b)
		d, _, _ := a.load()
		hash := strings.Repeat("a", 64)
		d.RuleCommits = []ruleCommit{{Owner: hash, Nonce: hash, Digest: hash, Expires: a.Now().Unix() + 600}}
		mutate(&d)
		if err := st.WriteDocument(documentName, d); err != nil {
			t.Fatal(err)
		}
		before := string(st.data)
		if _, err := CheckStorage(st); err == nil {
			t.Fatal("invalid replay journal accepted")
		}
		if before != string(st.data) {
			t.Fatal("preflight mutated storage")
		}
	}
}

func TestAppRuleStatusDoesNotRollbackExpiredRootConfirmation(t *testing.T) {
	a, b, st := setup(t)
	b.current.Current.Profiles[0].Apps = "1"
	_ = b.current.Seal()
	activate(t, a, b)
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Profiles[0].Name = "Alex updated"
	okResult(t, a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})))
	now := a.Now().Add(3 * time.Minute)
	a.Now = func() time.Time { return now }
	before := string(st.data)
	hash := strings.Repeat("a", 64)
	r := a.AppRule(RuleChange{Owner: hash, Nonce: hash, Digest: hash, Profile: "alex", Package: "com.example.video", Mode: "blocked", Revision: "old", Expires: now.Unix() + 600}, true, func(_ platform.Context) bool { return true })
	if r["error"] != "confirmation_pending" || before != string(st.data) {
		t.Fatal("status changed expired confirmation", r)
	}
}
