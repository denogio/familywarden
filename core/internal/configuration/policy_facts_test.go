package configuration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// policyFacts wires the owned-mode facts contract around a controlled document.
func policyFacts(t *testing.T, mastCaptain, alexOverride string, alexDNS json.RawMessage) (*Authority, *appBackend) {
	t.Helper()
	c := sample()
	c.Current.Profiles[0].Apps = "1"
	c.Current.Profiles[0].ScreenMinutes = "10"
	_ = c.Seal()
	b := &appBackend{fakeBackend: &fakeBackend{current: c}, facts: platform.Context{
		RuntimeFormat: "runtime-facts-v1",
		BonusFormat:   bonusFormat,
		TimeZone:      "UTC",
		MastCaptain:   mastCaptain,
		Profiles: map[string]platform.Profile{
			"alex":   {Override: alexOverride, DNS: alexDNS},
			"taylor": {Override: "none"},
		},
	}}
	a, err := New(b, b, &memory{})
	if err != nil {
		t.Fatal(err)
	}
	a.Now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	activate(t, a, b.fakeBackend)
	return a, b
}

func TestVoyageDisplayNeverTurnsMissingOrInvalidWaitingTimeIntoZero(t *testing.T) {
	for _, raw := range []string{"", "bad", "-1", "253402300800", "0", "5400"} {
		t.Run(raw, func(t *testing.T) {
			c := sample()
			c.Pact = &rules.ConfigurationPact{Captain: "alex", Type: "until", Until: "1800007200", DelaySeconds: raw, Frozen: c.Current}
			out, err := PhonePolicy(c, platform.Policy{Captain: "alex", Rules: json.RawMessage(`{"override":"none"}`)}, "alex", time.Unix(1800000000, 0))
			if err != nil {
				t.Fatal(err)
			}
			var p phonePact
			if err := json.Unmarshal(out.Pact, &p); err != nil {
				t.Fatal(err)
			}
			known := raw == "0" || raw == "5400"
			if p.Details == nil || (p.Details.WaitingSeconds != nil) != known {
				t.Fatal("unknown waiting period became an assumed value", string(out.Pact))
			}
		})
	}
}

func TestOwnedPhonePolicyUsesPlatformFacts(t *testing.T) {
	a, b := policyFacts(t, "alex", "block", json.RawMessage(`{"hostname":"unverified.example.test"}`))
	b.dnsRuntime.LegacyAcks = []string{"alex fw-alex dns.example.test - -"}
	p, err := a.Policy("alex", 3)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Override string `json:"override"`
	}
	if err := json.Unmarshal(p.Rules, &r); err != nil || r.Override != "block" {
		t.Fatal("captain override fact not applied", string(p.Rules))
	}
	var dns struct {
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(p.DNS, &dns); err != nil || dns.Hostname != "fw-alex.dns.example.test" {
		t.Fatal("core-owned DNS acknowledgement not used", string(p.DNS), err)
	}
	if p.Captain != "alex" {
		t.Fatal("mast captain fact missing", p.Captain)
	}

	// A non-captain profile gets no captain fields and no v2 app wire policy.
	other, err := a.Policy("taylor", 2)
	if err != nil {
		t.Fatal(err)
	}
	if other.Captain != "" || other.Apps != nil || other.DNS != nil {
		t.Fatal("non-captain policy carries captain facts", other)
	}
	var none struct {
		Override string `json:"override"`
	}
	if err := json.Unmarshal(other.Rules, &none); err != nil || none.Override != "none" {
		t.Fatal("non-captain rules not the plain override fact", string(other.Rules))
	}

	// The mutation path must take the platform lock through the facts read.
	var locked bool
	if err := a.WithContext(func(platform.Context) error {
		locked = b.locked
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("owned mutation did not hold the platform state lock")
	}
}

func TestOwnedPhonePolicyRefusesDivergentMastCaptain(t *testing.T) {
	a, _ := policyFacts(t, "taylor", "none", nil)
	if _, err := a.Policy("alex", 3); err == nil {
		t.Fatal("platform captain divergence accepted")
	}
}
