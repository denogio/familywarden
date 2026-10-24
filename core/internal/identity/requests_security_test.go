package identity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecisionRequiresExactHashAndExplicitBoolean(t *testing.T) {
	for _, mutate := range []func(string) string{
		func(s string) string { return strings.Replace(s, `"request_sha256":"HASH"`, `"request_sha256":""`, 1) },
		func(s string) string { return strings.Replace(s, `"request_sha256":"HASH",`, "", 1) },
		func(s string) string { return strings.Replace(s, `"approve":true`, `"approve":null`, 1) },
		func(s string) string { return strings.Replace(s, `"approve":true,`, "", 1) },
	} {
		f := setupFamily(t)
		raw, _ := f.request(t, "security-001", 30)
		decision := decisionJSON("security-001", "HASH", "fw-parent", true, 20, t0)
		decision = strings.Replace(mutate(decision), "HASH", sha(raw), 1)
		if m := send(t, f.parent, f.s, "decision", decision); m["error"] != "malformed" {
			t.Fatalf("ambiguous decision accepted: %s: %v", decision, m)
		}
	}
}

func TestSignedUploadsRejectDuplicateCaseAliasedAndNullFields(t *testing.T) {
	for _, fields := range []string{`"minutes":10,"minutes":20`, `"minutes":10,"Minutes":20`, `"minutes":null`} {
		f := setupFamily(t)
		raw := requestJSON("security-002", "fw-kid", "kid", "total", 30, t0)
		raw = strings.Replace(raw, `"minutes":30`, fields, 1)
		if m := send(t, f.kid, f.s, "request", raw); m["error"] != "malformed" {
			t.Fatalf("ambiguous upload accepted: %s: %v", raw, m)
		}
	}
	var v requestMsg
	if decodeStrict([]byte(requestJSON("security-002", "fw-kid", "kid", "total", 30, t0)+" garbage"), &v, "type v request_id device profile target minutes created", "label note") {
		t.Fatal("malformed trailing bytes treated as EOF")
	}
}

func TestGrantBelongsOnlyToRequestingPhone(t *testing.T) {
	f := setupFamily(t)
	f.request(t, "security-003", 30)
	if r := f.s.AnswerRequest(Answer{RequestID: "security-003", Approve: true, Minutes: 20}); r["state"] != "approved" {
		t.Fatal(r)
	}
	second := newPhone(t)
	enroll(t, f.s, second, "fw-second", "kid")
	m := reply(t, f.s, second.query("fw-second", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	if m["apps"].(map[string]any)["grants"] != nil {
		t.Fatal("one grant was duplicated onto another phone")
	}
}

func TestApprovalRechecksRequesterAndAppControl(t *testing.T) {
	for _, change := range []string{"delete", "rekey", "move", "off"} {
		t.Run(change, func(t *testing.T) {
			f := setupFamily(t)
			raw, _ := f.request(t, "security-004", 30)
			switch change {
			case "delete":
				f.s.Delete("fw-kid")
			case "move":
				f.s.Update(Change{Device: "fw-kid", Profile: "parent"})
			case "rekey":
				enroll(t, f.s, newPhone(t), "fw-kid", "kid")
			case "off":
				f.ad.ctx.Profiles["kid"] = platformProfile("Kid", "")
			}
			m := send(t, f.parent, f.s, "decision", decisionJSON("security-004", sha(raw), "fw-parent", true, 20, t0))
			if m["error"] == nil {
				t.Fatalf("approved after %s: %v", change, m)
			}
			if len(f.ad.tickets) != 0 {
				t.Fatal("opened internet for an invalidated request")
			}
		})
	}
}

func TestRetryRepairsFailedTicketWithoutExtendingGrant(t *testing.T) {
	f := setupFamily(t)
	f.ad.ticketOK = false
	f.request(t, "security-005", 30)
	first := f.s.AnswerRequest(Answer{RequestID: "security-005", Approve: true, Minutes: 20})
	f.ad.ticketOK = true
	again := f.s.AnswerRequest(Answer{RequestID: "security-005", Approve: true, Minutes: 20})
	if first["home_internet"] != false || again["home_internet"] != true || first["not_after"] != again["not_after"] {
		t.Fatalf("retry did not recover at the original deadline: %v / %v", first, again)
	}
}

func TestCorruptLedgerDoesNotSilentlyBecomeAnEmptyGrantPolicy(t *testing.T) {
	f := setupFamily(t)
	f.st.docs[requestsFile] = []byte(`{"version":1,"requests":[{"id":"bad","state":"approved"}]}`)
	f.s.ledger = nil
	code, body := f.s.Phone(f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	if code != StatusError || body != nil {
		t.Fatalf("corrupt ledger produced a fresh policy: %d %s", code, body)
	}
}

func TestExpiryCannotBeRewoundAfterRestart(t *testing.T) {
	f := setupFamily(t)
	f.request(t, "security-006", 30)
	f.s.AnswerRequest(Answer{RequestID: "security-006", Approve: true, Minutes: 20})
	f.s.Now = func() time.Time { return time.Unix(t0+7200, 0) }
	reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	restarted, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = func() time.Time { return time.Unix(t0+3600, 0) }
	code, body := restarted.Phone(f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	if code == StatusOK {
		_, raw, _ := strings.Cut(string(body), "\n")
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		if m["apps"].(map[string]any)["grants"] != nil {
			t.Fatal("clock rollback revived an expired grant")
		}
	}
}
