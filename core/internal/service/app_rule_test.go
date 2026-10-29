package service

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/rules"
	"strings"
	"testing"
	"time"
)

func ruleQuery(f *portableFixture, p portablePhone, rev, mode string, minutes int) map[string]any {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		f.t.Fatal(err)
	}
	return map[string]any{"type": "rule", "v": 1, "device": p.id, "profile": "alex", "package": "com.example.video", "mode": mode, "minutes": minutes, "apps_rev": rev, "nonce": hex.EncodeToString(nonce), "created": f.now().Unix(), "expires": f.now().Unix() + 600}
}
func appsRevision(f *portableFixture, p portablePhone) string {
	r := f.upload(p, "installed", map[string]any{"type": "installed", "v": 1, "device": p.id, "profile": "alex", "after": "", "nonce": strings.Repeat("b", 64), "created": f.now().Unix()})
	return r["apps_rev"].(string)
}

func TestPortableAppRuleTransitionsStatusAndRestart(t *testing.T) {
	f := newPortableFixture(t)
	parent := f.phone("fw-parent", "robin", "alex")
	child := f.phone("fw-alex", "alex")
	before := f.configuration()
	for _, mode := range []string{"blocked", "limit", "always", "allowed", "limit"} {
		q := ruleQuery(f, parent, appsRevision(f, parent), mode, 0)
		if r := f.upload(parent, "rule_status", q); r["state"] != "retryable" {
			t.Fatal(r)
		}
		if r := f.upload(parent, "rule", q); r["state"] != "saved" {
			t.Fatal(r)
		}
		rev := f.configuration().Revision
		// Model a lost response, an exact retry and restart: equality is not a receipt.
		f.restart()
		if r := f.upload(parent, "rule_status", q); r["state"] != "matched" {
			t.Fatal(r)
		}
		if r := f.upload(parent, "rule", q); r["state"] != "matched" || f.configuration().Revision != rev {
			t.Fatal(r)
		}
		apps := objectField(t, f.sync(child)["apps"])
		if mode == "limit" && len(apps["limits"].([]any)) != 1 {
			t.Fatal(apps)
		}
	}
	after := f.configuration()
	if after.Current.Settings != before.Current.Settings || len(after.Current.Schedules) != len(before.Current.Schedules) || after.Current.Profiles[0].ScreenMinutes != "40" {
		t.Fatal("unrelated policy changed")
	}
}

func TestPortableAppRuleCASAndCurrentAuthority(t *testing.T) {
	f := newPortableFixture(t)
	a := f.phone("fw-parent-a", "robin", "alex")
	b := f.phone("fw-parent-b", "robin", "alex")
	child := f.phone("fw-alex", "alex")
	rev := appsRevision(f, a)
	qa, qb := ruleQuery(f, a, rev, "blocked", 0), ruleQuery(f, b, rev, "always", 0)
	if r := f.upload(a, "rule", qa); r["state"] != "saved" {
		t.Fatal(r)
	}
	if r := f.upload(b, "rule", qb); r["state"] != "conflict" {
		t.Fatal(r)
	}
	if r := f.upload(b, "rule_status", qb); r["state"] != "conflict" {
		t.Fatal(r)
	}
	unscoped := f.phone("fw-administrator", "robin")
	f.api("device_update", map[string]any{"device": unscoped.id, "profile": "robin", "administrator": "1"})
	if r := f.upload(unscoped, "rule", ruleQuery(f, unscoped, rev, "allowed", 0)); r["error"] != "not_parent" {
		t.Fatal("administrator/new key bypassed assigned scope", r)
	}
	q := ruleQuery(f, child, appsRevision(f, child), "allowed", 0)
	if r := f.upload(child, "rule", q); r["error"] != "not_parent" {
		t.Fatal(r)
	}
	f.api("device_update", map[string]any{"device": a.id, "profile": "robin", "approves": ""})
	if r := f.upload(a, "rule_status", qa); r["error"] != "not_parent" {
		t.Fatal("matched state leaked after revocation", r)
	}
	// Expiry is checked at commit, but status still reconciles old signed operations.
	qb = ruleQuery(f, b, appsRevision(f, b), "allowed", 0)
	f.advance(601 * time.Second)
	if r := f.upload(b, "rule", qb); r["state"] != "expired" {
		t.Fatal(r)
	}
}

func TestPortableAppRuleRejectsMalformedAndUnknownApplyOutcome(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-parent", "robin", "alex")
	q := ruleQuery(f, p, appsRevision(f, p), "limit", 1441)
	before := f.configuration().Revision
	if r := f.upload(p, "rule", q); r["error"] != "malformed" || f.configuration().Revision != before {
		t.Fatal(r)
	}
	q = ruleQuery(f, p, appsRevision(f, p), "blocked", 0)
	q["administrator"] = true
	if r := f.upload(p, "rule", q); r["error"] != "malformed" {
		t.Fatal(r)
	}
	delete(q, "administrator")
	f.host.mu.Lock()
	f.host.failConfiguration = 1
	f.host.mu.Unlock()
	if r := f.upload(p, "rule", q); r["error"] != "outcome_unknown" {
		t.Fatal(r)
	}
	if r := f.upload(p, "rule_status", q); r["error"] != "outcome_unknown" {
		t.Fatal(r)
	}
	f.restart()
	if r := f.upload(p, "rule_status", q); r["state"] != "matched" {
		t.Fatal(r)
	}
}

func TestPortableAppRuleLegacyRefusalCannotActivateOwnership(t *testing.T) {
	// Reuse a production host but a fresh, unowned store. No phone-triggered import.
	f := newPortableFixture(t)
	f.app.Close()
	f.dir = t.TempDir()
	f.host.mu.Lock()
	f.host.owned = false
	f.host.mu.Unlock()
	f.start()
	p := f.phone("fw-parent", "robin", "alex")
	before := f.configurationLegacyRevision()
	generation, err := f.app.Identity.Generation()
	if err != nil {
		t.Fatal(err)
	}
	q := ruleQuery(f, p, "0123456789abcdef", "blocked", 0)
	for _, op := range []string{"rule", "rule_status"} {
		if r := f.upload(p, op, q); r["error"] != "configuration_required" {
			t.Fatal(r)
		}
	}
	after, err := f.app.Identity.Generation()
	if err != nil || after.Counter != generation.Counter || after.Epoch != generation.Epoch || before != f.configurationLegacyRevision() {
		t.Fatal("legacy refusal mutated authority", err)
	}
	f.host.mu.Lock()
	f.host.legacyCalls = 0
	f.host.mu.Unlock() // expected legacy reads in this test
}
func (f *portableFixture) configurationLegacyRevision() string {
	r := f.api("configuration_export", nil)
	if r["owner"] != "legacy" {
		f.t.Fatal("phone imported configuration", r)
	}
	return objectField(f.t, r["configuration"])["revision"].(string)
}

func TestPortableAppRulePactAndOldKeyNeverBypassScope(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-parent", "robin", "alex", "jordan")
	q := ruleQuery(f, p, appsRevision(f, p), "blocked", 0)
	// Re-key: the old signed operation is no longer authenticated.
	f.phone(p.id, "robin", "alex", "jordan")
	raw := mustJSON(t, q)
	code, _ := f.exchange("/app/rule", []byte(p.key.Sign(protocol.Message("rule", raw))+"\n"+string(raw)))
	if code != 403 {
		t.Fatal("old key accepted", code)
	}
	p = f.phone("fw-second-parent", "robin", "jordan")
	f.api("configuration_pact", map[string]any{"until": f.now().Add(time.Hour).Unix()})
	q = ruleQuery(f, p, "0123456789abcdef", "always", 0)
	q["profile"] = "jordan"
	before := f.configuration().Revision
	if r := f.upload(p, "rule", q); r["error"] != "not_parent" || before != f.configuration().Revision {
		t.Fatal(r)
	}
}

func TestPortableAppRuleSimultaneousParentsAndReadOnlyStatus(t *testing.T) {
	f := newPortableFixture(t)
	a := f.phone("fw-a", "robin", "alex")
	b := f.phone("fw-b", "robin", "alex")
	rev := appsRevision(f, a)
	qa, qb := ruleQuery(f, a, rev, "blocked", 0), ruleQuery(f, b, rev, "always", 0)
	generation, _ := f.app.Identity.Generation()
	before := f.configuration().Revision
	for i := 0; i < 2; i++ {
		if r := f.upload(a, "rule_status", qa); r["state"] != "retryable" {
			t.Fatal(r)
		}
	}
	after, _ := f.app.Identity.Generation()
	if before != f.configuration().Revision || generation.Counter != after.Counter || generation.Epoch != after.Epoch {
		t.Fatal("status mutated authority")
	}
	start := make(chan struct{})
	results := make(chan map[string]any, 2)
	for _, v := range []struct {
		p portablePhone
		q map[string]any
	}{{a, qa}, {b, qb}} {
		go func(p portablePhone, q map[string]any) { <-start; results <- f.upload(p, "rule", q) }(v.p, v.q)
	}
	close(start)
	states := map[any]int{}
	for i := 0; i < 2; i++ {
		states[(<-results)["state"]]++
	}
	if states["saved"] != 1 || states["conflict"] != 1 {
		t.Fatal(states)
	}
}

func TestPortableAppRuleCommittedNoncePreventsABAAfterRootEditAndRestart(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-parent", "robin", "alex")
	original := f.configuration()
	q := ruleQuery(f, p, appsRevision(f, p), "blocked", 0)
	if r := f.upload(p, "rule", q); r["state"] != "saved" {
		t.Fatal(r)
	}
	// Another administrator/parent restores the exact base policy, including its
	// content-hash revision. Root apply/confirmation must preserve replay protection.
	current := f.configuration()
	current.Current = original.Current
	f.commit(current)
	if rev := appsRevision(f, p); rev != q["apps_rev"] {
		t.Fatal("fixture did not restore base hash", rev, q)
	}
	f.restart()
	before := f.configuration().Revision
	generation, _ := f.app.Identity.Generation()
	for _, op := range []string{"rule_status", "rule", "rule_status"} {
		if r := f.upload(p, op, q); r["state"] != "conflict" {
			t.Fatal("ABA revived committed intent", r)
		}
	}
	mutated := map[string]any{}
	for k, v := range q {
		mutated[k] = v
	}
	mutated["mode"] = "always"
	if r := f.upload(p, "rule", mutated); r["error"] != "replay" {
		t.Fatal("nonce reused for different body", r)
	}
	after, _ := f.app.Identity.Generation()
	if f.configuration().Revision != before || generation.Counter != after.Counter {
		t.Fatal("replay/status mutated authority")
	}
	f.api("device_update", map[string]any{"device": p.id, "profile": "robin", "approves": ""})
	if r := f.upload(p, "rule_status", q); r["error"] != "not_parent" {
		t.Fatal("journal bypassed current authorization", r)
	}
}

func TestPortableAppRuleReplayJournalCapacityAndWriteOnlyPruning(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-parent", "robin", "alex")
	original := f.configuration()
	var first map[string]any
	for i := 0; i < 64; i++ {
		mode := "blocked"
		if i%2 == 1 {
			mode = "always"
		}
		q := ruleQuery(f, p, appsRevision(f, p), mode, 0)
		if i == 0 {
			first = q
		}
		if r := f.upload(p, "rule", q); r["state"] != "saved" {
			t.Fatal(i, r)
		}
	}
	f.restart()
	q := ruleQuery(f, p, appsRevision(f, p), "allowed", 0)
	before := f.configuration().Revision
	generation, _ := f.app.Identity.Generation()
	if r := f.upload(p, "rule", q); r["error"] != "operation_capacity" {
		t.Fatal(r)
	}
	after, _ := f.app.Identity.Generation()
	if f.configuration().Revision != before || after.Counter != generation.Counter {
		t.Fatal("capacity refusal mutated authority")
	}
	f.advance(601 * time.Second)
	// Status must not prune expired entries, advance generation or change config.
	fresh := ruleQuery(f, p, appsRevision(f, p), "allowed", 0)
	generation, _ = f.app.Identity.Generation()
	if r := f.upload(p, "rule_status", fresh); r["state"] != "retryable" {
		t.Fatal(r)
	}
	after, _ = f.app.Identity.Generation()
	if after.Counter != generation.Counter {
		t.Fatal("status pruned expired journal")
	}
	if r := f.upload(p, "rule", fresh); r["state"] != "saved" {
		t.Fatal("write did not prune expired entries", r)
	}
	// After pruning, restoring the base hash and rolling back wall time cannot
	// revive the old signed intent: the committed time floor is durable too.
	restored := f.configuration()
	restored.Current = original.Current
	f.commit(restored)
	f.advance(-601 * time.Second)
	f.restart()
	if r := f.upload(p, "rule", first); r["state"] != "expired" {
		t.Fatal("clock rollback revived pruned intent", r)
	}

}

func TestPortableAppRuleUsesAuthoritativeScopeWithSparseConfigurationFacts(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-parent", "robin", "alex", "jordan")
	// Reproduce the native fixture: no total limit, suspend-new enabled and one
	// unrelated one-minute app row. Configuration transactions contain only the
	// OpenWrt handshake/timezone envelope, unlike ordinary runtime phone reads.
	c := f.configuration()
	c.Current.Profiles[0].ScreenMinutes = ""
	c.Current.Profiles[0].SuspendNew = "1"
	c.Current.Apps = []rules.ConfigurationApp{{ID: "fixture", Profile: "alex", Package: "com.example.fixture", Minutes: "1"}}
	f.commit(c)
	f.host.mu.Lock()
	f.host.sparseConfigurationFacts = true
	f.host.mu.Unlock()
	f.api("configuration_bonus_add", map[string]any{"profile": "alex", "minutes": 5})
	q := ruleQuery(f, p, appsRevision(f, p), "blocked", 0)
	q["package"] = "com.example.puzzle"
	if r := f.upload(p, "rule_status", q); r["state"] != "retryable" {
		t.Fatal(r)
	}
	r := f.upload(p, "rule", q)
	if r["state"] != "saved" {
		t.Fatal("sparse transaction facts refused native-shaped rule", r)
	}
	if r["apps_rev"] != appsRevision(f, p) {
		t.Fatal("write/read base revisions disagree", r)
	}
	config := f.configuration()
	if len(config.Current.Apps) != 2 || config.Current.Apps[0].Package != "com.example.fixture" || config.Current.Apps[0].Minutes != "1" {
		t.Fatal("unrelated rule changed", config.Current.Apps)
	}
	// No platform pact_captain exists in this transaction envelope: authorization
	// must nevertheless exclude the authoritative pact target.
	f.api("configuration_pact", map[string]any{"until": f.now().Add(time.Hour).Unix()})
	q = ruleQuery(f, p, "0123456789abcdef", "always", 0)
	q["profile"] = "jordan"
	before := f.configuration().Revision
	if r := f.upload(p, "rule", q); r["error"] != "not_parent" || before != f.configuration().Revision {
		t.Fatal("sparse facts bypassed core pact", r)
	}
}
