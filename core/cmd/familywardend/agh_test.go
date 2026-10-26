package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAGHCommandsRejectInvalidInputWithoutOutput(t *testing.T) {
	for name, command := range map[string]func(io.Reader, io.Writer) error{"sync": aghSync, "services": aghServices} {
		t.Run(name, func(t *testing.T) {
			for _, raw := range []string{"", "null", "[]", "{}", `{"unknown":true}`, `{"url":"http://example.invalid","auth":"token"} {}`, `{"url":"file:///etc/passwd","auth":"token"}`, `{"url":"http://user:password@example.invalid","auth":"token"}`, `{"url":"http://example.invalid?x=1","auth":"token"}`, "{\"auth\":\"\xff\"}", strings.Repeat(" ", 1<<20+1)} {
				var out bytes.Buffer
				if err := command(strings.NewReader(raw), &out); err == nil || out.Len() != 0 {
					t.Fatalf("accepted %q: %v %s", raw[:min(80, len(raw))], err, out.String())
				}
			}
		})
	}
}

func TestAGHServicesCommandAndRedirectRefusal(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		if r.Header.Get("Authorization") != "Basic token" {
			t.Error("missing credentials")
		}
		_, _ = w.Write([]byte(`{"blocked_services":[{"id":"youtube","name":"YouTube"}]}`))
	}))
	defer srv.Close()
	raw, _ := json.Marshal(map[string]string{"url": srv.URL, "auth": "token"})
	var out bytes.Buffer
	if err := aghServices(bytes.NewReader(raw), &out); err != nil || out.String() != "{\"services\":[{\"id\":\"youtube\",\"name\":\"YouTube\"}]}\n" {
		t.Fatal(err, out.String())
	}
	redirect = true
	out.Reset()
	if err := aghServices(bytes.NewReader(raw), &out); err != nil || !strings.Contains(out.String(), `"error"`) || targetCalls != 0 {
		t.Fatal(err, out.String(), targetCalls)
	}
}
