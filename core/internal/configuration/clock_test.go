package configuration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestPhoneWindowsNormalizeMidnightWithoutWidening(t *testing.T) {
	for _, tc := range []struct {
		name, days, start, stop string
		want                    []phoneWindow
	}{
		{"end of day", "mon", "21:00", "24:00", []phoneWindow{{"mon", "21:00", "00:00"}}},
		{"whole day", "mon", "00:00", "24:00", []phoneWindow{{"mon", "00:00", "00:00"}}},
		{"start next day", "fri sat", "24:00", "08:00", []phoneWindow{{"sat sun", "00:00", "08:00"}}},
		{"equal full day", "mon", "24:00", "24:00", []phoneWindow{{"mon", "00:00", "00:00"}}},
		{"empty midnight interval", "mon", "24:00", "00:00", []phoneWindow{}},
		{"ordinary", "mon", "07:00", "20:00", []phoneWindow{{"mon", "07:00", "20:00"}}},
		{"overnight", "mon", "22:00", "01:00", []phoneWindow{{"mon", "22:00", "01:00"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sample()
			c.Current.Schedules = []rules.ConfigurationWindow{{ID: "clock", Profile: "alex", Days: tc.days, Start: tc.start, Stop: tc.stop}}
			if err := Validate(c); err != nil {
				t.Fatal("accepted document boundary refused", err)
			}
			p, err := PhonePolicy(c, shellPolicy("alex", "none"), "alex", time.Unix(1800000000, 0))
			if err != nil {
				t.Fatal(err)
			}
			var wire phoneRules
			if err = json.Unmarshal(p.Rules, &wire); err != nil || !reflect.DeepEqual(wire.Windows, tc.want) {
				t.Fatal(string(p.Rules), err)
			}
		})
	}
}

func TestNewAppDowntimeCannotBeSilentlyDiscarded(t *testing.T) {
	for _, w := range []rules.ConfigurationWindow{
		{ID: "bed", Profile: "alex", Days: "mon", Start: "21:00", Stop: "24:00"},
		{ID: "bed", Profile: "alex", Days: "mon", Start: "24:00", Stop: "08:00"},
		{ID: "bed", Profile: "alex", Days: "", Start: "21:00", Stop: "08:00"},
		{ID: "bed", Profile: "alex", Days: "mon\u00a0tue", Start: "21:00", Stop: "08:00"},
	} {
		a, b, st := setup(t)
		activate(t, a, b)
		d, _, _ := a.load()
		next := clone(d.Configuration)
		next.Current.Downtime = []rules.ConfigurationWindow{w}
		before := string(st.data)
		applies := b.applies
		r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))
		if r["error"] != "invalid app downtime" || string(st.data) != before || b.applies != applies {
			t.Fatal("invalid downtime saved or applied", r)
		}
	}
	// Legacy normalization remains unchanged on an unrelated profile edit.
	a, b, _ := setup(t)
	b.current.Current.Downtime = []rules.ConfigurationWindow{{ID: "legacy", Profile: "alex", Days: "mon", Start: "21:00", Stop: "24:00"}}
	_ = b.current.Seal()
	activate(t, a, b)
	okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
}

type legacyClockBackend struct {
	*fakeBackend
	raw json.RawMessage
}

func (b *legacyClockBackend) Policy(string, int) (platform.Policy, error) {
	c, _ := b.Context()
	return platform.Policy{Context: c, Captain: "alex", Rules: b.raw}, nil
}

func TestLegacyOwnedPhoneWindowsAreNormalizedWithoutLosingOtherRules(t *testing.T) {
	raw := json.RawMessage(`{"enabled":"1","policy":"scheduled","override":"none","windows":[{"days":"mon","start":"21:00","stop":"24:00","future_window_guard":{"blocked":true}}],"future_guard":{"blocked":true}}`)
	b := &legacyClockBackend{fakeBackend: &fakeBackend{current: sample()}, raw: raw}
	a, err := New(b, b, &memory{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Policy("alex", 3)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err = json.Unmarshal(p.Rules, &got); err != nil {
		t.Fatal(err)
	}
	var windows []phoneWindow
	if json.Unmarshal(got["windows"], &windows) != nil || len(windows) != 1 || windows[0].Stop != "00:00" || string(got["future_guard"]) != `{"blocked":true}` {
		t.Fatal("legacy clock invalid or other rule lost", string(p.Rules))
	}
	var rawWindows []map[string]json.RawMessage
	if json.Unmarshal(got["windows"], &rawWindows) != nil || string(rawWindows[0]["future_window_guard"]) != `{"blocked":true}` {
		t.Fatal("unknown window restriction lost", string(p.Rules))
	}
	// Ordinary legacy bytes stay exact; do not rebuild unknown policy fields.
	b.raw = json.RawMessage(`{"windows":[], "future_guard": {"blocked":true}}`)
	p, err = a.Policy("alex", 2)
	if err != nil || string(p.Rules) != string(b.raw) {
		t.Fatal("ordinary legacy policy rewritten", string(p.Rules), err)
	}
}

func TestNewScheduleDaysMatchKernelTokenization(t *testing.T) {
	for _, days := range []string{"mon\u00a0tue", "mon\vtue", "mon\ftue"} {
		a, b, st := setup(t)
		activate(t, a, b)
		d, _, _ := a.load()
		next := clone(d.Configuration)
		next.Current.Schedules[0].Days = days
		before := string(st.data)
		applies := b.applies
		if r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})); r["error"] != "invalid schedule days" || string(st.data) != before || b.applies != applies {
			t.Fatal("invalid day token saved", days, r)
		}
	}
}

func TestPactWindowRequiresOneKnownDay(t *testing.T) {
	for _, day := range []string{"mon tue", "mon\ttue", "mon\u00a0tue", "Monday"} {
		a, b, st := setup(t)
		activate(t, a, b)
		before := string(st.data)
		applies := b.applies
		r := a.API("configuration_pact", request(map[string]any{"until": 0, "window_day": day, "window_start": "10:00", "window_stop": "11:00"}))
		if r["error"] != "invalid amendment window" || string(st.data) != before || b.applies != applies {
			t.Fatal("compound or unknown pact day accepted", day, r)
		}
	}
}

func TestLegacyPhoneDaysCannotExceedKernelDays(t *testing.T) {
	for days, want := range map[string]string{"mon\u00a0tue": "", "mon\vtue fri": "fri", "mon\ttue": "mon\ttue"} {
		raw, _ := json.Marshal(map[string]any{"enabled": "1", "policy": "scheduled", "override": "none", "windows": []phoneWindow{{Days: days, Start: "07:00", Stop: "20:00"}}})
		p, err := normalizePhoneWindows(platform.Policy{Rules: raw})
		if err != nil {
			t.Fatal(err)
		}
		var got phoneRules
		if json.Unmarshal(p.Rules, &got) != nil {
			t.Fatal(string(p.Rules))
		}
		if want == "" {
			if len(got.Windows) != 0 {
				t.Fatal("phone acquired allow-days missing from kernel", got)
			}
		} else if len(got.Windows) != 1 || got.Windows[0].Days != want {
			t.Fatal(got)
		}
	}
}
