package identity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

func TestInventoryIconBatchesAndAdminPagesCover200Apps(t *testing.T) {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	rand.New(rand.NewSource(4)).Read(img.Pix)
	var b bytes.Buffer
	png.Encode(&b, img)
	icon := base64.StdEncoding.EncodeToString(b.Bytes())
	apps := []string{}
	packages := []string{}
	for i := 0; i < 200; i++ {
		pkg := fmt.Sprintf("com.example.app%03d", i)
		packages = append(packages, pkg)
		apps = append(apps, fmt.Sprintf(`{"package":%q,"label":"App","system":false,"essential":false}`, pkg))
	}
	if m := send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 200, "["+strings.Join(apps, ",")+"]")); m["state"] != "stored" {
		t.Fatal(m)
	}
	upload := func(pkg, profile string, seq int64) string {
		raw, _ := json.Marshal(map[string]any{"type": "inventory_icons", "v": 1, "device": "fw-kid", "profile": profile, "inventory_seq": 1, "seq": seq, "icons": []map[string]string{{"package": pkg, "icon": icon}}})
		return string(raw)
	}
	for _, pkg := range packages {
		raw := upload(pkg, "kid", 1)
		if len(raw) > maxIconJSON {
			t.Fatal("fixture exceeds bound")
		}
		if m := send(t, f.kid, f.s, "inventory_icons", raw); m["state"] != "stored" {
			t.Fatal(m)
		}
	}
	if m := send(t, f.kid, f.s, "inventory_icons", upload(packages[0], "kid", 1)); m["state"] != "unchanged" {
		t.Fatal("replay", m)
	}
	if m := send(t, f.kid, f.s, "inventory_icons", upload(packages[0], "parent", 2)); m["error"] != "wrong_profile" {
		t.Fatal(m)
	}
	if m := send(t, f.kid, f.s, "inventory_icons", upload("com.example.missing", "kid", 2)); m["error"] != "malformed" {
		t.Fatal(m)
	}
	if code, _ := f.parent.sendTo(f.s, "inventory_icons", upload(packages[0], "kid", 2)); code != StatusForbidden {
		t.Fatal("foreign signature accepted")
	}
	count := 0
	for start := 0; start < len(packages); start += 32 {
		end := min(start+32, len(packages))
		requested := packages[start:end]
		for offset := 0; offset < len(requested); {
			r := f.s.InventoryIcons(IconQuery{"kid", requested, offset})
			raw, _ := json.Marshal(r)
			if len(raw) > maxIconJSON {
				t.Fatal("oversized admin page")
			}
			next := r["next"].(int)
			if next <= offset {
				t.Fatal("pagination stalled")
			}
			icons := r["icons"].(map[string]string)
			for _, pkg := range requested[offset:next] {
				if icons[pkg] != icon {
					t.Fatal("missing later icon", pkg)
				}
				count++
			}
			offset = next
		}
	}
	bounded := upload(packages[0], "kid", 2)
	bounded += strings.Repeat(" ", maxIconJSON-len(bounded))
	if m := send(t, f.kid, f.s, "inventory_icons", bounded); m["state"] != "stored" {
		t.Fatal("exact JSON bound refused", m)
	}
	if m := send(t, f.kid, f.s, "inventory_icons", bounded+" "); m["error"] != "malformed" {
		t.Fatal("oversized JSON accepted", m)
	}
	for _, raw := range []string{
		strings.Replace(upload(packages[0], "kid", 3), `"seq":3`, `"seq":3,"extra":true`, 1),
		strings.Replace(upload(packages[0], "kid", 3), `"icon":`, `"unknown":true,"icon":`, 1),
		strings.Replace(upload(packages[0], "kid", 3), icon, "invalid", 1),
	} {
		if m := send(t, f.kid, f.s, "inventory_icons", raw); m["error"] != "malformed" {
			t.Fatal("invalid batch accepted", m)
		}
	}
	if count != 200 {
		t.Fatal("incomplete", count)
	}
	r := f.s.InventoryIcons(IconQuery{"kid", []string{"com.example.absent", packages[199]}, 0})
	if r["next"] != 2 || r["icons"].(map[string]string)[packages[199]] != icon {
		t.Fatal("missing image stopped progress")
	}
	if f.s.InventoryIcons(IconQuery{"kid", packages, 0})["error"] == nil {
		t.Fatal("unbounded query")
	}
	for i, d := range f.s.devices {
		if d.ID == "fw-kid" {
			f.s.devices[i].PubKey = f.other.pub
		}
	}
	if len(f.s.InventoryIcons(IconQuery{"kid", packages[:1], 0})["icons"].(map[string]string)) != 0 {
		t.Fatal("revoked key leaked")
	}
}

func TestInventoryIconsDoNotReviveAfterBindingReturns(t *testing.T) {
	f := usageFamily(t)
	f.s.Inventory = f.s.Usage.(InventoryStore)
	icon := inventoryIcon(t, 48)
	send(t, f.kid, f.s, "inventory", inventoryJSON("fw-kid", "kid", 1, 1, `[{"package":"com.example.game","label":"Game","system":false,"essential":false,"icon":"`+icon+`"}]`))
	for _, profile := range []string{"parent", "kid"} {
		if r := f.s.Update(Change{Device: "fw-kid", Profile: profile}); r["error"] != nil {
			t.Fatal(r)
		}
	}
	r := f.s.InventoryIcons(IconQuery{"kid", []string{"com.example.game"}, 0})
	if len(r["icons"].(map[string]string)) != 0 {
		t.Fatal("old binding revived", r)
	}
}
