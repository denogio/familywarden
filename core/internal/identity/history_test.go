package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/store"
)

func historyJSON(device, profile, day string, created int64) string {
	return fmt.Sprintf(`{"type":"history","v":1,"device":%q,"profile":%q,"day":%q,"created":%d,"nonce":%q}`, device, profile, day, created, strings.Repeat("a", 64))
}

func TestHistoryIsAReadOfRetainedReportsNotMissingZeros(t *testing.T) {
	f := usageFamily(t)
	send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 0, `{}`, ""))
	before, _ := json.Marshal(f.st.docs)
	f.st.failing = true // historical reads must not need durable mutation
	for _, device := range []struct {
		id  string
		key phone
	}{{"fw-parent", f.parent}, {"fw-kid", f.kid}} {
		m := send(t, device.key, f.s, "history", historyJSON(device.id, "kid", usageDay, t0))
		if m["reported"] != true || m["total"] != float64(0) || len(m["devices"].([]any)) != 1 || m["type"] != "history_reply" {
			t.Fatal(m)
		}
		if m["devices"].([]any)[0].(map[string]any)["received"] != float64(t0) {
			t.Fatal(m)
		}
		missing := send(t, device.key, f.s, "history", historyJSON(device.id, "kid", "2027-01-14", t0))
		if missing["reported"] != false || len(missing["devices"].([]any)) != 0 || len(missing["apps"].([]any)) != 0 {
			t.Fatal(missing)
		}
	}
	after, _ := json.Marshal(f.st.docs)
	if string(before) != string(after) || len(f.ad.tickets) != 0 {
		t.Fatal("read mutated authority")
	}
}

func TestHistoryUsesCurrentScopeKeyAndPactProtection(t *testing.T) {
	f := usageFamily(t)
	raw := historyJSON("fw-parent", "kid", usageDay, t0)
	if m := send(t, f.parent, f.s, "history", raw); m["error"] != nil {
		t.Fatal(m)
	}
	f.ad.ctx.PactCaptain = "kid"
	if m := send(t, f.parent, f.s, "history", raw); m["error"] != "not_parent" {
		t.Fatal(m)
	}
	f.ad.ctx.PactCaptain = ""
	for i := range f.s.devices {
		if f.s.devices[i].ID == "fw-parent" {
			f.s.devices[i].Approves = nil
			f.s.devices[i].Administrator = true
		}
	}
	if m := send(t, f.parent, f.s, "history", raw); m["error"] != "not_parent" {
		t.Fatal("admin implies parent", m)
	}
	if m := send(t, f.kid, f.s, "history", historyJSON("fw-kid", "adult", usageDay, t0)); m["error"] != "not_parent" {
		t.Fatal(m)
	}
	enroll(t, f.s, newPhone(t), "fw-parent", "parent", "kid")
	if code, _ := f.s.App("history", f.parent.upload("history", raw)); code != StatusForbidden {
		t.Fatal("old key read history", code)
	}
	if code, _ := f.s.App("history", f.kid.upload("usage", historyJSON("fw-kid", "kid", usageDay, t0))); code != StatusForbidden {
		t.Fatal("cross-domain read", code)
	}
}

func TestHistoryCalendarBoundsFollowRouterZoneAcrossDST(t *testing.T) {
	f := usageFamily(t)
	loc, err := time.LoadLocation("Europe/Copenhagen")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 0, 30, 0, 0, loc)
	f.ad.ctx.TimeZone = loc.String()
	f.s.Now = func() time.Time { return now }
	for _, day := range []string{"2026-10-26", "2026-09-27"} {
		if r := send(t, f.parent, f.s, "history", historyJSON("fw-parent", "kid", day, now.Unix())); r["error"] != nil {
			t.Fatal(r)
		}
	}
	for _, day := range []string{"2026-10-27", "2026-09-26"} {
		if r := send(t, f.parent, f.s, "history", historyJSON("fw-parent", "kid", day, now.Unix())); r["error"] != "day_out_of_range" {
			t.Fatal(r)
		}
	}
}

type brokenHistory struct{ UsageStore }

func (brokenHistory) UsageDay(string, string) (map[string]store.Report, error) {
	return nil, errors.New("unavailable")
}

func TestHistoryBoundsStrictDecodingAndErrors(t *testing.T) {
	f := usageFamily(t)
	raw := historyJSON("fw-parent", "kid", usageDay, t0)
	for _, tc := range []struct{ raw, want string }{
		{historyJSON("fw-parent", "kid", "2027-01-16", t0), "day_out_of_range"},
		{historyJSON("fw-parent", "kid", "2026-12-16", t0), "day_out_of_range"},
		{historyJSON("fw-parent", "kid", "2027-1-15", t0), "day_out_of_range"},
		{historyJSON("fw-parent", "kid", usageDay, t0-300), "request_expired"},
		{historyJSON("fw-parent", "kid", usageDay, t0+301), "request_expired"},
		{strings.Replace(raw, `"v":1`, `"v":1,"v":1`, 1), "malformed"},
		{strings.Replace(raw, `"v":1`, `"v":1,"method":"configuration_apply"`, 1), "malformed"},
		{strings.Replace(raw, strings.Repeat("a", 64), "no-nonce", 1), "malformed"},
	} {
		if m := send(t, f.parent, f.s, "history", tc.raw); m["error"] != tc.want {
			t.Fatalf("want %s: %v", tc.want, m)
		}
	}
	// The inclusive retention edge uses calendar days.
	f.s.Now = func() time.Time { return time.Unix(t0, 0) }
	if m := send(t, f.parent, f.s, "history", historyJSON("fw-parent", "kid", "2026-12-17", t0)); m["error"] != nil {
		t.Fatal(m)
	}
	f.s.Usage = brokenHistory{f.s.Usage}
	if m := send(t, f.parent, f.s, "history", raw); m["error"] != "unavailable" || m["reported"] != nil {
		t.Fatal("storage failure became empty history", m)
	}
	f.s.Usage = nil
	if m := send(t, f.parent, f.s, "history", raw); m["error"] != "unavailable" {
		t.Fatal(m)
	}
}
