package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

// Small constructor smoke test; the owned production-service and shared-UI
// boundary scenarios live in portability_test.go and portability_web_test.go.
type emptyHost struct {
	platform.Adapter
	configuration.Backend
}

func (emptyHost) LegacyDevices() ([]platform.LegacyDevice, error) { return nil, nil }
func (emptyHost) RemoveLegacyDevices() error                      { return nil }
func (emptyHost) Context() (platform.Context, error) {
	return platform.Context{TimeZone: "UTC", Profiles: map[string]platform.Profile{}}, nil
}

func TestProductionCompositionHasNoOpenWrtConstruction(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host := emptyHost{}
	app, err := New(host, host, st, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	for path, want := range map[string]string{"/health": `"status":"ok"`, "/version": `"core":"test-version"`, "/api/core_status": `"core":"test-version"`, "/api/devices": `"devices":[]`, "/api/configuration_status": `"owner":"legacy"`} {
		method := http.MethodGet
		if strings.HasPrefix(path, "/api/") {
			method = http.MethodPost
		}
		rec := httptest.NewRecorder()
		app.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Fatal(path, rec.Code, rec.Body.String())
		}
	}
}
