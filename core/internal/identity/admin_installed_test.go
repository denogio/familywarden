package identity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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
	for _, q := range []AdminInstalledQuery{{Profile: "unknown"}, {Profile: "kid", Page: -1}, {Profile: "kid", Filter: "bogus"}, {Profile: "kid", Search: strings.Repeat("x", 257)}} {
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
