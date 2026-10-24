package identity

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentParentsAndIdenticalRetries(t *testing.T) {
	f := setupFamily(t)
	raw, _ := f.request(t, "concurrent-001", 30)
	start := make(chan struct{})
	type result struct {
		code int
		body []byte
		raw  string
	}
	replies := make(chan result, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			p, dev, minutes := f.parent, "fw-parent", 10
			if i%2 == 1 {
				p, dev, minutes = f.other, "fw-other", 20
			}
			message := decisionJSON("concurrent-001", sha(raw), dev, true, minutes, t0)
			code, body := p.sendTo(f.s, "decision", message)
			replies <- result{code, body, message}
		}(i)
	}
	close(start)
	wg.Wait()
	close(replies)
	winningMinutes := 0.0
	for result := range replies {
		r := uploadReply(t, result.code, result.body, result.raw)
		if r["error"] == "already_answered" {
			continue
		}
		if r["error"] != nil {
			t.Fatal(r)
		}
		got := r["granted"].(float64)
		if winningMinutes != 0 && got != winningMinutes {
			t.Fatal("two different decisions won")
		}
		winningMinutes = got
	}
	if winningMinutes == 0 || len(f.s.ledger) != 1 || f.s.ledger[0].Granted != int(winningMinutes) {
		t.Fatal("missing or duplicated winning grant")
	}
	for _, deadline := range f.ad.tickets {
		if deadline != fmt.Sprintf("kid@%d", t0+7200) {
			t.Fatal("retry extended the grant")
		}
	}
}

func TestFullLedgerRecoversAfterRetentionAndCannotReplayPrunedUpload(t *testing.T) {
	f := setupFamily(t)
	first := ""
	for i := 0; i < maxLedger; i++ {
		id := fmt.Sprintf("bounded-%04d", i)
		raw, m := f.request(t, id, 1)
		if i == 0 {
			first = raw
		}
		if m["state"] != "pending" {
			t.Fatal(m)
		}
		if r := f.s.AnswerRequest(Answer{RequestID: id}); r["state"] != "denied" {
			t.Fatal(r)
		}
	}
	m := send(t, f.kid, f.s, "request", requestJSON("bounded-full", "fw-kid", "kid", "total", 1, t0))
	if m["error"] != "unavailable" {
		t.Fatal("ledger bound was not enforced", m)
	}
	later := int64(t0 + 90000)
	f.s.Now = func() time.Time { return time.Unix(later, 0) }
	if r := f.s.Requests(); len(r["requests"].([]Reply)) != 0 {
		t.Fatal("retention did not prune the full ledger")
	}
	emptyRestart, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	emptyRestart.Now = func() time.Time { return time.Unix(t0, 0) }
	if m := send(t, f.kid, emptyRestart, "request", first); m["error"] != "unavailable" {
		t.Fatal("rollback replayed a pruned upload into an empty ledger", m)
	}
	m = send(t, f.kid, f.s, "request", requestJSON("bounded-fresh", "fw-kid", "kid", "total", 1, later))
	if m["state"] != "pending" || len(f.s.ledger) != 1 {
		t.Fatal("pruning did not recover a full ledger", m)
	}
	restarted, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = func() time.Time { return time.Unix(t0, 0) }
	if m := send(t, f.kid, restarted, "request", first); m["error"] != "unavailable" {
		t.Fatal("clock rollback replayed a pruned upload", m)
	}
}

func TestFailedExpiryWriteCannotIssueAFreshPolicy(t *testing.T) {
	f := setupFamily(t)
	f.request(t, "expiry-write-001", 10)
	f.s.AnswerRequest(Answer{RequestID: "expiry-write-001", Approve: true, Minutes: 10})
	f.s.Now = func() time.Time { return time.Unix(t0+7200, 0) }
	f.st.failing = true
	if code, _ := f.s.Phone(f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1")); code != StatusError {
		t.Fatal("expiry was acknowledged without being durable")
	}
	if f.s.ledger[0].GrantExpired {
		t.Fatal("failed write changed RAM")
	}
	f.st.failing = false
	m := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	if m["apps"].(map[string]any)["grants"] != nil || !f.s.ledger[0].GrantExpired {
		t.Fatal("expiry retry failed")
	}
}

func TestRequestDoesNotGainAnotherHourInTransit(t *testing.T) {
	f := setupFamily(t)
	raw := requestJSON("transit-001", "fw-kid", "kid", "total", 10, t0-3599)
	if m := send(t, f.kid, f.s, "request", raw); m["state"] != "pending" {
		t.Fatal(m)
	}
	f.s.Now = func() time.Time { return time.Unix(t0+1, 0) }
	if r := f.s.AnswerRequest(Answer{RequestID: "transit-001", Approve: true, Minutes: 10}); r["error"] != "request_expired" {
		t.Fatal("old upload gained a fresh hour", r)
	}
}

func TestGrantDeliveryRequiresTheRequestingPhone(t *testing.T) {
	f := setupFamily(t)
	f.request(t, "delivery-001", 10)
	f.s.AnswerRequest(Answer{RequestID: "delivery-001", Approve: true, Minutes: 10})
	caps := "apps-v1,grants-v1,screen-time-v1"
	m := reply(t, f.s, f.kid.query("fw-kid", true, "-", caps))
	rev := m["apps"].(map[string]any)["rev"].(string)
	sibling := newPhone(t)
	enroll(t, f.s, sibling, "fw-sibling", "kid")
	reply(t, f.s, sibling.query("fw-sibling", true, rev, caps))
	if f.s.Requests()["requests"].([]Reply)[0]["delivered"] != false {
		t.Fatal("sibling falsely acknowledged delivery")
	}
	// A fresh registry after restart still knows the grant and retries its exact ticket.
	restarted, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = f.s.Now
	before := len(f.ad.tickets)
	reply(t, restarted, f.kid.query("fw-kid", true, rev, caps))
	if len(f.ad.tickets) != before+1 || restarted.Requests()["requests"].([]Reply)[0]["delivered"] != true {
		t.Fatal("restart lost delivery or ticket reconciliation")
	}
}

func TestDecodeAdministratorAnswer(t *testing.T) {
	for _, raw := range []string{
		`{"request_id":"admin-001"}`,
		`{"request_id":"admin-001","approve":null}`,
		`{"request_id":"admin-001","approve":true,"approve":false}`,
		`{"request_id":"admin-001","Approve":true}`,
		`{"request_id":"admin-001","approve":true}garbage`,
	} {
		if _, err := DecodeAnswer([]byte(raw)); err == nil {
			t.Fatal("accepted ambiguous administrator answer", raw)
		}
	}
	if a, err := DecodeAnswer([]byte(`{"request_id":"admin-001","approve":false}`)); err != nil || a.Approve {
		t.Fatal(a, err)
	}
}

func TestFailedDecisionWriteDoesNotOpenInternet(t *testing.T) {
	f := setupFamily(t)
	f.request(t, "decision-write-001", 10)
	f.st.failing = true
	if r := f.s.AnswerRequest(Answer{RequestID: "decision-write-001", Approve: true, Minutes: 10}); r["error"] != "unavailable" {
		t.Fatal(r)
	}
	if len(f.ad.tickets) != 0 || f.s.ledger[0].State != "pending" {
		t.Fatal("failed write changed authority")
	}
	f.st.failing = false
	restarted, err := New(f.ad, f.st)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = f.s.Now
	if r := restarted.AnswerRequest(Answer{RequestID: "decision-write-001", Approve: true, Minutes: 10}); r["state"] != "approved" {
		t.Fatal(r)
	}
}

func TestActiveGrantPayloadBound(t *testing.T) {
	f := setupFamily(t)
	for i := 0; i < maxDeviceGrants; i++ {
		id := fmt.Sprintf("active-grant-%04d", i)
		f.request(t, id, 1)
		if r := f.s.AnswerRequest(Answer{RequestID: id, Approve: true, Minutes: 1}); r["state"] != "approved" {
			t.Fatal(r)
		}
	}
	f.request(t, "active-grant-full", 1)
	if r := f.s.AnswerRequest(Answer{RequestID: "active-grant-full", Approve: true, Minutes: 1}); r["error"] != "unavailable" {
		t.Fatal("active grant bound ignored", r)
	}
	m := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,grants-v1,screen-time-v1"))
	if len(m["apps"].(map[string]any)["grants"].([]any)) != maxDeviceGrants {
		t.Fatal("lost an approved grant")
	}
}

func TestPhoneIDCannotCollideWithAdministratorIssuer(t *testing.T) {
	f := setupFamily(t)
	raw, _ := f.request(t, "issuer-001", 10)
	f.s.AnswerRequest(Answer{RequestID: "issuer-001", Approve: true, Minutes: 10})
	alias := newPhone(t)
	enroll(t, f.s, alias, "router", "parent", "kid")
	if m := send(t, alias, f.s, "decision", decisionJSON("issuer-001", sha(raw), "router", true, 10, t0)); m["error"] != "already_answered" {
		t.Fatal("parent phone impersonated a recorded administrator decision", m)
	}
}
