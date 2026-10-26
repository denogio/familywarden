package configuration

import (
	"reflect"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/agh"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestPrivateDNSConfigurationContract(t *testing.T) {
	a, b, _ := setup(t)
	b.current.Current.Settings.ResolverURL = "http://192.0.2.1:3000"
	_ = b.current.Seal()
	activate(t, a, b)
	r := a.API("configuration_dns", nil)
	okResult(t, r)
	input, ok := r["input"].(map[string]any)
	if !ok || r["format"] != "dns-config-v1" || input["url"] != b.current.Current.Settings.ResolverURL {
		t.Fatal("private DNS contract omitted its version or authoritative endpoint", r)
	}
}

func TestDNSOnlyTightensForABoundCaptain(t *testing.T) {
	now := time.Unix(1800000000, 0)
	c := sample()
	c.Current.Settings.DNSDomain = "unfiltered.example.org"
	c.Current.Profiles[0].DNS, c.Current.Profiles[0].ClientID = "0", ""
	c.Current.Profiles[0].Protections, c.Current.Profiles[0].Services = []string{"safe_browsing"}, []string{"tiktok"}
	c.Current.Profiles[1].DNS = "1"
	c.Pact = &rules.ConfigurationPact{Captain: "alex", Type: "ongoing", WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00",
		DNS: rules.ConfigurationDNS{Enabled: "1", ClientID: "fwalex", Domain: "dns.example.net", Protections: []string{"filter_lists", "safe_browsing"}, Services: []string{"youtube"}}}

	domain, captain, got := DNS(c, now)
	want := []agh.Profile{
		{ID: "alex", Name: "Alex", ClientID: "fwalex", Protections: []string{"safe_browsing", "filter_lists", "safe_browsing"}, Services: []string{"tiktok", "youtube"}},
		{ID: "taylor", Name: "Taylor", ClientID: "fw-preserved", Protections: []string{}, Services: []string{}},
	}
	if domain != "dns.example.net" || captain != "alex" || !reflect.DeepEqual(got, want) {
		t.Fatalf("%s %s %#v", domain, captain, got)
	}

	// A deleted captain profile still keeps its snapshot DNS.
	c.Current.Profiles = c.Current.Profiles[1:]
	if _, _, got = DNS(c, now); len(got) != 2 || got[1].ID != "alex" || got[1].ClientID != "fwalex" {
		t.Fatalf("%#v", got)
	}

	// Without an active pact only the live configuration counts.
	c.Pact = nil
	if domain, captain, got = DNS(c, now); domain != "unfiltered.example.org" || captain != "" || len(got) != 1 || got[0].ID != "taylor" {
		t.Fatalf("%s %s %#v", domain, captain, got)
	}
}
