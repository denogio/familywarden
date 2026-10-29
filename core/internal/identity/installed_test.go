package identity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/rules"
)

func installedJSON(device, profile, after string, created int64) string {
	return fmt.Sprintf(`{"type":"installed","v":1,"device":%q,"profile":%q,"after":%q,"created":%d,"nonce":%q}`,
		device, profile, after, created, strings.Repeat("b", 64))
}

func installedFamily(t *testing.T) family {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	kid := f.ad.ctx.Profiles["kid"]
	kid.AppInput = &rules.AppConfig{Version: 1, Enabled: "1", SuspendNew: "1", AlwaysAllowed: []string{"com.example.music"},
		Apps: []rules.AppSetting{{Package: "com.example.video", Minutes: "30"}, {Package: "com.example.chat", Blocked: "1"},
			{Package: "com.example.unreported", Minutes: "0"}}}
	f.ad.ctx.Profiles["kid"] = kid
	return f
}

func appsByPackage(m map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, a := range m["apps"].([]any) {
		out[a.(map[string]any)["package"].(string)] = a.(map[string]any)
	}
	return out
}

func TestInstalledJoinsCurrentReportsWithCompiledRules(t *testing.T) {
	f := installedFamily(t)
	m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0))
	sources := m["sources"].([]any)
	if len(sources) != 1 || sources[0].(map[string]any)["reported"] != false {
		t.Fatal("a phone without a report is not shown as missing", m)
	}
	// Configured apps are listed with their rule but no reporting phone.
	if a := appsByPackage(m)["com.example.unreported"]; a == nil || len(a["on"].([]any)) != 0 || a["rule"] != "limit" || a["minutes"] != float64(0) {
		t.Fatal("zero-minute limit not kept distinct from block", m)
	}
	apps := `[{"package":"com.android.dialer","label":"Phone","system":true,"essential":true},` +
		`{"package":"com.example.chat","label":"Chat","system":false,"essential":false},` +
		`{"package":"com.example.game","label":"Game","system":false,"essential":false},` +
		`{"package":"com.example.music","label":"Music","system":false,"essential":false},` +
		`{"package":"com.example.video","label":"Video","system":false,"essential":false}]`
	send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 6, apps))
	before, _ := json.Marshal(f.st.docs)
	f.st.failing = true // reading must not need durable mutation
	m = send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0))
	src := m["sources"].([]any)[0].(map[string]any)
	if src["reported"] != true || src["installed"] != float64(6) || src["sent"] != float64(5) || src["received"] != float64(t0) {
		t.Fatal("partial snapshot provenance lost", src)
	}
	got := appsByPackage(m)
	for pkg, want := range map[string]string{"com.example.chat": "blocked", "com.example.music": "always",
		"com.example.video": "limit", "com.example.game": "none", "com.android.dialer": "none"} {
		if got[pkg]["rule"] != want || len(got[pkg]["on"].([]any)) != 1 {
			t.Fatal(pkg, got[pkg])
		}
	}
	if len(got["com.android.dialer"]["essential"].([]any)) != 1 || got["com.example.video"]["label"] != "Video" ||
		m["suspend_new"] != true || m["apps_rev"] != "0123456789abcdef" || m["more"] != false {
		t.Fatal(m)
	}
	after, _ := json.Marshal(f.st.docs)
	if string(before) != string(after) || len(f.ad.tickets) != 0 {
		t.Fatal("read mutated authority")
	}
	// The child may read its own list; nobody else's.
	f.st.failing = false
	if m := send(t, f.kid, f.s, "installed", installedJSON("fw-kid", "kid", "", t0)); m["error"] != nil {
		t.Fatal(m)
	}
	if m := send(t, f.kid, f.s, "installed", installedJSON("fw-kid", "parent", "", t0)); m["error"] != "not_parent" {
		t.Fatal(m)
	}
}

func TestInstalledUsesCurrentScopeKeysAndPagesWithoutGaps(t *testing.T) {
	f := installedFamily(t)
	var items []string
	for i := 0; i < 450; i++ {
		items = append(items, fmt.Sprintf(`{"package":"com.example.a%03dxxxxxxxxx","label":%q,"system":false,"essential":false}`,
			i, strings.Repeat("L", 50)))
	}
	// Larger than ordinary 16 KiB actions, within the 64 KiB transport.
	raw := inventoryJSON("fw-kid", "kid", 1, 450, "["+strings.Join(items, ",")+"]")
	if len(raw) <= 16<<10 || len(raw) > 60<<10 {
		t.Fatal("fixture size", len(raw))
	}
	if m := send(t, f.kid, f.s, "inventory", raw); m["state"] != "stored" {
		t.Fatal(m)
	}
	seen, after, snapshot, pages := map[string]bool{}, "", "", 0
	for page := 0; ; page++ {
		pages++
		m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", after, t0))
		if snapshot == "" {
			snapshot = m["snapshot"].(string)
		} else if m["snapshot"] != snapshot {
			t.Fatal("unchanged data changed snapshot id")
		}
		for _, a := range m["apps"].([]any) {
			pkg := a.(map[string]any)["package"].(string)
			if seen[pkg] || pkg <= after {
				t.Fatal("page overlap or order", pkg)
			}
			seen[pkg], after = true, pkg
		}
		if m["more"] == false {
			break
		}
		if page > 10 {
			t.Fatal("paging does not terminate")
		}
	}
	if len(seen) != 454 || pages < 2 { // 450 reported + four configured-only
		t.Fatal("pages lost apps", len(seen), pages)
	}
	// A newer report changes the snapshot id, so a client restarts instead of mixing.
	send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 2, 0, "[]"))
	if m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0)); m["snapshot"] == snapshot {
		t.Fatal("snapshot id ignores a new report")
	}
	// Revoked scope, pact protection, stale requests and app control off all refuse.
	f.ad.ctx.PactCaptain = "kid"
	if m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0)); m["error"] != "not_parent" {
		t.Fatal(m)
	}
	f.ad.ctx.PactCaptain = ""
	if m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0-600)); m["error"] != "request_expired" {
		t.Fatal(m)
	}
	if m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "total", t0)); m["error"] != "malformed" {
		t.Fatal(m)
	}
	for i := range f.s.devices {
		if f.s.devices[i].ID == "fw-parent" {
			f.s.devices[i].Approves = nil
		}
	}
	if m := send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0)); m["error"] != "not_parent" {
		t.Fatal(m)
	}
	// A re-keyed child phone's old snapshot is no current source.
	for i := range f.s.devices {
		if f.s.devices[i].ID == "fw-kid" {
			f.s.devices[i].PubKey = f.other.pub
		}
	}
	if m := send(t, f.other, f.s, "installed", installedJSON("fw-kid", "kid", "", t0)); m["sources"].([]any)[0].(map[string]any)["reported"] != false {
		t.Fatal("old key's snapshot used", m)
	}
	kid := f.ad.ctx.Profiles["kid"]
	kid.AppsRev = ""
	f.ad.ctx.Profiles["kid"] = kid
	if m := send(t, f.other, f.s, "installed", installedJSON("fw-kid", "kid", "", t0)); m["error"] != "app_control_off" {
		t.Fatal(m)
	}
}
