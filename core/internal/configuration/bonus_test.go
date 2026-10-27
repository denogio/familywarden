package configuration

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

type bonusBackend struct{ *appBackend }

func (b *bonusBackend) ConfigurationTransaction(fn func(rules.Configuration, platform.Context, map[string]platform.NetworkOverride, func(rules.Configuration, platform.NetworkIntent) error) error) error {
	return b.fakeBackend.ConfigurationTransaction(func(c rules.Configuration, _ platform.Context, overrides map[string]platform.NetworkOverride, apply func(rules.Configuration, platform.NetworkIntent) error) error {
		return fn(c, b.facts, overrides, apply)
	})
}

func bonusSetup(t *testing.T, legacy string) (*Authority, *bonusBackend, *memory) {
	t.Helper()
	c := sample()
	c.Current.Profiles[0].Apps = "1"
	c.Current.Profiles[0].ScreenMinutes = "40"
	_ = c.Seal()
	b := &bonusBackend{&appBackend{fakeBackend: &fakeBackend{current: c}, facts: platform.Context{
		BonusFormat: bonusFormat, LegacyBonus: legacy, TimeZone: "UTC", MastCaptain: "alex",
		Profiles: map[string]platform.Profile{
			"alex":   {AppInput: &rules.AppConfig{Version: 1, Enabled: "1", Bonus: "untrusted stale bonus"}, Override: "none"},
			"taylor": {AppInput: &rules.AppConfig{Version: 1}},
		},
	}}}
	s := &memory{}
	a, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	a.Now = func() time.Time { return time.Date(2027, 1, 15, 23, 30, 0, 0, time.UTC) }
	return a, b, s
}

func giveBonus(a *Authority, target string, minutes any) map[string]any {
	return a.API("configuration_bonus_add", request(map[string]any{"profile": "alex", "package": target, "minutes": minutes}))
}

func TestBonusImportAuthorityReadersAndRestart(t *testing.T) {
	legacy := "alex|2027-01-15|total|15\nalex|2027-01-15|com.example.game|10\nalex|2027-01-16|total|7\n"
	a, b, s := bonusSetup(t, legacy)
	activate(t, a, b.fakeBackend)
	before, _, _ := a.load()
	okResult(t, giveBonus(a, "", "0015"))
	okResult(t, giveBonus(a, "com.example.game", 20))
	for i := 0; i < 6; i++ {
		okResult(t, giveBonus(a, "total", 240))
	}
	r := a.API("configuration_bonus_read", nil)
	if r["error"] != nil || r["totals"].(map[string]int)["alex"] != 1440 {
		t.Fatal(r)
	}
	ctx, err := a.Context()
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Policy("alex", 3)
	if err != nil {
		t.Fatal(err)
	}
	var apps rules.AppPolicy
	if json.Unmarshal(p.Apps, &apps) != nil || apps.Bonus.Total != 1440 || apps.Bonus.Apps[0].Minutes != 30 || apps.Rev != ctx.Profiles["alex"].AppsRev {
		t.Fatal(string(p.Apps), ctx)
	}
	if err := a.WithContext(func(c platform.Context) error {
		if c.Profiles["alex"].AppsRev != apps.Rev || !b.locked {
			t.Fatal("mutation context differs from phone policy", c)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, _, _ := a.load()
	if after.Configuration.Revision != before.Configuration.Revision || b.applies != 1 {
		t.Fatal("runtime bonus changed configuration revision or applied firewall")
	}
	// Once imported, even a malformed legacy file cannot replace core-owned bonus.
	b.facts.LegacyBonus = "corrupt compatibility file"
	restarted, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = a.Now
	if got := restarted.API("configuration_bonus_read", nil); !reflect.DeepEqual(got, r) {
		t.Fatal("restart lost bonus", got, r)
	}
	// Router-local midnight, not UTC midnight; future legacy rows are preserved.
	b.facts.TimeZone = "Europe/Copenhagen"
	r = restarted.API("configuration_bonus_read", nil)
	if r["day"] != "2027-01-16" || r["totals"].(map[string]int)["alex"] != 7 {
		t.Fatal(r)
	}
}

func TestBonusUpgradeImportsOnceAndRequiresCompatibleBackend(t *testing.T) {
	a, b, s := bonusSetup(t, "alex|2027-01-15|total|25")
	// A previously imported household has no bonus member yet. Upgrade it under
	// the same platform transaction, without changing the configuration revision.
	s.data = request(document{Version: 1, Configuration: b.current})
	revision := b.current.Revision
	upgraded, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	upgraded.Now = a.Now
	if r := upgraded.API("configuration_bonus_read", nil); r["totals"].(map[string]int)["alex"] != 25 {
		t.Fatal(r)
	}
	d, _, _ := upgraded.load()
	if d.Bonus == nil || d.Configuration.Revision != revision {
		t.Fatal("upgrade changed configuration or did not record bonus ownership", d)
	}
	b.facts.LegacyBonus = "broken after upgrade"
	if _, err := New(b, b, s); err != nil {
		t.Fatal("legacy state was reimported", err)
	}
	b.facts.BonusFormat = ""
	if _, err := New(b, b, s); err == nil {
		t.Fatal("new core started with an incompatible backend")
	}
	if _, err := upgraded.Context(); err == nil {
		t.Fatal("live backend downgrade served owned policy")
	}
	b.facts.BonusFormat = bonusFormat
	s.data = request(document{Version: 1, Configuration: b.current})
	if _, err := upgraded.Context(); err == nil {
		t.Fatal("removed bonus ownership served an empty policy")
	}
}

func TestBonusRefusalsLeaveStateUntouched(t *testing.T) {
	for _, kind := range []string{"zero", "oversize", "negative", "decimal", "overflow", "bad package", "unknown profile", "apps off", "pact", "pending", "old adapter", "duplicate JSON"} {
		t.Run(kind, func(t *testing.T) {
			a, b, s := bonusSetup(t, "")
			activate(t, a, b.fakeBackend)
			input := map[string]any{"profile": "alex", "minutes": 15}
			switch kind {
			case "zero":
				input["minutes"] = 0
			case "oversize":
				input["minutes"] = 241
			case "negative":
				input["minutes"] = -1
			case "decimal":
				input["minutes"] = 1.5
			case "overflow":
				input["minutes"] = strings.Repeat("9", 100)
			case "bad package":
				input["package"] = "com..game"
			case "unknown profile":
				input["profile"] = "missing"
			case "apps off":
				input["profile"] = "taylor"
			case "pact":
				okResult(t, a.API("configuration_pact", request(map[string]any{"until": 0, "window_day": "sun", "window_start": "10:00", "window_stop": "11:00"})))
			case "pending":
				okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
			case "old adapter":
				b.facts.BonusFormat = ""
			}
			raw := request(input)
			if kind == "duplicate JSON" {
				raw = []byte(`{"profile":"alex","minutes":15,"minutes":30}`)
			}
			before := string(s.data)
			if r := a.API("configuration_bonus_add", raw); r["error"] == nil || string(s.data) != before {
				t.Fatal("refused bonus mutated state", r)
			}
			if a.timer != nil {
				a.timer.Stop()
			}
		})
	}
}

func TestBonusMigrationRefusesMissingFormatAndCorruption(t *testing.T) {
	for _, legacy := range []string{"old adapter", "broken", "alex|2027-01-15|total|9999", "alex|2027-02-30|total|15", "alex|2027-01-15|total|15\nalex|2027-01-15|total|20"} {
		t.Run(legacy, func(t *testing.T) {
			a, b, s := bonusSetup(t, legacy)
			if legacy == "old adapter" {
				b.facts.BonusFormat = ""
			}
			if r := a.API("configuration_import", request(map[string]any{"revision": b.current.Revision})); r["error"] == nil || len(s.data) != 0 || b.applies != 0 {
				t.Fatal("invalid legacy state imported", r)
			}
			// Existing owned documents migrate before application; a failed import
			// leaves the document and platform untouched, never falling back.
			s.data = request(document{Version: 1, Configuration: b.current})
			before := string(s.data)
			if _, err := New(b, b, s); err == nil || string(s.data) != before || b.applies != 0 {
				t.Fatal("invalid upgrade rewrote state")
			}
		})
	}
}

func TestBonusSurvivesConfigurationRollbackAndPactOperations(t *testing.T) {
	a, b, s := bonusSetup(t, "alex|2027-01-14|total|5\nalex|2027-01-15|total|10")
	activate(t, a, b.fakeBackend)
	okResult(t, giveBonus(a, "total", 15))
	okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
	restarted, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = a.Now
	okResult(t, restarted.API("configuration_pact", request(map[string]any{"until": 0, "window_day": "sun", "window_start": "10:00", "window_stop": "11:00"})))
	okResult(t, restarted.API("configuration_sail_request", nil))
	okResult(t, restarted.API("configuration_sail_cancel", nil))
	d, _, err := restarted.load()
	if err != nil || len(d.Bonus.Entries) != 1 || d.Bonus.Entries[0].Minutes != 25 {
		t.Fatal("configuration/pact operation dropped bonus or failed to prune old days", d, err)
	}
	if a.timer != nil {
		a.timer.Stop()
	}
}

func TestBonusConcurrentAdditionsAndFailedSave(t *testing.T) {
	a, b, s := bonusSetup(t, "")
	activate(t, a, b.fakeBackend)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := giveBonus(a, "total", 15); r["error"] != nil {
				t.Error(r)
			}
		}()
	}
	wg.Wait()
	if r := a.API("configuration_bonus_read", nil); r["totals"].(map[string]int)["alex"] != 150 {
		t.Fatal(r)
	}
	before := string(s.data)
	s.fail = true
	if r := giveBonus(a, "total", 15); r["error"] == nil || string(s.data) != before {
		t.Fatal("failed write accepted", r)
	}
	if _, err := a.Context(); err == nil {
		t.Fatal("uncertain persistence served policy")
	}
	s.fail = false
	restarted, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = a.Now
	if r := restarted.API("configuration_bonus_read", nil); r["totals"].(map[string]int)["alex"] != 150 {
		t.Fatal(r)
	}
	// An invalid persisted runtime row is not silently ignored.
	d, _, _ := restarted.load()
	d.Bonus.Entries[0].Minutes = -1
	s.data = request(d)
	if _, err := New(b, b, s); err == nil {
		t.Fatal("corrupt bonus accepted")
	}
}
