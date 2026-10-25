package identity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func usageWithNewApps(seq int64, newApps string) string {
	return strings.Replace(usageJSON("fw-kid", "kid", usageDay, seq, 60, `{}`, ""), `"incomplete":false`,
		`"incomplete":false,"new_apps":`+newApps, 1)
}

func appDecisionJSON(dev, profile, pkg, action string, created int64) string {
	return fmt.Sprintf(`{"type":"app_decision","v":1,"device":"%s","profile":"%s","package":"%s","action":"%s","created":%d}`,
		dev, profile, pkg, action, created)
}

func approvedList(t *testing.T, m map[string]any) []string {
	t.Helper()
	out := []string{}
	list, _ := m["apps"].(map[string]any)["approved"].([]any)
	for _, p := range list {
		out = append(out, p.(string))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestNewAppIsReportedApprovedByParentAndSigned(t *testing.T) {
	f := usageFamily(t)
	caps := "apps-v1,grants-v1,screen-time-v1,shared-v1"
	before := reply(t, f.s, f.kid.query("fw-kid", true, "-", caps))
	if m := send(t, f.kid, f.s, "usage", usageWithNewApps(1, `{"com.example.newgame":"New Game"}`)); m["state"] != "stored" {
		t.Fatal(m)
	}
	pm := reply(t, f.s, f.parent.query("fw-parent", true, "-", caps))
	list := pm["new_apps"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["package"] != "com.example.newgame" || list[0].(map[string]any)["label"] != "New Game" {
		t.Fatalf("parent new apps: %v", pm["new_apps"])
	}
	// The sibling-scope parent "other" may also decide; the child itself may not.
	if m := send(t, f.kid, f.s, "app", appDecisionJSON("fw-kid", "kid", "com.example.newgame", "approve", t0)); m["error"] != "not_parent" {
		t.Fatalf("child approved its own app: %v", m)
	}
	// A parent of a different child has no say over this one.
	stranger := newPhone(t)
	f.ad.ctx.Profiles["sib"] = platformProfile("Sib", "2222222222222222")
	enroll(t, f.s, stranger, "fw-stranger", "adult", "sib")
	if m := send(t, stranger, f.s, "app", appDecisionJSON("fw-stranger", "kid", "com.example.newgame", "approve", t0)); m["error"] != "not_parent" {
		t.Fatalf("out-of-scope parent approved: %v", m)
	}
	if m := send(t, f.parent, f.s, "app", appDecisionJSON("fw-parent", "kid", "com.example.newgame", "approve", t0-3600)); m["error"] != "decision_expired" {
		t.Fatalf("stale decision: %v", m)
	}
	if m := send(t, f.parent, f.s, "app", appDecisionJSON("fw-parent", "kid", "com.example.newgame", "approve", t0)); m["state"] != "approved" {
		t.Fatal(m)
	}
	after := reply(t, f.s, f.kid.query("fw-kid", true, "-", caps))
	if !contains(approvedList(t, after), "com.example.newgame") {
		t.Fatalf("approval not in signed policy: %v", after["apps"])
	}
	if after["apps"].(map[string]any)["rev"] == before["apps"].(map[string]any)["rev"] {
		t.Fatal("approval did not change the app revision")
	}
	if find(f.s, "fw-kid")["apps_rev"] != after["apps"].(map[string]any)["rev"] {
		t.Fatal("LuCI expects another revision than the phone gets")
	}
	if l := reply(t, f.s, f.parent.query("fw-parent", true, "-", caps))["new_apps"].([]any); len(l) != 0 {
		t.Fatalf("approved app still waiting: %v", l)
	}
	// Every further approval must change the revision again (phones re-apply and ack).
	send(t, f.kid, f.s, "usage", usageWithNewApps(2, `{"com.example.draw":"Draw"}`))
	f.s.AppDecision(AppDecisionInput{"kid", "com.example.draw", "approve"})
	third := reply(t, f.s, f.kid.query("fw-kid", true, "-", caps))
	if third["apps"].(map[string]any)["rev"] == after["apps"].(map[string]any)["rev"] {
		t.Fatal("a second approval did not change the app revision")
	}
	// Re-reporting an approved app changes nothing; revoking removes it again.
	send(t, f.kid, f.s, "usage", usageWithNewApps(3, `{"com.example.newgame":"New Game"}`))
	if r := f.s.AppDecision(AppDecisionInput{"kid", "com.example.newgame", "revoke"}); r["state"] != "revoked" {
		t.Fatal(r)
	}
	if contains(approvedList(t, reply(t, f.s, f.kid.query("fw-kid", true, "-", caps))), "com.example.newgame") {
		t.Fatal("revoked app still approved")
	}
}

func TestNewAppDenyHidesAndDoesNotApprove(t *testing.T) {
	f := usageFamily(t)
	send(t, f.kid, f.s, "usage", usageWithNewApps(1, `{"com.example.chat":"Chat","com.example.draw":"Draw"}`))
	if r := f.s.AppDecision(AppDecisionInput{"kid", "com.example.chat", "deny"}); r["state"] != "denied" || r["status"] != "ok" {
		t.Fatal(r)
	}
	send(t, f.kid, f.s, "usage", usageWithNewApps(2, `{"com.example.chat":"Chat","com.example.draw":"Draw"}`))
	pending := f.s.NewApps()["profiles"].([]Reply)[0]["pending"].([]object)
	if len(pending) != 1 || pending[0][0].v != "com.example.draw" {
		t.Fatalf("denied app came back: %v", pending)
	}
	// Uninstalled (no longer reported by that phone) disappears.
	send(t, f.kid, f.s, "usage", usageWithNewApps(3, `{}`))
	if p := f.s.NewApps()["profiles"].([]Reply)[0]["pending"].([]object); len(p) != 0 {
		t.Fatalf("unreported app still pending: %v", p)
	}
	if r := f.s.AppDecision(AppDecisionInput{"kid", "com.example.unseen", "approve"}); r["error"] != "unknown_app" {
		t.Fatalf("approved an app no phone reported: %v", r)
	}
}

func TestNewAppApprovalRespectsPactsBlocksAndBounds(t *testing.T) {
	f := usageFamily(t)
	send(t, f.kid, f.s, "usage", usageWithNewApps(1, `{"com.example.newgame":"New Game"}`))
	f.ad.ctx.PactCaptain = "kid"
	if r := f.s.AppDecision(AppDecisionInput{"kid", "com.example.newgame", "approve"}); r["error"] != "pact_forbids" {
		t.Fatalf("approval loosened a pact: %v", r)
	}
	if m := send(t, f.parent, f.s, "app", appDecisionJSON("fw-parent", "kid", "com.example.newgame", "approve", t0)); m["error"] != "not_parent" {
		t.Fatalf("parent approved for a pact-bound profile: %v", m)
	}
	f.ad.ctx.PactCaptain = ""
	f.s.AppDecision(AppDecisionInput{"kid", "com.example.newgame", "approve"})
	// A blocked app is never approved by a core approval.
	raw, err := withApprovals([]byte(`{"rev":"x","approved":["a.b"],"blocked":["com.example.newgame"]}`), []string{"com.example.newgame", "c.d"})
	if err != nil || strings.Contains(strings.Split(string(raw), `"blocked"`)[0], "com.example.newgame") || !strings.Contains(string(raw), `"c.d"`) {
		t.Fatalf("blocked app approved: %s %v", raw, err)
	}
	// Malformed reports are refused as a whole; too many apps too.
	big := "{"
	for i := 0; i <= maxNewPending; i++ {
		big += fmt.Sprintf(`"com.example.a%d":"A",`, i)
	}
	big = strings.TrimSuffix(big, ",") + "}"
	for _, bad := range []string{big, `{"../x":"X"}`, `{"total":"X"}`, `{"com.example.a":"\u0007"}`} {
		if m := send(t, f.kid, f.s, "usage", usageWithNewApps(9, bad)); m["error"] != "malformed" {
			t.Fatalf("accepted %s: %v", bad, m)
		}
	}
	// Persisted: a restart keeps the approval.
	s2, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	s2.Now, s2.Usage = func() time.Time { return time.Unix(t0, 0) }, f.s.Usage
	if !contains(approvedList(t, reply(t, s2, f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1,shared-v1"))), "com.example.newgame") {
		t.Fatal("approval lost on restart")
	}
}

func TestFailedNewAppWriteChangesNothing(t *testing.T) {
	f := usageFamily(t)
	send(t, f.kid, f.s, "usage", usageWithNewApps(1, `{"com.example.newgame":"New Game"}`))
	f.st.failing = true
	if r := f.s.AppDecision(AppDecisionInput{"kid", "com.example.newgame", "approve"}); r["error"] != "unavailable" {
		t.Fatal(r)
	}
	f.st.failing = false
	if len(f.s.approvedFor("kid")) != 0 {
		t.Fatal("failed write approved an app in RAM")
	}
}
