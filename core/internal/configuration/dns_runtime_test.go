package configuration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

func TestDNSRuntimeImportsOnceAndDoesNotTrustFacts(t *testing.T) {
	a, b, s := setup(t)
	b.dnsRuntime = platform.DNSRuntime{LegacyOwned: []string{"fw-alex"}, LegacyAcks: []string{"alex fw-alex dns.example.test safe_browsing youtube"}}
	activate(t, a, b)
	dns, err := a.verifiedDNS("alex")
	if err != nil || !strings.Contains(string(dns), "fw-alex.dns.example.test") {
		t.Fatal(string(dns), err)
	}
	b.dnsRuntime.LegacyAcks = []string{"alex changed dns.example.test - -"}
	again, err := a.verifiedDNS("alex")
	if err != nil || string(again) != string(dns) {
		t.Fatal("legacy file remained authority", string(again), err)
	}
	before := string(s.dns)
	okResult(t, a.API("configuration_block", []byte(`{"profile":"alex"}`)))
	if string(s.dns) != before {
		t.Fatal("network mutation changed DNS runtime")
	}
	var state dnsState
	if err := json.Unmarshal(s.dns, &state); err != nil {
		t.Fatal(err)
	}
	state.Version = 999
	s.dns = request(state)
	if _, err := a.verifiedDNS("alex"); err == nil {
		t.Fatal("future runtime accepted")
	}
}

func TestDNSRetryCadenceAndCredentialChanges(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "offline", 503) }))
	defer server.Close()
	a, b, s := setup(t)
	b.current.Current.Settings.DNSDomain = "dns.example.test"
	b.current.Current.Settings.ResolverURL = server.URL
	b.current.Current.Profiles[0].DNS = "1"
	b.current.Current.Profiles[0].ClientID = "fw-alex"
	b.dnsRuntime.Auth = "secret"
	_ = b.current.Seal()
	activate(t, a, b)
	r := a.API("configuration_dns_tick", nil)
	if r["error"] == nil || r["error"] == "" || calls == 0 {
		t.Fatal(r, calls)
	}
	previous := calls
	a.API("configuration_dns_tick", nil)
	if calls != previous {
		t.Fatal("retry ran before backoff")
	}
	old := a.Now()
	a.Now = func() time.Time { return old.Add(300 * time.Second) }
	a.API("configuration_dns_tick", nil)
	if calls <= previous {
		t.Fatal("retry did not run")
	}
	previous = calls
	okResult(t, a.API("configuration_dns_auth", []byte(`{"username":"test","password":"changed"}`)))
	a.API("configuration_dns_tick", nil)
	if calls <= previous {
		t.Fatal("credential change did not wake retry")
	}
	if strings.Contains(string(s.dns), "secret") || strings.Contains(string(s.dns), "changed") {
		t.Fatal("credentials persisted in core runtime")
	}
}

func TestDNSSyncOnlyAcknowledgesVerifiedResultAndKeepsStateOnSaveFailure(t *testing.T) {
	var clients []map[string]any
	failReadback := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/control/access/list":
			json.NewEncoder(w).Encode(map[string]any{"allowed_clients": []string{}, "disallowed_clients": []string{}, "blocked_hosts": []string{}})
		case "/control/clients":
			if failReadback {
				json.NewEncoder(w).Encode(map[string]any{"clients": []any{}})
			} else {
				if clients == nil {
					clients = []map[string]any{}
				}
				json.NewEncoder(w).Encode(map[string]any{"clients": clients})
			}
		case "/control/clients/add", "/control/clients/update":
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			if data, ok := v["data"].(map[string]any); ok {
				v = data
			}
			clients = []map[string]any{v}
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	a, b, s := setup(t)
	b.current.Current.Settings.DNSDomain = "dns.example.test"
	b.current.Current.Settings.ResolverURL = server.URL
	b.current.Current.Profiles[0].DNS = "1"
	b.current.Current.Profiles[0].ClientID = "fw-alex"
	b.dnsRuntime.Auth = "secret"
	_ = b.current.Seal()
	activate(t, a, b)
	if r := a.API("configuration_dns_sync", nil); r["error"] != "" {
		t.Fatal(r)
	}
	before := string(s.dns)
	raw, err := a.verifiedDNS("alex")
	if err != nil || !strings.Contains(string(raw), "fw-alex.dns.example.test") {
		t.Fatal(string(raw), err)
	}
	failReadback = true
	if r := a.API("configuration_dns_sync", nil); r["error"] == "" {
		t.Fatal("unverified result acknowledged", r)
	}
	raw, err = a.verifiedDNS("alex")
	if err != nil || !strings.Contains(string(raw), "fw-alex.dns.example.test") {
		t.Fatal("old verified ack lost", string(raw), err)
	}
	s.dns = []byte(before)
	s.fail = true
	if r := a.API("configuration_dns_sync", nil); r["error"] == nil {
		t.Fatal("save failure hidden", r)
	}
	if string(s.dns) != before {
		t.Fatal("unpersisted result became authority")
	}
}
