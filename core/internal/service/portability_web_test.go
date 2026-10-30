package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAdminInstalledAPI(t *testing.T) {
	f := newPortableFixture(t)
	phone := f.phone("fw-alex", "alex")
	r := f.upload(phone, "inventory", map[string]any{"type": "inventory", "v": 1, "device": phone.id, "profile": "alex", "seq": 1, "installed": 1,
		"apps": []map[string]any{{"package": "com.example.unused", "label": "Alex unused app", "system": false, "essential": false}}})
	if r["state"] != "stored" {
		t.Fatal(r)
	}
	r = f.api("admin_installed", map[string]any{"profile": "alex", "search": "UNUSED", "filter": "all", "page": 0, "sort": "name"})
	if r["total"] != float64(1) || r["page_size"] != float64(20) {
		t.Fatal(r)
	}
	a := r["apps"].([]any)[0].(map[string]any)
	if a["package"] != "com.example.unused" || a["seconds"] != nil {
		t.Fatal(a)
	}
	if r := f.call("/api/admin_installed", map[string]any{"profile": "alex", "extra": true}); r["error"] != "malformed request" {
		t.Fatal(r)
	}
	for _, value := range []any{"invalid", 42, nil} {
		if r := f.call("/api/admin_installed", map[string]any{"profile": "alex", "sort": value}); r["error"] != "malformed request" {
			t.Fatal(r)
		}
	}
}

// Opt-in only because this requires the repository's existing Playwright install.
// CI runs it explicitly. The helper process is the browser driver, never a native
// adapter command; production services still run with an empty PATH.
func TestPortableSharedUI(t *testing.T) {
	if os.Getenv("FW_TEST_SHARED_UI") != "1" {
		t.Skip("set FW_TEST_SHARED_UI=1 after installing test/e2e dependencies and Chromium")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := newPortableFixture(t)
	phone := f.phone("fw-alex", "alex")
	r := f.upload(phone, "usage", map[string]any{"type": "usage", "v": 1, "device": phone.id, "profile": "alex",
		"day": f.now().Format(time.DateOnly), "seq": 1, "total": 900,
		"apps": map[string]int{"com.example.video": 600, "com.example.legacy": 60}, "labels": map[string]string{"com.example.video": "Video"}})
	if r["state"] != "stored" {
		t.Fatal(r)
	}
	var icon bytes.Buffer
	normalized := image.NewRGBA(image.Rect(0, 0, 96, 96))
	draw.Draw(normalized, normalized.Bounds(), &image.Uniform{color.RGBA{50, 160, 100, 255}}, image.Point{}, draw.Src)
	for y := 24; y < 72; y++ {
		for x := 36; x < 72-int(math.Abs(float64(y-48))*1.5); x++ {
			normalized.Set(x, y, color.White)
		}
	}

	if err := png.Encode(&icon, normalized); err != nil {
		t.Fatal(err)
	}
	r = f.upload(phone, "inventory", map[string]any{"type": "inventory", "v": 1, "device": phone.id, "profile": "alex", "seq": 1, "installed": 2,
		"apps": []map[string]any{
			{"package": "com.example.legacy", "label": "Legacy", "system": false, "essential": false},
			{"package": "com.example.video", "label": "Video", "system": false, "essential": false},
		}})
	if r["state"] != "stored" {
		t.Fatal(r)
	}
	r = f.upload(phone, "inventory_icons", map[string]any{"type": "inventory_icons", "v": 1, "device": phone.id, "profile": "alex", "inventory_seq": 1, "seq": 1, "icons": []map[string]string{{"package": "com.example.video", "icon": base64.StdEncoding.EncodeToString(icon.Bytes())}}})
	if r["state"] != "stored" {
		t.Fatal(r)
	}
	f.host.failNet = 1 // the browser's first network action must fail closed
	mux := http.NewServeMux()
	mux.Handle("/api/", f.app.Handler)
	mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir(filepath.Join(repo, "web")))))
	// Test-only loopback host: not a product listener, authentication scheme or
	// standalone mode. Native session/ACL checks remain each real host's job.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html lang="en"><body><main id="control"></main>
<script type="module">
import {mount} from '/web/control.js';
window.control = mount(document.getElementById('control'), {lang:'en', interval:60000,
  call:async (method, params = {}) => {
    const response = await fetch('/api/' + method, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(params)});
    if (!response.ok) throw new Error('Core HTTP failure');
    return response.json();
  }
});
</script></body></html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(repo, "test/e2e/portable-control.cjs"), server.URL)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shared UI against production service: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
	server.Close() // finish all browser requests before inspecting/restarting the service
	f.host.mu.Lock()
	name := f.host.current.Current.Profiles[0].Name
	budget := f.host.current.Current.Profiles[0].ScreenMinutes
	restores := len(f.host.restored)
	f.host.mu.Unlock()
	if name != "Alex portable" || budget != "45" || restores != 1 {
		t.Fatal("browser did not reach real configuration/apply contracts", name, budget, restores)
	}
	f.restart()
	if f.configuration().Current.Profiles[0].Name != name || f.sync(phone)["enrollment"] != "approved" {
		t.Fatal("browser changes or phone identity did not survive reconciliation")
	}
}
