package configuration

import (
	"github.com/denogio/familywarden/core/internal/rules"
	"strings"
	"testing"
)

func TestInvalidDNSEditsNeverReachPersistence(t *testing.T) {
	cases := []struct {
		name   string
		change func(*rules.Configuration)
	}{
		{"existing ID still requires a domain", func(e *rules.Configuration) { e.Current.Profiles[1].DNS = "1" }},
		{"invalid ClientID", func(e *rules.Configuration) { e.Current.Profiles[1].ClientID = "bad.id" }},
		{"unknown protection", func(e *rules.Configuration) { e.Current.Profiles[1].Protections = []string{"not_a_protection"} }},
		{"invalid service", func(e *rules.Configuration) { e.Current.Profiles[1].Services = []string{"bad service"} }},
		{"oversized service", func(e *rules.Configuration) { e.Current.Profiles[1].Services = []string{strings.Repeat("a", 65)} }},
		{"URL as DNS domain", func(e *rules.Configuration) { e.Current.Settings.DNSDomain = "https://dns.example.net" }},
		{"invalid label", func(e *rules.Configuration) { e.Current.Settings.DNSDomain = "-bad.example.net" }},
		{"invalid endpoint", func(e *rules.Configuration) { e.Current.Settings.ResolverURL = "file:///etc/passwd" }},
		{"credentials in endpoint", func(e *rules.Configuration) { e.Current.Settings.ResolverURL = "http://user:password@192.0.2.1" }},
		{"oversized combined hostname", func(e *rules.Configuration) {
			e.Current.Settings.DNSDomain = strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 40)
			e.Current.Profiles[1].DNS = "1"
			e.Current.Profiles[1].ClientID = strings.Repeat("x", 63)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b, s := setup(t)
			activate(t, a, b)
			d, _, _ := a.load()
			e := clone(d.Configuration)
			tc.change(&e)
			before := append([]byte{}, s.data...)
			applies := b.applies
			r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": e.Current}))
			if r["error"] == nil {
				t.Fatal("invalid DNS edit accepted", r)
			}
			if string(s.data) != string(before) || b.applies != applies {
				t.Fatal("invalid DNS edit changed storage or platform")
			}
		})
	}
}

func TestValidDNSEditAndDisablingBeforeClearingDomain(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Settings.DNSDomain = "DoT.Example.NET"
	next.Current.Settings.ResolverURL = "http://192.0.2.1:3000/agh"
	next.Current.Profiles[1].DNS = "1"
	next.Current.Profiles[1].Protections = []string{"filter_lists", "safe_browsing", "parental", "safe_search"}
	next.Current.Profiles[1].Services = []string{"youtube", "test_service"}
	r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))
	okResult(t, r)
	okResult(t, a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": r["token"]})))
	d, _, _ = a.load()
	next = clone(d.Configuration)
	next.Current.Settings.DNSDomain = ""
	if r = a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})); r["error"] == nil {
		t.Fatal("domain cleared while DNS remained enabled")
	}
	next.Current.Profiles[1].DNS = "0"
	okResult(t, a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})))
}

func TestUnchangedLegacyDNSFieldsSurviveUnrelatedEdits(t *testing.T) {
	a, b, _ := setup(t)
	b.current.Current.Settings.DNSDomain = "legacy invalid domain"
	b.current.Current.Settings.ResolverURL = "legacy invalid endpoint"
	b.current.Current.Profiles[1].ClientID = "Legacy.ID"
	b.current.Current.Profiles[1].Protections = []string{"legacy-protection"}
	b.current.Current.Profiles[1].Services = []string{"legacy service"}
	_ = b.current.Seal()
	activate(t, a, b)
	okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
	d, _, _ := a.load()
	if d.Configuration.Current.Profiles[1].ClientID != "Legacy.ID" || d.Configuration.Current.Profiles[1].Services[0] != "legacy service" {
		t.Fatal("unrelated edit normalized legacy DNS state")
	}
}

func TestDNSHostnameMatchesPhoneContract(t *testing.T) {
	for host, want := range map[string]bool{
		"A.b": true, "fwalex.dns.example.net": true, "a.local": true, "192.0.2.1": false, "a..b": false, "a.b.": false, "a.-b": false, "æ.b": false,
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61): true,
		strings.Repeat("a", 64) + ".b": false,
	} {
		if validDNSHostname(host) != want {
			t.Errorf("validDNSHostname(%q) != %v", host, want)
		}
	}
}
