package identity

import (
	"fmt"
	"strings"
	"testing"
)

func inventoryJSON(dev, profile string, seq int64, installed int, apps string) string {
	return fmt.Sprintf(`{"type":"inventory","v":1,"device":"%s","profile":"%s","seq":%d,"installed":%d,"apps":%s}`,
		dev, profile, seq, installed, apps)
}

func TestInventoryIsAnOwnProfileObservationOnly(t *testing.T) {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	before := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1"))["apps"]
	apps := `[{"package":"com.example.game","label":"Game\u200e\t ","system":false,"essential":false},` +
		`{"package":"com.example.phone","label":"Phone","system":true,"essential":true}]`
	raw := inventoryJSON("fw-kid", "kid", 1, 3, apps)
	if m := send(t, f.kid, f.s, "inventory", raw); m["state"] != "stored" || m["seq"] != float64(1) {
		t.Fatal(m)
	}
	if m := send(t, f.kid, f.s, "inventory", raw); m["state"] != "unchanged" {
		t.Fatal("replay stored again", m)
	}
	got, err := f.s.Inventory.Inventories("kid")
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	for _, v := range got {
		if v.Complete() || v.Installed != 3 || v.Apps[0].Label != "Game" || !v.Apps[1].Essential || v.Name == "" || v.Received != t0 {
			t.Fatal("snapshot not kept honestly", v)
		}
	}
	// A snapshot changes no signed policy and grants nothing.
	if after := reply(t, f.s, f.kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1"))["apps"]; fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatal("inventory changed the signed app policy")
	}
	for name, c := range map[string]struct {
		p     phone
		raw   string
		error string
	}{
		"other profile":   {f.kid, inventoryJSON("fw-kid", "parent", 2, 0, "[]"), "wrong_profile"},
		"parent for kid":  {f.parent, inventoryJSON("fw-parent", "kid", 2, 0, "[]"), "wrong_profile"},
		"app control off": {f.parent, inventoryJSON("fw-parent", "parent", 2, 0, "[]"), "app_control_off"},
		"unknown field":   {f.kid, strings.Replace(inventoryJSON("fw-kid", "kid", 2, 0, "[]"), `"apps"`, `"x":1,"apps"`, 1), "malformed"},
		"unknown app field": {f.kid, inventoryJSON("fw-kid", "kid", 2, 1,
			`[{"package":"com.example.a","label":"A","system":false,"essential":false,"blocked":true}]`), "malformed"},
		"total target": {f.kid, inventoryJSON("fw-kid", "kid", 2, 1,
			`[{"package":"total","label":"A","system":false,"essential":false}]`), "malformed"},
		"unsorted": {f.kid, inventoryJSON("fw-kid", "kid", 2, 2,
			`[{"package":"com.example.b","label":"","system":false,"essential":false},{"package":"com.example.a","label":"","system":false,"essential":false}]`), "malformed"},
		"undercount": {f.kid, inventoryJSON("fw-kid", "kid", 2, 0,
			`[{"package":"com.example.a","label":"","system":false,"essential":false}]`), "malformed"},
	} {
		if m := send(t, c.p, f.s, "inventory", c.raw); m["error"] != c.error {
			t.Fatal(name, m)
		}
	}
	// Another device's signature cannot upload for the child.
	if code, _ := f.parent.sendTo(f.s, "inventory", inventoryJSON("fw-kid", "kid", 3, 0, "[]")); code != StatusForbidden {
		t.Fatal("foreign signature accepted", code)
	}
	f.s.Inventory = nil
	if m := send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 4, 0, "[]")); m["error"] != "unavailable" {
		t.Fatal(m)
	}
}
