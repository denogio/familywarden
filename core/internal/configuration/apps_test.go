package configuration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestAppContextUsesDocumentRulesAndCoreBonus(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	c := sample()
	c.Current.Profiles[0].Apps = "1"
	c.Current.Profiles[0].ScreenMinutes = "40"
	c.Current.Profiles[0].ScreenWeekday = "0"
	c.Current.Profiles[1].Name = "" // Match the legacy profile-ID label fallback.
	c.Current.Profiles[0].AlwaysAllowed = []string{"com.example.reader"}
	c.Current.Apps = []rules.ConfigurationApp{
		{ID: "game", Profile: "alex", Package: "com.example.game", Minutes: "20"},
		{ID: "game_block", Profile: "alex", Package: "com.example.game", Blocked: "1"},
		{ID: "other", Profile: "taylor", Package: "com.example.other", Blocked: "1"},
	}
	c.Current.Downtime = []rules.ConfigurationWindow{{ID: "bedtime", Profile: "alex", Days: "mon tue", Start: "21:00", Stop: "07:00"}}
	bonus := "bonus|total|15|2027-01-15"
	facts := platform.Context{BonusFormat: bonusFormat, TimeZone: "Europe/Copenhagen", Profiles: map[string]platform.Profile{
		"alex":   {Name: "Stale label", AppInput: &rules.AppConfig{Version: 1, Enabled: "1", ScreenMinutes: "999", Bonus: bonus}},
		"taylor": {Name: "Taylor", AppInput: &rules.AppConfig{Version: 1}},
		"stale":  {Name: "Deleted", AppInput: &rules.AppConfig{Version: 1}},
	}}
	got, err := AppContext(c, facts, now, map[string]string{"alex": bonus})
	if err != nil {
		t.Fatal(err)
	}
	if got.Profiles["alex"].Name != "Alex" || got.Profiles["taylor"].Name != "taylor" || len(got.Profiles) != 2 || got.TimeZone != facts.TimeZone || facts.Profiles["alex"].AppInput.ScreenMinutes != "999" {
		t.Fatal(got, facts)
	}
	apps, requires, err := got.Profiles["alex"].AppInput.Compile()
	if err != nil || apps.Screen["mon"] != 0 || apps.Screen["sun"] != 40 || apps.Bonus.Total != 15 || len(apps.Downtime) != 1 || !reflect.DeepEqual(apps.Blocked, []string{"com.example.game"}) || !reflect.DeepEqual(apps.AlwaysAllowed, []string{"com.example.reader"}) || !reflect.DeepEqual(requires, got.Profiles["alex"].Requires) || apps.Rev != got.Profiles["alex"].AppsRev {
		t.Fatal(apps, requires, err)
	}
	// The complete frozen app configuration wins outside an amendment window.
	c.Pact = &rules.ConfigurationPact{Captain: "alex", Type: "ongoing", WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00", Frozen: frozen(c.Current, "alex")}
	c.Current.Profiles[0].ScreenMinutes = "999"
	c.Current.Apps = nil
	facts.PactCaptain = "alex"
	got, err = AppContext(c, facts, now, map[string]string{"alex": bonus})
	if err != nil {
		t.Fatal(err)
	}
	frozenApps, _, err := got.Profiles["alex"].AppInput.Compile()
	if err != nil || frozenApps.Rev != apps.Rev {
		t.Fatal(frozenApps, err)
	}
	delete(facts.Profiles, "alex")
	if _, err = AppContext(c, facts, now, nil); err == nil {
		t.Fatal("missing runtime facts accepted")
	}
}

type appBackend struct {
	*fakeBackend
	facts  platform.Context
	locked bool
}

func (b *appBackend) Context() (platform.Context, error) { return b.facts, nil }
func (b *appBackend) Facts() (platform.Context, error)   { return b.facts, nil }
func (b *appBackend) WithContext(fn func(platform.Context) error) error {
	b.locked = true
	defer func() { b.locked = false }()
	return fn(b.facts)
}
func (b *appBackend) FactsContext(fn func(platform.Context) error) error {
	b.locked = true
	defer func() { b.locked = false }()
	return fn(b.facts)
}
func (b *appBackend) Policy(id string, version int) (platform.Policy, error) {
	p := shellPolicy("alex", "block")
	p.Context = b.facts
	p.Apps = json.RawMessage(`{"stale":true}`)
	return p, nil
}

func TestOwnedPhoneAndMutationContextsUseTheSameAppRevision(t *testing.T) {
	c := sample()
	c.Current.Profiles[0].Apps = "1"
	c.Current.Profiles[0].ScreenMinutes = "10"
	_ = c.Seal()
	b := &appBackend{fakeBackend: &fakeBackend{current: c}, facts: platform.Context{BonusFormat: bonusFormat, TimeZone: "UTC", MastCaptain: "alex", Profiles: map[string]platform.Profile{
		"alex":   {AppInput: &rules.AppConfig{Version: 1, Enabled: "1", ScreenMinutes: "999"}, Override: "none"},
		"taylor": {AppInput: &rules.AppConfig{Version: 1}},
	}}}
	a, err := New(b, b, &memory{})
	if err != nil {
		t.Fatal(err)
	}
	// Legacy policy is unchanged until explicit import.
	p, err := a.Policy("alex", 3)
	if err != nil || string(p.Apps) != `{"stale":true}` {
		t.Fatal(p, err)
	}
	okResult(t, a.API("configuration_import", request(map[string]any{"revision": c.Revision})))
	ctx, err := a.Context()
	if err != nil {
		t.Fatal(err)
	}
	p, err = a.Policy("alex", 3)
	if err != nil {
		t.Fatal(err)
	}
	var apps rules.AppPolicy
	if json.Unmarshal(p.Apps, &apps) != nil || apps.Screen["mon"] != 10 || apps.Rev != ctx.Profiles["alex"].AppsRev {
		t.Fatal(string(p.Apps), ctx)
	}
	if err = a.WithContext(func(locked platform.Context) error {
		if !b.locked || locked.Profiles["alex"].AppsRev != apps.Rev {
			t.Fatal("mutation escaped state lock or used a different revision", locked)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, err = a.Policy("alex", 2)
	if err != nil || p.Apps != nil {
		t.Fatal("v2 acquired an app wire policy", p, err)
	}
}

func TestAppContextRefusesUnavailableFactsOrInvalidCoreBonus(t *testing.T) {
	c := sample()
	c.Current.Profiles[0].Apps = "1"
	for _, kind := range []string{"missing profile", "malformed bonus", "divergent pact"} {
		t.Run(kind, func(t *testing.T) {
			facts := platform.Context{BonusFormat: bonusFormat, Profiles: map[string]platform.Profile{
				"alex":   {AppInput: &rules.AppConfig{Version: 1}},
				"taylor": {AppInput: &rules.AppConfig{Version: 1}},
			}}
			bonus := map[string]string{}
			switch kind {
			case "missing profile":
				delete(facts.Profiles, "alex")
			case "malformed bonus":
				bonus["alex"] = "bonus|total|invalid|2027-01-15"
			case "divergent pact":
				facts.PactCaptain = "alex"
			}
			if _, err := AppContext(c, facts, time.Unix(1800000000, 0), bonus); err == nil {
				t.Fatal("invalid runtime facts accepted")
			}
		})
	}
}
