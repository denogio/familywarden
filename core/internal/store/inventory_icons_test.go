package store

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func noisyIcon(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	rand.New(rand.NewSource(7)).Read(img.Pix)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if b.Len() > MaxAppIconBytes {
		t.Fatal("fixture exceeds budget")
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestAppIconsAll512PersistAcrossMetadataAndRestart(t *testing.T) {
	s, dir := open(t)
	device := "fw-alex@0123456789abcdef"
	icon := noisyIcon(t)
	pkgs := []string{}
	for i := 0; i < 512; i++ {
		pkgs = append(pkgs, fmt.Sprintf("com.example.app%03d", i))
	}
	if _, err := s.ApplyInventory("alex", inv(device, 1, pkgs...)); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if ok, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, icon); err != nil || !ok {
			t.Fatal(pkg, ok, err)
		}
	}
	if _, err := s.ApplyInventory("alex", inv(device, 2, pkgs...)); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.AppIcons("alex", device, nil)
	if err != nil || len(got) != 512 || got[pkgs[511]] != icon {
		t.Fatal("restart coverage", len(got), err)
	}
	if _, err = s.Inventories("alex"); err != nil {
		t.Fatal("sidecars collided with inventories", err)
	}
	if _, err = s.ApplyAppIcon("alex", device, 1, 2, pkgs[0], icon); err == nil {
		t.Fatal("stale metadata accepted")
	}
	if _, err = s.ApplyInventory("alex", inv(device, 3, pkgs[1:]...)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyInventory("alex", inv(device, 4, pkgs...)); err != nil {
		t.Fatal(err)
	}
	got, err = s.AppIcons("alex", device, nil)
	if err != nil || len(got) != 511 || got[pkgs[0]] != "" {
		t.Fatal("reinstall revived old icon", len(got), err)
	}
	if _, err = s.ApplyAppIcon("alex", device, 4, 3, pkgs[0], icon); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.ApplyAppIcon("alex", device, 4, 2, pkgs[0], icon); err != nil || changed {
		t.Fatal("replay replaced icon", err)
	}
	if _, err = s.ApplyInventory("other", inv(device, 5, pkgs...)); err != nil {
		t.Fatal(err)
	}
	got, err = s.AppIcons("other", device, nil)
	if err != nil || len(got) != 0 {
		t.Fatal("reassignment retained icons", err)
	}
}

func TestAppIconValidationAndFailedWritePreserveData(t *testing.T) {
	s, _ := open(t)
	device := "fw-alex@0123456789abcdef"
	pkg := "com.example.game"
	icon := noisyIcon(t)
	s.ApplyInventory("alex", inv(device, 1, pkg))
	if !ValidAppIcon(icon) || ValidInventoryIcon(icon) {
		t.Fatal("new and legacy bounds conflated")
	}
	for _, bad := range []string{"", "https://example.invalid/icon.png", icon + "=", base64.StdEncoding.EncodeToString(make([]byte, MaxAppIconBytes+1))} {
		if _, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, bad); err == nil {
			t.Fatal("invalid PNG accepted")
		}
	}
	if _, err := s.ApplyAppIcon("alex", device, 1, 1, "com.example.missing", icon); err == nil {
		t.Fatal("unobserved package accepted")
	}
	if _, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, icon); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAppIcon("alex", device, 1, 2, pkg, "invalid"); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	got, err := s.AppIcons("alex", device, nil)
	if err != nil || got[pkg] != icon {
		t.Fatal("failed upload destroyed icon", err)
	}
}

func TestAppIconCacheLossAndCorruptionNeverBlockStartupAndIdenticalUploadRepairs(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "corrupt-png"} {
		t.Run(kind, func(t *testing.T) {
			s, dir := open(t)
			device := "fw-alex@0123456789abcdef"
			pkg := "com.example.game"
			icon := noisyIcon(t)
			s.ApplyInventory("alex", inv(device, 1, pkg))
			if _, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, icon); err != nil {
				t.Fatal(err)
			}
			path, _ := s.iconPath("alex", device, pkg)
			if kind == "missing" {
				os.Remove(path)
			} else if kind == "corrupt-png" {
				writeJSON(path, AppIcon{1, device, pkg, 1, 1, "invalid"})
			} else {
				os.WriteFile(path, []byte("corrupt"), 0600)
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := reopened.CheckPresence(); err != nil {
				t.Fatal("cache blocked startup", err)
			}
			if err := reopened.AdoptPresence(); err != nil {
				t.Fatal("cache blocked adoption", err)
			}
			v, err := reopened.Inventories("alex")
			if err != nil || len(v[device].Apps) != 1 {
				t.Fatal("metadata lost", err)
			}
			got, err := reopened.AppIcons("alex", device, nil)
			if err != nil || len(got) != 0 {
				t.Fatal("invalid cache is not neutral", err)
			}
			if _, err := reopened.ApplyAppIcon("alex", device, 1, 1, pkg, icon); err != nil {
				t.Fatal("identical upload did not repair", err)
			}
			got, err = reopened.AppIcons("alex", device, nil)
			if err != nil || got[pkg] != icon {
				t.Fatal("repair incomplete", err)
			}
		})
	}
}

func TestAppIconDiskFailureIsNotAcknowledged(t *testing.T) {
	s, _ := open(t)
	device := "fw-alex@0123456789abcdef"
	pkg := "com.example.game"
	s.ApplyInventory("alex", inv(device, 1, pkg))
	path, _ := s.iconPath("alex", device, pkg)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, noisyIcon(t)); err == nil || changed {
		t.Fatal("failed atomic rename acknowledged", changed, err)
	}
}

func TestAppIconLongestPackageHasPortableFilename(t *testing.T) {
	s, _ := open(t)
	device := "fw-alex@0123456789abcdef"
	pkg := "com.example." + strings.Repeat("a", 243)
	if _, err := s.ApplyInventory("alex", inv(device, 1, pkg)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAppIcon("alex", device, 1, 1, pkg, noisyIcon(t)); err != nil {
		t.Fatal("valid package exceeds filesystem name bound", err)
	}
}

// Identical-image refreshes must persist their sequence too: otherwise a replay
// with an intermediate sequence could replace newer artwork after restart.
func TestAppIconIdenticalRefreshPersistsReplayWatermark(t *testing.T) {
	s, dir := open(t)
	device := "fw-alex@0123456789abcdef"
	pkg := "com.example.game"
	if _, err := s.ApplyInventory("alex", inv(device, 1, pkg)); err != nil {
		t.Fatal(err)
	}
	a := noisyIcon(t)
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	for _, seq := range []int64{1, 3} {
		if _, err := s.ApplyAppIcon("alex", device, 1, seq, pkg, a); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := reopened.ApplyAppIcon("alex", device, 1, 2, pkg, b); err != nil || changed {
		t.Fatal("identical refresh lost durable watermark", changed, err)
	}
	got, err := reopened.AppIcons("alex", device, nil)
	if err != nil || got[pkg] != a {
		t.Fatal("replay replaced newer artwork", err)
	}
}
