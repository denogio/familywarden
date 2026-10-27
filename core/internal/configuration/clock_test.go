package configuration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

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
