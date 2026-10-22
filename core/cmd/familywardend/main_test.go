package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/store"
)

func TestHealthAndVersion(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := handler(st)
	for path, want := range map[string]string{"/health": `"status":"ok"`, "/version": `"core":"dev"`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /health: %d", rec.Code)
	}
}

// Short /tmp paths: unix socket paths are limited to ~108 bytes.
func socketPath(t *testing.T) string {
	dir, err := os.MkdirTemp("", "fwd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "core.sock")
}

func TestListenTakesOverOnlyStaleSockets(t *testing.T) {
	sock := socketPath(t)
	if err := os.WriteFile(sock, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listen(sock); err == nil {
		t.Fatal("listened over a regular file")
	}
	if b, _ := os.ReadFile(sock); string(b) != "keep" {
		t.Fatal("regular file was touched")
	}
	os.Remove(sock)

	l, err := listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(sock); fi.Mode().Perm() != 0o600 {
		t.Errorf("socket mode %v, want 0600", fi.Mode().Perm())
	}
	go http.Serve(l, http.NotFoundHandler())
	if _, err := listen(sock); err == nil || !strings.Contains(err.Error(), "another familywardend") {
		t.Fatalf("second instance: %v", err)
	}
	l.Close()

	// A crashed core leaves its socket file behind: that one is taken over.
	stale, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	stale.Close()
	l, err = listen(sock)
	if err != nil {
		t.Fatalf("stale socket not taken over: %v", err)
	}
	l.Close()
}

func TestStatusBridge(t *testing.T) {
	sock := socketPath(t)
	var out bytes.Buffer
	status(sock, &out)
	if got := out.String(); got != `{"error":"core not reachable","running":false}`+"\n" {
		t.Errorf("down: %s", got)
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l, err := listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go http.Serve(l, handler(st))
	out.Reset()
	status(sock, &out)
	if got := out.String(); got != `{"core":"dev","running":true}`+"\n" {
		t.Errorf("up: %s", got)
	}
}
