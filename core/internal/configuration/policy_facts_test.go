package configuration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
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

func TestOwnedPhonePolicyUsesPlatformFacts(t *testing.T) {
	a, b := policyFacts(t, "alex", "block", json.RawMessage(`{"hostname":"alex.fw"}`))
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
	if string(p.DNS) != `{"hostname":"alex.fw"}` {
		t.Fatal("verified DNS input dropped", p.DNS)
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
