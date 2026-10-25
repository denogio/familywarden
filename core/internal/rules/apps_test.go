package rules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompileAppsPreservesWirePolicy(t *testing.T) {
	in := AppInput{
		Rules: "always|com.example.calls||\napp|com.example.blocked|5|1\napp|com.example.free||0\napp|com.example.video|0|0\ndown|mon tue|21:30|07:00\nnew|1||\nscreen|mon|60|\nscreen|sun|120|",
		Bonus: "bonus|com.example.video|15|2026-10-25\nbonus|total|30|2026-10-25",
	}
	p, requires, err := CompileApps(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requires, []string{"apps-v1", "screen-time-v1"}) {
		t.Fatal(requires)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	// This wire fixture includes the legacy empty arrays and a zero-minute limit.
	want := `{"rev":"4afce71f23b3eed9","suspend_new":true,"limits":[{"package":"com.example.video","minutes":0}],"blocked":["com.example.blocked"],"approved":["com.example.free","com.example.video"],"downtime":[{"days":"mon tue","start":"21:30","stop":"07:00"}],"screen":{"mon":60,"sun":120},"always_allowed":["com.example.calls"],"bonus":{"day":"2026-10-25","total":30,"apps":[{"package":"com.example.video","minutes":15}]}}`
	if string(data) != want {
		t.Fatalf("wire policy changed:\n%s\nwant:\n%s", data, want)
	}
}

func TestCompileAppsOffAndFeatureRequirements(t *testing.T) {
	p, requires, err := CompileApps(AppInput{})
	if err != nil || p != nil || len(requires) != 0 {
		t.Fatal(p, requires, err)
	}
	for _, test := range []struct {
		rules string
		bonus string
		count int
	}{
		{"new|0||", "", 1},
		{"app|com.example.video|1440|0\nnew|1||", "", 1},
		{"down|sun|00:00|00:00\nnew|1||", "", 2},
		{"always|com.example.calls||\nnew|1||", "", 2},
		{"new|1||\nscreen|sat|0|", "", 2},
		{"new|1||", "bonus|total|0|2026-10-25", 2},
	} {
		p, requires, err := CompileApps(AppInput{test.rules, test.bonus})
		if err != nil || p == nil || len(requires) != test.count {
			t.Fatal(test, p, requires, err)
		}
		if test.rules == "new|0||" && (p.SuspendNew || len(p.Rev) != 16) {
			t.Fatal(p)
		}
	}
}

func TestCompileAppsRejectsMalformedInputsRatherThanWeakerPolicy(t *testing.T) {
	for _, lines := range []string{
		"app|com.example.video|1|0", // no suspend-new rule
		"new|2||", "new|0||\nnew|1||", "new|1||\n", "new|1||\napp|com.example.video|1|0",
		"app|com.example.video|-1|0\nnew|1||", "app|com.example.video|1441|0\nnew|1||",
		"app|com.example.video|1|2\nnew|1||", "app|../bad|1|0\nnew|1||",
		"app|com.example..bad|1|0\nnew|1||", "app|com.example.video|1|0\napp|com.example.video|2|0\nnew|1||",
		"down|bad|21:00|07:00\nnew|1||", "down|mon|24:00|07:00\nnew|1||",
		"new|1||\nscreen|bad|60|", "new|1||\nscreen|mon|60|\nscreen|mon|90|",
		"new|1||\nscreen|mon|99999|", "always|com.example.calls|oops|\nnew|1||", "future|restriction||\nnew|1||",
	} {
		if p, _, err := CompileApps(AppInput{Rules: lines}); err == nil || p != nil {
			t.Fatalf("accepted %q: %+v", lines, p)
		}
	}
	for _, bonus := range []string{
		"bonus|total|-1|2026-10-25", "bonus|total|30|2026-02-30", "bonus|../bad|30|2026-10-25",
		"bonus|total|15|2026-10-25\nbonus|total|30|2026-10-25",
		"bonus|com.example.video|15|2026-10-24\nbonus|total|30|2026-10-25",
		"bonus|total|30|2026-10-25\n", "other|total|30|2026-10-25",
	} {
		if p, _, err := CompileApps(AppInput{Rules: "new|1||", Bonus: bonus}); err == nil || p != nil {
			t.Fatalf("accepted bonus %q", bonus)
		}
	}
	if _, _, err := CompileApps(AppInput{Bonus: "bonus|total|30|2026-10-25"}); err == nil {
		t.Fatal("bonus accepted when app control is off")
	}
	// A change to a limit, approval, downtime, bonus or day must change the revision.
	base, _, _ := CompileApps(AppInput{Rules: "new|1||"})
	for _, line := range []string{"app|com.example.video|30|0", "always|com.example.calls||", "down|mon|21:00|07:00"} {
		p, _, err := CompileApps(AppInput{Rules: line + "\nnew|1||"})
		if err != nil || p.Rev == base.Rev || strings.TrimSpace(p.Rev) != p.Rev {
			t.Fatal(line, p, err)
		}
	}
}
