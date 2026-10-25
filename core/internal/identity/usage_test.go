package identity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/store"
)

const usageDay = "2027-01-15" // t0 in UTC

func usageJSON(dev, profile, day string, seq int64, total int, apps, grants string) string {
	g := ""
	if grants != "" {
		g = `,"grants_used":` + grants
	}
	return fmt.Sprintf(`{"type":"usage","v":1,"device":"%s","profile":"%s","day":"%s","seq":%d,"total":%d,"apps":%s,"labels":{},"incomplete":false%s}`,
		dev, profile, day, seq, total, apps, g)
}

func usageFamily(t *testing.T) family {
	f := setupFamily(t)
	f.ad.ctx.TimeZone = "UTC"
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.s.Usage = st
	return f
}

func TestUsageReportsAreStoredAndShownOnlyInScope(t *testing.T) {
	f := usageFamily(t)
	raw := usageJSON("fw-kid", "kid", usageDay, 1, 900, `{"com.example.video":600,"com.example.game":300}`, "")
	if m := send(t, f.kid, f.s, "usage", raw); m["state"] != "stored" {
		t.Fatal(m)
	}
	if m := send(t, f.kid, f.s, "usage", raw); m["state"] != "unchanged" {
		t.Fatalf("retry: %v", m)
	}
	pm := reply(t, f.s, f.parent.query("fw-parent", true, "-", "apps-v1,screen-time-v1"))
	usage := pm["usage"].([]any)
	if len(usage) != 1 || usage[0].(map[string]any)["total"] != float64(900) ||
		usage[0].(map[string]any)["apps"].([]any)[0].(map[string]any)["package"] != "com.example.video" {
		t.Fatalf("parent usage: %v", pm["usage"])
	}
	// The child's own phone gets no family usage field; the administrator gets everything.
	if km := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1")); km["usage"] != nil {
		t.Fatal("usage of others leaked to a phone without approval scope")
	}
	r := f.s.UsageReport()
	b, _ := json.Marshal(r)
	var v struct {
		Day      string `json:"day"`
		Profiles []struct {
			ID    string                `json:"id"`
			Total int                   `json:"total"`
			Week  []struct{ Total int } `json:"week"`
		} `json:"profiles"`
	}
	if json.Unmarshal(b, &v) != nil || v.Day != usageDay || len(v.Profiles) != 1 || v.Profiles[0].ID != "kid" ||
		v.Profiles[0].Total != 900 || len(v.Profiles[0].Week) != 7 || v.Profiles[0].Week[6].Total != 900 {
		t.Fatalf("admin usage: %s", b)
	}
	// Parent scope is current, not historical: a pact-bound child is hidden from parents.
	f.ad.ctx.PactCaptain = "kid"
	if pm := reply(t, f.s, f.parent.query("fw-parent", true, "-", "apps-v1,screen-time-v1")); pm["usage"] != nil {
		t.Fatalf("parent still sees a pact-bound profile: %v", pm["usage"])
	}
}

func TestUsageReportsAreAuthenticatedBoundedAndMonotonic(t *testing.T) {
	f := usageFamily(t)
	send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 5, 900, `{"com.example.video":900}`, ""))
	for name, c := range map[string]struct {
		raw, want string
	}{
		"other profile":    {usageJSON("fw-kid", "parent", usageDay, 6, 1, `{}`, ""), "wrong_profile"},
		"future day":       {usageJSON("fw-kid", "kid", "2027-01-16", 6, 1, `{}`, ""), "day_out_of_range"},
		"beyond retention": {usageJSON("fw-kid", "kid", "2026-12-16", 6, 1, `{}`, ""), "day_out_of_range"},
		"lower total":      {usageJSON("fw-kid", "kid", usageDay, 6, 899, `{"com.example.video":900}`, ""), "usage_regressed"},
		"lower app":        {usageJSON("fw-kid", "kid", usageDay, 6, 900, `{"com.example.video":10}`, ""), "usage_regressed"},
		"impossible total": {usageJSON("fw-kid", "kid", usageDay, 6, 25*3600+1, `{}`, ""), "malformed"},
		"unknown grant":    {usageJSON("fw-kid", "kid", usageDay, 6, 900, `{"com.example.video":900}`, `{"`+fmt.Sprintf("%032x", 7)+`":1}`), "unknown_grant"},
		"unknown field":    {strings.Replace(usageJSON("fw-kid", "kid", usageDay, 6, 900, `{}`, ""), `"v":1`, `"v":1,"x":1`, 1), "malformed"},
		"duplicate key":    {`{"type":"usage","v":1,"v":1,"device":"fw-kid","profile":"kid","day":"` + usageDay + `","seq":6,"total":900,"apps":{}}`, "malformed"},
	} {
		if m := send(t, f.kid, f.s, "usage", c.raw); m["error"] != c.want {
			t.Errorf("%s: %v", name, m)
		}
	}
	// A report signed by another phone, or for another phone, is not even answered.
	raw := usageJSON("fw-kid", "kid", usageDay, 7, 2000, `{}`, "")
	if code, body := f.s.App("usage", f.parent.upload("usage", raw)); code != StatusForbidden || body != nil {
		t.Fatal("foreign usage report accepted")
	}
	if code, _ := f.s.App("usage", f.kid.upload("request", raw)); code != StatusForbidden {
		t.Fatal("cross-type signature accepted")
	}
	// Re-keying the phone keeps the old counters but starts a separate record.
	enroll(t, f.s, newPhone(t), "fw-kid", "kid")
	f.ad.ctx.Profiles["parent"] = platformProfile("Parent", "")
	reports, _ := f.s.Usage.UsageDay("kid", usageDay)
	if len(reports) != 1 {
		t.Fatalf("records: %v", reports)
	}
}

func TestReportedGrantUseIsBoundAndShownToTheAdministrator(t *testing.T) {
	f := usageFamily(t)
	f.request(t, "usage-grant-001", 30)
	r := f.s.AnswerRequest(Answer{RequestID: "usage-grant-001", Approve: true, Minutes: 20})
	id := f.s.ledger[0].GrantID
	if r["state"] != "approved" {
		t.Fatal(r)
	}
	if m := send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 60, `{}`, `{"`+id+`":1201}`)); m["error"] != "unknown_grant" {
		t.Fatalf("more than granted: %v", m)
	}
	if m := send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 60, `{}`, `{"`+id+`":600}`)); m["state"] != "stored" {
		t.Fatal(m)
	}
	// A sibling phone cannot report (or erase) another phone's grant.
	sib := newPhone(t)
	enroll(t, f.s, sib, "fw-sib", "kid")
	if m := send(t, sib, f.s, "usage", usageJSON("fw-sib", "kid", usageDay, 1, 60, `{}`, `{"`+id+`":0}`)); m["error"] != "unknown_grant" {
		t.Fatalf("sibling reported grant: %v", m)
	}
	f.s.Now = func() time.Time { return time.Unix(t0+60, 0) }
	if got := f.s.Requests()["requests"].([]Reply)[0]["used"]; got != 600 {
		t.Fatalf("used: %v", got)
	}
}

func TestUsageDisabledWithoutStoreOrAppControl(t *testing.T) {
	f := setupFamily(t)
	if m := send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 1, `{}`, "")); m["error"] != "unavailable" {
		t.Fatal(m)
	}
	if r := f.s.UsageReport(); r["error"] == nil {
		t.Fatal("usage report without a store")
	}
	f = usageFamily(t)
	if m := send(t, f.parent, f.s, "usage", usageJSON("fw-parent", "parent", usageDay, 1, 1, `{}`, "")); m["error"] != "app_control_off" {
		t.Fatal(m)
	}
}
