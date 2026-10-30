package identity

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/rules"
)

func adminInventoryJSON(t *testing.T, f family, q AdminInstalledQuery) map[string]any {
	t.Helper()
	b, err := json.Marshal(f.s.AdminInstalled(q))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAdminInstalledSearchCountsNumberedPagesAndMissingUsage(t *testing.T) {
	f := installedFamily(t)
	var items []string
	for i := 0; i < 45; i++ {
		items = append(items, fmt.Sprintf(`{"package":"com.example.a%03d","label":"Alex app %03d","system":false,"essential":false}`, i, i))
	}
	send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 50, "["+strings.Join(items, ",")+"]"))
	q := AdminInstalledQuery{Profile: "kid"}
	first := adminInventoryJSON(t, f, q)
	if len(first["apps"].([]any)) != 20 || first["pages"] != float64(3) || first["total"] != float64(49) {
		t.Fatal(first)
	}
	counts := first["counts"].(map[string]any)
	if counts["all"] != float64(49) || counts["limit"] != float64(2) || counts["unlimited"] != float64(1) || counts["blocked"] != float64(1) {
		t.Fatal(counts)
	}
	src := first["sources"].([]any)[0].(map[string]any)
	if src["installed"] != float64(50) || src["sent"] != float64(45) {
		t.Fatal(src)
	}
	q.Page = 2
	last := adminInventoryJSON(t, f, q)
	if len(last["apps"].([]any)) != 9 || last["start"] != float64(40) || last["snapshot"] != first["snapshot"] {
		t.Fatal(last)
	}
	for _, a := range last["apps"].([]any) {
		if _, exists := a.(map[string]any)["seconds"]; exists {
			t.Fatal("missing usage fabricated", a)
		}
	}
	q.Page = 0
	q.Search = "ALEX APP 042"
	found := adminInventoryJSON(t, f, q)
	if found["total"] != float64(1) || found["apps"].([]any)[0].(map[string]any)["package"] != "com.example.a042" {
		t.Fatal(found)
	}
	q.Search = "EXAMPLE.A044"
	if m := adminInventoryJSON(t, f, q); m["total"] != float64(1) {
		t.Fatal(m)
	}
	q.Search = ""
	q.Filter = "blocked"
	if m := adminInventoryJSON(t, f, q); m["total"] != float64(1) || m["counts"].(map[string]any)["all"] != float64(49) {
		t.Fatal(m)
	}
	// Registry removal hides observations, while manually configured rules remain editable.
	f.s.devices = nil
	q.Filter = "all"
	m := adminInventoryJSON(t, f, q)
	if m["total"] != float64(4) || len(m["sources"].([]any)) != 0 || m["snapshot"] == first["snapshot"] {
		t.Fatal(m)
	}
}

func TestAdminInstalledRefusalMissingAndCaptainRead(t *testing.T) {
	f := installedFamily(t)
	q := AdminInstalledQuery{Profile: "kid"}
	m := adminInventoryJSON(t, f, q)
	if m["sources"].([]any)[0].(map[string]any)["reported"] != false {
		t.Fatal(m)
	}
	f.ad.ctx.PactCaptain = "kid"
	if m := adminInventoryJSON(t, f, q); m["error"] != nil {
		t.Fatal("admin observational read must not become pact mutation", m)
	}
	for _, q := range []AdminInstalledQuery{{Profile: "unknown"}, {Profile: "kid", Page: -1}, {Profile: "kid", Filter: "bogus"}, {Profile: "kid", Sort: "bogus"}, {Profile: "kid", Search: strings.Repeat("x", 257)}} {
		if m := adminInventoryJSON(t, f, q); m["error"] == nil {
			t.Fatal(q, m)
		}
	}
	f.s.Inventory = nil
	if m := adminInventoryJSON(t, f, AdminInstalledQuery{Profile: "kid"}); m["error"] == nil {
		t.Fatal(m)
	}
	if _, err := DecodeAdminInstalledQuery([]byte(`{"profile":"kid","surprise":true}`)); err == nil {
		t.Fatal("unknown query fields accepted")
	}
}

func TestAdminInstalledPendingAndUsageRemainCurrentRegistryScoped(t *testing.T) {
	f := installedFamily(t)
	usage := `{"type":"usage","v":1,"device":"fw-kid","profile":"kid","day":"2026-10-21","seq":1,"total":120,"apps":{"com.example.video":120},"new_apps":{"com.example.fresh":"Alex new app"}}`
	// Use the fixture's actual router-local day rather than a wall-clock default.
	day := f.s.Now().In(routerZone(f.ad.ctx)).Format("2006-01-02")
	usage = strings.Replace(usage, "2026-10-21", day, 1)
	if m := send(t, f.kid, f.s, "usage", usage); m["state"] != "stored" {
		t.Fatal(m)
	}
	q := AdminInstalledQuery{Profile: "kid"}
	m := adminInventoryJSON(t, f, q)
	if m["counts"].(map[string]any)["pending"] != float64(1) {
		t.Fatal(m)
	}
	apps := appsByPackage(m)
	if apps["com.example.video"]["seconds"] != float64(120) || apps["com.example.fresh"]["rule"] != "pending" {
		t.Fatal(apps)
	}
	q.Filter = "limit"
	m = adminInventoryJSON(t, f, q)
	if m["total"] != float64(2) || m["counts"].(map[string]any)["all"] != float64(5) {
		t.Fatal(m)
	}
	f.s.devices = nil
	q.Filter = "all"
	m = adminInventoryJSON(t, f, q)
	if m["total"] != float64(4) {
		t.Fatal("removed registry pending observation leaked", m)
	}
	if a := appsByPackage(m)["com.example.video"]; a["seconds"] != nil {
		t.Fatal("removed key usage leaked", a)
	}
}

func TestDecodeAdminInstalledSort(t *testing.T) {
	for _, value := range []string{"", "used", "name", "rule"} {
		q, err := DecodeAdminInstalledQuery([]byte(fmt.Sprintf(`{"profile":"kid","sort":%q}`, value)))
		if err != nil || q.Sort != value {
			t.Fatalf("sort %q: %+v %v", value, q, err)
		}
	}
	for _, raw := range []string{`{"profile":"kid","sort":1}`, `{"profile":"kid","sort":null}`, `{"profile":"kid","sort":"name","extra":true}`} {
		if _, err := DecodeAdminInstalledQuery([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestAdminInstalledSortOrdering(t *testing.T) {
	f := installedFamily(t)
	f.ad.ctx.Profiles["kid"].AppInput.Apps = append(f.ad.ctx.Profiles["kid"].AppInput.Apps,
		rules.AppSetting{Package: "com.example.approved"})
	// Deliberately reverse package and label order, including case-insensitive ties.
	items := `[{"package":"com.example.video","label":"Zulu","system":false,"essential":false},
 {"package":"com.example.chat","label":"beta","system":false,"essential":false},
 {"package":"com.example.music","label":"Beta","system":false,"essential":false},
 {"package":"com.example.approved","label":"Gamma","system":false,"essential":false},
 {"package":"com.example.zero","label":"Aardvark","system":false,"essential":false},
 {"package":"com.example.none","label":"Alpha","system":false,"essential":false}]`
	var observed []inventoryApp
	if err := json.Unmarshal([]byte(items), &observed); err != nil {
		t.Fatal(err)
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].Package < observed[j].Package })
	encoded, _ := json.Marshal(observed)
	if m := send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 6, string(encoded))); m["state"] != "stored" {
		t.Fatal(m)
	}
	check := func(mode string, want []string) {
		t.Helper()
		m := adminInventoryJSON(t, f, AdminInstalledQuery{Profile: "kid", Sort: mode})
		var got []string
		for _, raw := range m["apps"].([]any) {
			got = append(got, strings.TrimPrefix(raw.(map[string]any)["package"].(string), "com.example."))
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("sort %q: got %v want %v", mode, got, want)
		}
	}
	name := []string{"zero", "none", "chat", "music", "unreported", "approved", "video"}
	check("", name)
	check("used", name)
	check("name", name)
	day := f.s.Now().In(routerZone(f.ad.ctx)).Format("2006-01-02")
	usage := fmt.Sprintf(`{"type":"usage","v":1,"device":"fw-kid","profile":"kid","day":%q,"seq":1,"total":250,"apps":{"com.example.video":120,"com.example.chat":120,"com.example.music":10,"com.example.zero":0},"new_apps":{"com.example.fresh":"Alex pending"}}`, day)
	if m := send(t, f.kid, f.s, "usage", usage); m["state"] != "stored" {
		t.Fatal(m)
	}
	check("used", []string{"chat", "video", "music", "zero", "fresh", "none", "unreported", "approved"})
	check("", []string{"chat", "video", "music", "zero", "fresh", "none", "unreported", "approved"})
	check("name", []string{"zero", "fresh", "none", "chat", "music", "unreported", "approved", "video"})
	check("rule", []string{"chat", "unreported", "video", "music", "fresh", "approved", "zero", "none"})
}

func TestAdminInstalledSortBeforeNumberedPages(t *testing.T) {
	f := installedFamily(t)
	var items []string
	for i := 0; i < 25; i++ {
		items = append(items, fmt.Sprintf(`{"package":"com.example.sort%02d","label":"Alex sorted %02d","system":false,"essential":false}`, i, 24-i))
	}
	if m := send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 25, "["+strings.Join(items, ",")+"]")); m["state"] != "stored" {
		t.Fatal(m)
	}
	for _, mode := range []string{"", "used", "name", "rule"} {
		for page := 0; page < 2; page++ {
			m := adminInventoryJSON(t, f, AdminInstalledQuery{Profile: "kid", Search: "Alex sorted", Sort: mode, Page: page})
			if m["pages"] != float64(2) || m["total"] != float64(25) || m["start"] != float64(page*20) {
				t.Fatal(m)
			}
			for i, raw := range m["apps"].([]any) {
				want := fmt.Sprintf("com.example.sort%02d", 24-page*20-i)
				if raw.(map[string]any)["package"] != want {
					t.Fatalf("sort %q page %d item %d: want %s got %v", mode, page, i, want, raw)
				}
			}
		}
	}
}
