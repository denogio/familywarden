package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/usign"
)

const t0 = 1800000000

func (p phone) upload(kind, raw string) []byte {
	return []byte(usign.Sign(p.priv, p.id, protocol.Message(kind, []byte(raw))) + "\n" + raw)
}

func requestJSON(id, dev, profile, target string, minutes int, created int64) string {
	return fmt.Sprintf(`{"type":"request","v":1,"request_id":"%s","device":"%s","profile":"%s","target":"%s","minutes":%d,"label":"Video","created":%d}`,
		id, dev, profile, target, minutes, created)
}

func decisionJSON(id, hash, dev string, approve bool, minutes int, created int64) string {
	return fmt.Sprintf(`{"type":"decision","v":1,"request_id":"%s","request_sha256":"%s","device":"%s","approve":%t,"minutes":%d,"created":%d}`,
		id, hash, dev, approve, minutes, created)
}

func sha(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// send uploads, verifies the router signature and decodes a reply to an upload.
func send(t *testing.T, p phone, s *Service, kind, raw string) map[string]any {
	t.Helper()
	code, body := p.sendTo(s, kind, raw)
	return uploadReply(t, code, body, raw)
}

func uploadReply(t *testing.T, code int, body []byte, raw string) map[string]any {
	t.Helper()
	if code != StatusOK {
		t.Fatalf("status %d", code)
	}
	sig, rest, _ := strings.Cut(string(body), "\n")
	pub, _ := usign.ParsePublicKey(routerKey(t).Public())
	if !pub.Verify(sig, []byte(rest)) {
		t.Fatal("upload reply not signed by the router")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(rest), &m); err != nil {
		t.Fatal(err)
	}
	if m["message_sha256"] != sha(raw) {
		t.Fatal("upload reply is not bound to the exact uploaded bytes")
	}
	return m
}

type family struct {
	s           *Service
	ad          *fakeAdapter
	st          *memStore
	kid, parent phone
	other       phone
}

// setupFamily: kid (app control on), parent may approve for kid; other parent
// may approve for kid too; "adult" is no one's child.
func setupFamily(t *testing.T) family {
	s, ad, st := setup(t)
	f := family{s: s, ad: ad, st: st, kid: newPhone(t), parent: newPhone(t), other: newPhone(t)}
	ad.ticketOK = true
	enroll(t, s, f.kid, "fw-kid", "kid")
	enroll(t, s, f.parent, "fw-parent", "parent", "kid")
	enroll(t, s, f.other, "fw-other", "adult", "kid")
	return f
}

func (f family) request(t *testing.T, id string, minutes int) (string, map[string]any) {
	raw := requestJSON(id, "fw-kid", "kid", "com.example.video", minutes, t0)
	return raw, send(t, f.kid, f.s, "request", raw)
}

func (p phone) sendTo(s *Service, kind, raw string) (int, []byte) {
	return s.App(kind, p.upload(kind, raw))
}

func TestRequestLifecycleApproveDeliverExpire(t *testing.T) {
	f := setupFamily(t)
	raw, m := f.request(t, "req-00000001", 30)
	if m["state"] != "pending" || m["request_id"] != "req-00000001" || m["device"] != "fw-kid" {
		t.Fatalf("submit: %v", m)
	}
	// The parent sees it, with the hash it must sign; the child sees its status.
	pm := reply(t, f.s, f.parent.query("fw-parent", true, "-", "apps-v1,screen-time-v1"))
	reqs := pm["requests"].([]any)
	if len(reqs) != 1 || reqs[0].(map[string]any)["request_sha256"] != sha(raw) || reqs[0].(map[string]any)["label"] != "Video" {
		t.Fatalf("parent view: %v", pm["requests"])
	}
	km := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1"))
	if km["request"].(map[string]any)["state"] != "pending" {
		t.Fatalf("child view: %v", km["request"])
	}
	oldRev := km["apps"].(map[string]any)["rev"]

	dm := send(t, f.parent, f.s, "decision", decisionJSON("req-00000001", sha(raw), "fw-parent", true, 20, t0))
	if dm["state"] != "approved" || dm["granted"] != float64(20) || dm["not_after"] != float64(t0+7200) || dm["home_internet"] != true {
		t.Fatalf("decision: %v", dm)
	}
	if len(f.ad.tickets) != 1 || f.ad.tickets[0] != fmt.Sprintf("kid@%d", t0+7200) {
		t.Fatalf("ticket: %v", f.ad.tickets)
	}

	// A phone without grant support is refused (it would silently ignore the grant).
	if r := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1")); r["error"] != "unsupported_capabilities" {
		t.Fatalf("old client got a grant policy: %v", r)
	}
	km = reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	apps := km["apps"].(map[string]any)
	grants := apps["grants"].([]any)
	if len(grants) != 1 || apps["rev"] == oldRev || grants[0].(map[string]any)["minutes"] != float64(20) ||
		grants[0].(map[string]any)["target"] != "com.example.video" {
		t.Fatalf("grant in policy: %v", apps)
	}
	if km["request"].(map[string]any)["state"] != "approved" || km["request"].(map[string]any)["answered_by"] != "fw-parent" {
		t.Fatalf("child status: %v", km["request"])
	}
	if find(f.s, "fw-kid")["apps_rev"] != apps["rev"] {
		t.Fatal("LuCI revision differs from the signed one")
	}
	// Acknowledging the grant revision marks it delivered.
	reply(t, f.s, f.kid.query("fw-kid", true, apps["rev"].(string), "apps-v1,grants-v1,screen-time-v1"))
	if r := f.s.Requests()["requests"].([]Reply)[0]; r["delivered"] != true || r["state"] != "approved" {
		t.Fatalf("delivery: %v", r)
	}
	// Two hours after approval the grant is gone and the revision is back.
	f.s.Now = func() time.Time { return time.Unix(t0+7200, 0) }
	km = reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1"))
	if km["apps"].(map[string]any)["grants"] != nil || km["apps"].(map[string]any)["rev"] != oldRev {
		t.Fatalf("expired grant still in policy: %v", km["apps"])
	}
}

func TestDecisionsFirstWinsIdempotentAndBound(t *testing.T) {
	f := setupFamily(t)
	raw, _ := f.request(t, "req-00000002", 30)
	approve := decisionJSON("req-00000002", sha(raw), "fw-parent", true, 30, t0)
	for name, c := range map[string]struct {
		p    phone
		raw  string
		want string
	}{
		"hash of other bytes": {f.parent, decisionJSON("req-00000002", sha(raw+" "), "fw-parent", true, 30, t0), "unknown_request"},
		"more than asked":     {f.parent, decisionJSON("req-00000002", sha(raw), "fw-parent", true, 31, t0), "invalid_minutes"},
		"stale decision":      {f.parent, decisionJSON("req-00000002", sha(raw), "fw-parent", true, 30, t0-3600), "decision_expired"},
		"own profile":         {f.kid, decisionJSON("req-00000002", sha(raw), "fw-kid", true, 30, t0), "not_parent"},
	} {
		if m := send(t, c.p, f.s, "decision", c.raw); m["error"] != c.want {
			t.Errorf("%s: %v", name, m)
		}
	}
	first := send(t, f.parent, f.s, "decision", approve)
	again := send(t, f.parent, f.s, "decision", approve)
	if first["state"] != "approved" || again["state"] != "approved" || again["not_after"] != first["not_after"] {
		t.Fatalf("retry: %v vs %v", first, again)
	}
	if len(f.ad.tickets) != 2 || f.ad.tickets[0] != f.ad.tickets[1] {
		t.Fatal("an idempotent retry changed the ticket deadline")
	}
	m := send(t, f.other, f.s, "decision", decisionJSON("req-00000002", sha(raw), "fw-other", false, 0, t0))
	if m["error"] != "already_answered" {
		t.Fatalf("second parent: %v", m)
	}
	if send(t, f.parent, f.s, "decision", decisionJSON("req-00000002", sha(raw), "fw-parent", false, 0, t0))["error"] != "already_answered" {
		t.Fatal("same parent changed its answer")
	}
}

func TestRequestRulesAndReplay(t *testing.T) {
	f := setupFamily(t)
	cases := map[string]struct {
		raw  string
		want string
	}{
		"wrong profile": {requestJSON("req-a0000001", "fw-kid", "parent", "total", 30, t0), "wrong_profile"},
		"zero minutes":  {requestJSON("req-a0000002", "fw-kid", "kid", "total", 0, t0), "invalid_minutes"},
		"over 240":      {requestJSON("req-a0000003", "fw-kid", "kid", "total", 241, t0), "invalid_minutes"},
		"bad target":    {requestJSON("req-a0000004", "fw-kid", "kid", "../x", 30, t0), "unknown_package"},
		"old":           {requestJSON("req-a0000005", "fw-kid", "kid", "total", 30, t0-3600), "request_expired"},
		"future":        {requestJSON("req-a0000006", "fw-kid", "kid", "total", 30, t0+600), "request_expired"},
		"short id":      {requestJSON("r1", "fw-kid", "kid", "total", 30, t0), "malformed"},
		"unknown field": {strings.Replace(requestJSON("req-a0000007", "fw-kid", "kid", "total", 30, t0), `"v":1`, `"v":1,"admin":true`, 1), "malformed"},
	}
	for name, c := range cases {
		if m := send(t, f.kid, f.s, "request", c.raw); m["error"] != c.want {
			t.Errorf("%s: %v", name, m)
		}
	}
	// Unparsable or unauthenticated input gets no signed reply.
	if code, body := f.kid.sendTo(f.s, "request", requestJSON("req-a0000008", "fw-kid", "kid", "total", 30, t0)+"{}"); code != StatusMalformed || body != nil {
		t.Fatal("trailing data accepted")
	}
	raw := requestJSON("req-b0000001", "fw-kid", "kid", "total", 30, t0)
	if code, body := f.s.App("request", f.parent.upload("request", raw)); code != StatusForbidden || body != nil {
		t.Fatal("request signed by another phone accepted")
	}
	if code, _ := f.s.App("request", f.kid.upload("decision", raw)); code != StatusForbidden {
		t.Fatal("signature for another message type accepted")
	}
	if code, _ := f.s.App("grant", f.kid.upload("request", raw)); code != StatusForbidden {
		t.Fatal("unknown op accepted")
	}
	// Total target, retry, replay with other bytes, one open request per child.
	if m := send(t, f.kid, f.s, "request", raw); m["state"] != "pending" {
		t.Fatal(m)
	}
	if m := send(t, f.kid, f.s, "request", raw); m["state"] != "pending" {
		t.Fatalf("retry: %v", m)
	}
	if m := send(t, f.kid, f.s, "request", strings.Replace(raw, `"minutes":30`, `"minutes":60`, 1)); m["error"] != "replay" {
		t.Fatalf("replay with other bytes: %v", m)
	}
	if m := send(t, f.kid, f.s, "request", requestJSON("req-b0000002", "fw-kid", "kid", "total", 30, t0)); m["error"] != "request_open" {
		t.Fatalf("second open request: %v", m)
	}
	// After an hour it has expired: a new one is allowed, the old can't be approved.
	f.s.Now = func() time.Time { return time.Unix(t0+3600, 0) }
	if m := send(t, f.parent, f.s, "decision", decisionJSON("req-b0000001", sha(raw), "fw-parent", true, 30, t0+3600)); m["error"] != "request_expired" {
		t.Fatalf("approved an expired request: %v", m)
	}
	if m := send(t, f.kid, f.s, "request", requestJSON("req-b0000003", "fw-kid", "kid", "total", 30, t0+3600)); m["state"] != "pending" {
		t.Fatalf("new request after expiry: %v", m)
	}
}

func TestPactAndAppControlGateRequests(t *testing.T) {
	f := setupFamily(t)
	adult := newPhone(t)
	enroll(t, f.s, adult, "fw-adult", "adult")
	f.ad.ctx.Profiles["adult"] = platformProfile("Adult", "1111111111111111")
	f.ad.ctx.PactCaptain = "adult"
	if m := send(t, adult, f.s, "request", requestJSON("req-c0000001", "fw-adult", "adult", "total", 30, t0)); m["error"] != "pact_forbids" {
		t.Fatalf("captain under pact may request: %v", m)
	}
	if m := send(t, f.parent, f.s, "request", requestJSON("req-c0000002", "fw-parent", "parent", "total", 30, t0)); m["error"] != "app_control_off" {
		t.Fatalf("request without app control: %v", m)
	}
	// A pending request of a profile that becomes pact-bound can't be approved.
	f.ad.ctx.PactCaptain = ""
	raw, _ := f.request(t, "req-c0000003", 30)
	f.ad.ctx.PactCaptain = "kid"
	if m := send(t, f.parent, f.s, "decision", decisionJSON("req-c0000003", sha(raw), "fw-parent", true, 30, t0)); m["error"] != "not_parent" {
		t.Fatalf("approved for a pact-bound profile: %v", m)
	}
	if r := f.s.AnswerRequest(Answer{RequestID: "req-c0000003", Approve: true, Minutes: 30}); r["error"] != "not_parent" {
		t.Fatalf("administrator approved for a pact-bound profile: %v", r)
	}
}

func TestLedgerSurvivesRestartAndFailedWrites(t *testing.T) {
	f := setupFamily(t)
	raw, _ := f.request(t, "req-d0000001", 30)
	if r := f.s.AnswerRequest(Answer{RequestID: "req-d0000001", Approve: false}); r["state"] != "denied" || r["status"] != "ok" {
		t.Fatalf("administrator deny: %v", r)
	}
	// Restart: the answered request stays answered and its id stays used.
	s2, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	s2.Now = f.s.Now
	if m := send(t, f.kid, s2, "request", raw); m["state"] != "denied" {
		t.Fatalf("after restart: %v", m)
	}
	if m := send(t, f.parent, s2, "decision", decisionJSON("req-d0000001", sha(raw), "fw-parent", true, 30, t0)); m["error"] != "already_answered" {
		t.Fatalf("revived after restart: %v", m)
	}
	// A failed write changes nothing.
	f.st.failing = true
	if m := send(t, f.kid, s2, "request", requestJSON("req-d0000002", "fw-kid", "kid", "total", 30, t0)); m["error"] != "unavailable" {
		t.Fatalf("failed write: %v", m)
	}
	f.st.failing = false
	if m := send(t, f.kid, s2, "request", requestJSON("req-d0000002", "fw-kid", "kid", "total", 30, t0)); m["state"] != "pending" {
		t.Fatalf("after failed write: %v", m)
	}
}

func TestManualBlockOrPactLeavesTicketClosed(t *testing.T) {
	f := setupFamily(t)
	f.ad.ticketOK = false // the adapter reports a manual block
	f.request(t, "req-e0000001", 30)
	r := f.s.AnswerRequest(Answer{RequestID: "req-e0000001", Approve: true, Minutes: 15})
	if r["state"] != "approved" || r["home_internet"] != false {
		t.Fatalf("grant must stand, internet reported closed: %v", r)
	}
}

func platformProfile(name, rev string) platform.Profile {
	return platform.Profile{Name: name, AppsRev: rev, Requires: []string{"apps-v1"}}
}
