package rules

import (
	"testing"
	"time"
)

// Wednesday 2026-10-21 in a fixed zone, so the test does not depend on tzdata.
var cph = time.FixedZone("CEST", 2*3600)

func at(day, hhmm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, cph)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPrecedence(t *testing.T) {
	p := Policy{
		Rules:      []Rule{{ID: "games", Packages: []string{"com.roblox.client"}, DailyMinutes: 60}, {ID: "social", Category: "social", DailyMinutes: 30}},
		Downtime:   []Window{{Days: []string{"mon", "tue", "wed", "thu"}, Start: "21:00", Stop: "07:00"}},
		Blocked:    []string{"com.example.blocked"},
		Essential:  []string{"com.google.android.dialer"},
		Approved:   []string{"com.roblox.client", "com.instagram.android", "com.example.blocked", "com.google.android.dialer"},
		SuspendNew: true,
	}
	full := State{Remaining: map[string]int{"games": 600, "social": 600}}
	empty := State{Remaining: map[string]int{"games": 0, "social": 0}}
	granted := State{Remaining: map[string]int{"games": 0}, Granted: map[string]int{"com.roblox.client": 1200, "com.example.blocked": 600}}

	cases := []struct {
		name, pkg, cat string
		s              State
		when           time.Time
		want           Reason
		allowed        bool
	}{
		{"essential wins over downtime", "com.google.android.dialer", "", empty, at("2026-10-21", "23:00"), Essential, true},
		{"blocked even with a grant", "com.example.blocked", "", granted, at("2026-10-21", "12:00"), Blocked, false},
		{"new app suspended until approved", "com.new.app", "", full, at("2026-10-21", "12:00"), NotApproved, false},
		{"grant wins over downtime (D3)", "com.roblox.client", "", granted, at("2026-10-21", "23:30"), Grant, true},
		{"grant wins over exhausted budget", "com.roblox.client", "", granted, at("2026-10-21", "12:00"), Grant, true},
		{"downtime evening", "com.roblox.client", "", full, at("2026-10-21", "21:00"), Downtime, false},
		{"downtime continues after midnight", "com.roblox.client", "", full, at("2026-10-22", "06:59"), Downtime, false},
		{"downtime ends at stop", "com.roblox.client", "", full, at("2026-10-22", "07:00"), Allowed, true},
		{"no downtime starting friday evening", "com.roblox.client", "", full, at("2026-10-23", "22:00"), Allowed, true},
		{"thursday's window runs into friday morning", "com.roblox.client", "", full, at("2026-10-23", "06:00"), Downtime, false},
		{"budget by package", "com.roblox.client", "", empty, at("2026-10-21", "12:00"), Budget, false},
		{"budget by category", "com.instagram.android", "social", empty, at("2026-10-21", "12:00"), Budget, false},
		{"allowed", "com.instagram.android", "social", full, at("2026-10-21", "12:00"), Allowed, true},
	}
	for _, c := range cases {
		d := Decide(p, c.s, c.pkg, c.cat, c.when)
		if d.Reason != c.want || d.Allowed != c.allowed {
			t.Errorf("%s: got %+v, want %s/%v", c.name, d, c.want, c.allowed)
		}
	}
}

func TestMalformedWindowsNeverLock(t *testing.T) {
	for _, w := range []Window{
		{Days: []string{"wed"}, Start: "10:00", Stop: "10:00"},
		{Days: []string{"wed"}, Start: "25:00", Stop: "10:00"},
		{Days: []string{"wed"}, Start: "9:00", Stop: "10:00"},
	} {
		if w.Active(at("2026-10-21", "09:30")) {
			t.Errorf("%+v active", w)
		}
	}
}
