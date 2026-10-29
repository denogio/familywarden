package store

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func inv(device string, seq int64, pkgs ...string) Inventory {
	v := Inventory{Version: InventoryVersion, Device: device, Seq: seq, Installed: len(pkgs), Received: 1800000000, Apps: []InventoryApp{}}
	for _, p := range pkgs {
		v.Apps = append(v.Apps, InventoryApp{Package: p, Label: "App"})
	}
	return v
}

func TestInventoryKeepsNewestSnapshotPerPhoneAndReplacesOldKeysAndProfiles(t *testing.T) {
	s, dir := open(t)
	before, err := s.Generation()
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ApplyInventory("alex", inv("fw-alex@0123456789abcdef", 2, "com.example.a", "com.example.b")); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := s.ApplyInventory("alex", inv("fw-alex@0123456789abcdef", 2, "com.example.c")); ok || err != nil {
		t.Fatal("replayed seq replaced the snapshot", ok, err)
	}
	if ok, _ := s.ApplyInventory("alex", inv("fw-alex@0123456789abcdef", 1)); ok {
		t.Fatal("older seq replaced the snapshot")
	}
	got, err := s.Inventories("alex")
	if err != nil || len(got) != 1 || len(got["fw-alex@0123456789abcdef"].Apps) != 2 {
		t.Fatal(got, err)
	}
	// A re-keyed phone, then a reassigned one, leave exactly one snapshot.
	if ok, err := s.ApplyInventory("alex", inv("fw-alex@fedcba9876543210", 1, "com.example.a")); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := s.ApplyInventory("sam", inv("fw-alex@fedcba9876543210", 2)); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got, err := s.Inventories("alex"); err != nil || len(got) != 0 {
		t.Fatal("old profile kept a moved phone", got, err)
	}
	if got, err := s.Inventories("sam"); err != nil || len(got) != 1 || !got["fw-alex@fedcba9876543210"].Complete() {
		t.Fatal("reported empty snapshot not kept as complete", got, err)
	}
	if err := s.CheckPresence(); err != nil {
		t.Fatal(err)
	}
	after, err := s.Generation()
	if err != nil || after.Epoch != before.Epoch || after.Counter != before.Counter {
		t.Fatal("observational inventory advanced the household generation", before, after, err)
	}
	// Losing a witnessed snapshot is detected at startup, not read as no report.
	if err := os.Remove(filepath.Join(dir, "inventory", "sam", "fw-alex@fedcba9876543210.json")); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckPresence(); err == nil {
		t.Fatal("lost inventory not detected")
	}
}

func TestInventoryRejectsUnboundedUnsortedOrInconsistentSnapshots(t *testing.T) {
	s, _ := open(t)
	partial := inv("fw-alex@0123456789abcdef", 1, "com.example.a")
	partial.Installed = 3
	if ok, err := s.ApplyInventory("alex", partial); !ok || err != nil || partial.Complete() {
		t.Fatal("honest partial snapshot refused", ok, err)
	}
	many := make([]string, MaxInventoryApps+1)
	for i := range many {
		many[i] = "com.example.a" + strings.Repeat("a", i/26) + string(rune('a'+i%26))
	}
	for name, v := range map[string]Inventory{
		"unsorted":  inv("fw-alex@0123456789abcdef", 5, "com.example.b", "com.example.a"),
		"duplicate": inv("fw-alex@0123456789abcdef", 5, "com.example.a", "com.example.a"),
		"package":   inv("fw-alex@0123456789abcdef", 5, "not a package"),
		"device":    inv("../alex", 5),
		"too many":  inv("fw-alex@0123456789abcdef", 5, many...),
		"undercount": func() Inventory {
			v := inv("fw-alex@0123456789abcdef", 5, "com.example.a")
			v.Installed = 0
			return v
		}(),
		"label": func() Inventory {
			v := inv("fw-alex@0123456789abcdef", 5, "com.example.a")
			v.Apps[0].Label = strings.Repeat("x", 65)
			return v
		}(),
	} {
		if ok, err := s.ApplyInventory("alex", v); ok || err == nil {
			t.Fatal("accepted", name)
		}
	}
	if _, err := s.ApplyInventory("../x", inv("fw-alex@0123456789abcdef", 6)); err == nil {
		t.Fatal("profile path accepted")
	}
}

func testIcon(t *testing.T, size int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestInventoryPNGValidationAndPersistence(t *testing.T) {
	icon := testIcon(t, 48)
	raw, _ := base64.StdEncoding.DecodeString(icon)
	for name, value := range map[string]string{
		"base64": "!!!", "svg": base64.StdEncoding.EncodeToString([]byte(`<svg/>`)),
		"dimensions": testIcon(t, 49), "truncated": base64.StdEncoding.EncodeToString(raw[:len(raw)-1]),
		"trailing":  base64.StdEncoding.EncodeToString(append(raw, 'x')),
		"oversized": base64.StdEncoding.EncodeToString(make([]byte, MaxInventoryIconBytes+1)),
	} {
		if ValidInventoryIcon(value) {
			t.Fatal("accepted", name)
		}
	}
	if !ValidInventoryIcon(icon) {
		t.Fatal("valid PNG rejected")
	}
	s, dir := open(t)
	v := inv("fw-alex@0123456789abcdef", 1, "com.example.game")
	v.Apps[0].Icon = icon
	if _, err := s.ApplyInventory("alex", v); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Inventories("alex")
	if err != nil || got[v.Device].Apps[0].Icon != icon {
		t.Fatal("icon lost on reopen", err)
	}
	v.Seq++
	v.Apps[0].Icon = "invalid"
	if _, err := s.ApplyInventory("alex", v); err == nil {
		t.Fatal("invalid stored PNG accepted")
	}
	v.Apps[0].Icon = ""
	if _, err := s.ApplyInventory("alex", v); err != nil {
		t.Fatal("legacy snapshot rejected", err)
	}
}

func TestInventoryStoreRejectsExcessTotalIconBytes(t *testing.T) {
	s, _ := open(t)
	v := inv("fw-alex@0123456789abcdef", 1)
	icon := testIcon(t, 48)
	for i := 0; i < MaxInventoryApps; i++ {
		v.Apps = append(v.Apps, InventoryApp{Package: fmt.Sprintf("com.example.app%03d", i), Icon: icon})
	}
	v.Installed = len(v.Apps)
	if _, err := s.ApplyInventory("alex", v); err == nil {
		t.Fatal("accepted unbounded total icons")
	}
}
