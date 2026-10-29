package identity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/denogio/familywarden/core/internal/store"
	"image"
	"image/png"
	"math/rand"
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

func inventoryIcon(t *testing.T, size int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestInventoryOptionalIconsSanitizedAndShownOnlyFromCurrentBindings(t *testing.T) {
	f := installedFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	icon := inventoryIcon(t, 48)
	for i, optional := range []string{`"` + icon + `"`, `"bad"`, `"` + inventoryIcon(t, 49) + `"`, `"` + base64.StdEncoding.EncodeToString(make([]byte, 2049)) + `"`, `42`, `null`} {
		raw := inventoryJSON("fw-kid", "kid", int64(i+1), 1,
			`[{"package":"com.example.game","label":"Game","system":false,"essential":false,"icon":`+optional+`}]`)
		if m := send(t, f.kid, f.s, "inventory", raw); m["state"] != "stored" {
			t.Fatal(m)
		}
		got, err := f.s.Inventory.Inventories("kid")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range got {
			if (v.Apps[0].Icon != "") != (i == 0) {
				t.Fatal("optional icon not sanitized", i)
			}
		}
	}
	raw := inventoryJSON("fw-kid", "kid", 9, 1, `[{"package":"com.example.game","label":"Game","system":false,"essential":false,"icon":"`+icon+`"}]`)
	send(t, f.kid, f.s, "inventory", raw)
	send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 60, `{"com.example.game":60}`, ""))
	send(t, f.kid, f.s, "request", requestJSON("icon-request-01", "fw-kid", "kid", "com.example.game", 10, t0))
	send(t, f.kid, f.s, "usage", usageWithNewApps(2, `{"com.example.game":"Game"}`))
	for name, r := range map[string]Reply{"usage": f.s.UsageReport(), "requests": f.s.Requests(), "new_apps": f.s.NewApps()} {
		b, _ := json.Marshal(r)
		if bytes.Contains(b, []byte(`"icon"`)) {
			t.Fatal("polled reply repeats icons", name, string(b))
		}
	}
	if m := send(t, f.parent, f.s, "app", appDecisionJSON("fw-parent", "kid", "com.example.game", "approve", t0)); m["state"] != "approved" {
		t.Fatal(m)
	}
	approved, _ := json.Marshal(f.s.InventoryIcons(IconQuery{Profile: "kid", Packages: []string{"com.example.game"}}))
	if !bytes.Contains(approved, []byte(icon)) {
		t.Fatal("approved row lost its icon")
	}
	installed, _ := json.Marshal(send(t, f.parent, f.s, "installed", installedJSON("fw-parent", "kid", "", t0)))
	if bytes.Contains(installed, []byte(`"icon"`)) {
		t.Fatal("icons changed scoped installed reads")
	}
	// Signed parent policy reads are unchanged: icons belong to admin presentation only.
	b, _ := json.Marshal(reply(t, f.s, f.parent.query("fw-parent", true, "-", "apps-v1,screen-time-v1")))
	if bytes.Contains(b, []byte(`"icon"`)) {
		t.Fatal("icons leaked into signed policy")
	}
	ctx := f.ad.ctx
	if f.s.inventoryIcons("kid", ctx)["com.example.game"] != icon {
		t.Fatal("current icon missing")
	}
	for id, d := range f.s.devices {
		if d.ID == "fw-kid" {
			d.Profile = "parent"
			f.s.devices[id] = d
		}
	}
	if len(f.s.inventoryIcons("kid", ctx)) != 0 {
		t.Fatal("moved phone still supplies icons")
	}
	for i := range f.s.devices {
		if f.s.devices[i].ID == "fw-kid" {
			f.s.devices[i].Profile = "kid"
			f.s.devices[i].PubKey = f.other.pub
		}
	}
	if len(f.s.inventoryIcons("kid", ctx)) != 0 {
		t.Fatal("old key still supplies icons")
	}
}

func TestInventoryDropsExcessIconBytesWithoutDroppingApps(t *testing.T) {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	_, _ = rand.New(rand.NewSource(1)).Read(img.Pix)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	icon := base64.StdEncoding.EncodeToString(b.Bytes())
	if !store.ValidInventoryIcon(icon) {
		t.Fatal("invalid fixture")
	}
	apps := []string{}
	for i := 0; i < 20; i++ {
		apps = append(apps, fmt.Sprintf(`{"package":"com.example.app%02d","label":"App","system":false,"essential":false,"icon":"%s"}`, i, icon))
	}
	raw := inventoryJSON("fw-kid", "kid", 1, len(apps), "["+strings.Join(apps, ",")+"]")
	if m := send(t, f.kid, f.s, "inventory", raw); m["state"] != "stored" {
		t.Fatal(m)
	}
	got, err := f.s.Inventory.Inventories("kid")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range got {
		if len(v.Apps) != 20 || !v.Complete() {
			t.Fatal("images displaced apps")
		}
		kept := 0
		for _, a := range v.Apps {
			if a.Icon != "" {
				kept++
			}
		}
		if kept == 0 || kept == 20 || kept*b.Len() > store.MaxInventoryIconsBytes {
			t.Fatal("unbounded or missing icons", kept)
		}
	}
}

func TestInventoryIconsBoundRepeatedHistoricalRequestRows(t *testing.T) {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	icon := inventoryIcon(t, 48)
	send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 1,
		`[{"package":"com.example.game","label":"Game","system":false,"essential":false,"icon":"`+icon+`"}]`))
	f.s.ledger = []entry{}
	for i := 0; i < 200; i++ {
		f.s.ledger = append(f.s.ledger, entry{ID: fmt.Sprintf("request-%03d", i), Profile: "kid", Device: "fw-kid", Target: "com.example.game", State: "denied", Received: t0, DecidedAt: t0, Expires: t0 + 60})
	}
	r := f.s.Requests()
	rows := r["requests"].([]Reply)
	if len(rows) != 200 {
		t.Fatal("icons displaced requests")
	}
	used, count := 0, 0
	for _, row := range rows {
		if value, ok := row["icon"].(string); ok {
			used += len(value)
			count++
		}
	}
	if used != 0 || count != 0 {
		t.Fatal("repeated request icons unbounded", used, count)
	}
}
