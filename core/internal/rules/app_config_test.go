package rules

import (
	"reflect"
	"strings"
	"testing"
)

func TestAppConfigurationNormalizesToLegacyCanonicalPolicy(t *testing.T) {
	c := AppConfig{
		Version: 1, Enabled: "1", SuspendNew: "0",
		ScreenMinutes: "120", ScreenWeekday: "060", ScreenWeekend: "0",
		AlwaysAllowed: []string{"com.example.calls", "bad package", "com.example.calls"},
		Apps: []AppSetting{
			{"com.example.video", "90", "0"}, {"com.example.video", "15", "1"}, {"com.example.video", "", "0"},
			{"com.example.game", "nonsense", "0"}, {"com.example.free", "", "maybe"},
			{"com.example.huge", "99999999999999999", "0"}, {"bad package", "10", "1"},
			{"com.example.zero", "0000", "0"},
		},
		Downtime: []AppWindow{
			{"mon\t tue", "21:00", "07:00"}, {"mon tue", "21:00", "07:00"},
			{"bad", "21:00", "07:00"}, {"sun", "24:00", "07:00"}, {"sun\r", "21:00", "07:00"},
		},
		Bonus: "bonus|total|15|2026-10-25",
	}
	in, err := c.Canonical()
	want := "always|com.example.calls||\napp|com.example.free||0\napp|com.example.game|0|0\napp|com.example.huge|1440|0\napp|com.example.video|15|1\napp|com.example.zero|0000|0\ndown|mon tue|21:00|07:00\nnew|0||\nscreen|fri|060|\nscreen|mon|060|\nscreen|sat|0|\nscreen|sun|0|\nscreen|thu|060|\nscreen|tue|060|\nscreen|wed|060|"
	if err != nil || in.Rules != want || in.Bonus != c.Bonus {
		t.Fatalf("%v\ngot: %s\nwant: %s", err, in.Rules, want)
	}
	p, requires, err := c.Compile()
	if err != nil || p.SuspendNew || p.Screen["mon"] != 60 || p.Screen["sun"] != 0 ||
		!reflect.DeepEqual(requires, []string{"apps-v1", "screen-time-v1"}) ||
		!reflect.DeepEqual(p.Blocked, []string{"com.example.video"}) {
		t.Fatal(p, requires, err)
	}
	// The same canonical input is used for wire policy and pact snapshot export.
	q, _, err := CompileApps(in)
	if err != nil || !reflect.DeepEqual(p, q) {
		t.Fatal("snapshot and signed policy disagree", p, q, err)
	}
}

func TestAppConfigurationStrictestDuplicatesAndBounds(t *testing.T) {
	for _, pair := range [][2]string{{"", "15"}, {"90", "15"}, {"0015", "15"}, {"-1", "90"}, {"99999", "90"}} {
		for _, blocked := range []string{"0", "1"} {
			c := AppConfig{Version: 1, Enabled: "1", Apps: []AppSetting{
				{"com.example.video", pair[0], "0"}, {"com.example.video", pair[1], blocked}}}
			p, _, err := c.Compile()
			if err != nil {
				t.Fatal(pair, err)
			}
			if blocked == "1" {
				if len(p.Limits) != 0 || len(p.Approved) != 0 || len(p.Blocked) != 1 {
					t.Fatal("block lost to a limit", pair, p)
				}
			} else {
				want := 15
				if pair[0] == "-1" {
					want = 0
				} else if pair[0] == "99999" {
					want = 90
				}
				if len(p.Limits) != 1 || p.Limits[0].Minutes != want {
					t.Fatal("strictest limit lost", pair, p)
				}
			}
		}
	}
	for _, value := range []string{"", "0", "0000", "1440", "1441", "99999", "-1", "1.0", " 10", "١٠"} {
		c := AppConfig{Version: 1, Enabled: "1", ScreenMinutes: value, Apps: []AppSetting{{"com.example.video", value, "0"}}}
		p, _, err := c.Compile()
		if err != nil {
			t.Fatal(value, err)
		}
		_, minuteErr := appMinutes(value)
		if minuteErr == nil && len(p.Screen) != 7 || minuteErr != nil && len(p.Screen) != 0 {
			t.Fatal("invalid daily total was not ignored", value, p)
		}
	}
}

func TestAppConfigurationVersionOffAndWeekdayFallback(t *testing.T) {
	if _, err := (AppConfig{}).Canonical(); err == nil {
		t.Fatal("unversioned configuration accepted")
	}
	if _, _, err := (AppConfig{Version: 2, Enabled: "1"}).Compile(); err == nil {
		t.Fatal("future configuration version accepted")
	}
	for _, off := range []string{"", "0", "true", "invalid"} {
		c := AppConfig{Version: 1, Enabled: off, Bonus: "not valid but app control is off"}
		p, _, err := c.Compile()
		if err != nil || p != nil {
			t.Fatal(off, p, err)
		}
	}
	c := AppConfig{Version: 1, Enabled: "1", ScreenMinutes: "120", ScreenWeekday: "invalid", SuspendNew: "invalid"}
	p, _, err := c.Compile()
	if err != nil || !p.SuspendNew || len(p.Screen) != 2 || p.Screen["sat"] != 120 {
		t.Fatal("invalid override must not fall back to a looser total", p, err)
	}
	c.ScreenWeekday = ""
	p, _, err = c.Compile()
	if err != nil || len(p.Screen) != 7 {
		t.Fatal("unset overrides inherit all-days value", p, err)
	}
	in, _ := c.Canonical()
	if strings.HasSuffix(in.Rules, "\n") {
		t.Fatal("canonical bytes changed")
	}
}
