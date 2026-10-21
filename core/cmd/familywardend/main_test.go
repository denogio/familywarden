package main

import (
	"net/http"
	"net/http/httptest"
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
