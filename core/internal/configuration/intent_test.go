package configuration

import (
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// Friday 2027-01-15 08:00 UTC; the fixture's alex window (Mon 07:00-20:00) is
// closed at this instant.
func intentNow() time.Time { return time.Unix(1800000000, 0).UTC() }

func findIntentProfile(t *testing.T, i platform.NetworkIntent, id string) *platform.ProfileIntent {
	t.Helper()
	for j := range i.Profiles {
		if i.Profiles[j].ID == id {
			return &i.Profiles[j]
		}
	}
	t.Fatalf("intent profile %q missing: %+v", id, i.Profiles)
	return nil
}

func TestNetworkIntentBasicShape(t *testing.T) {
	i, err := BuildNetworkIntent(sample(), intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.Format != platform.NetworkIntentFormat || !i.Enabled || i.DefaultPolicy != "block" {
		t.Fatal(i)
	}
	if len(i.AssignedMACs) != 1 || i.AssignedMACs[0] != "aa:bb:cc:00:00:01" {
		t.Fatal(i.AssignedMACs)
	}
	if len(i.Profiles) != 2 || len(i.Tickets) != 0 || i.Captain != nil {
		t.Fatal(i)
	}
	alex := findIntentProfile(t, i, "alex")
	if !alex.Enabled || alex.Policy != "scheduled" || alex.BlockRouter || alex.Override != "none" || len(alex.Devices) != 1 {
		t.Fatal(*alex)
	}
	if len(alex.Windows) != 1 || alex.Windows[0].Days[0] != "mon" || alex.Windows[0].Start != "07:00" || alex.Windows[0].Stop != "20:00" {
		t.Fatal(alex.Windows)
	}
	taylor := findIntentProfile(t, i, "taylor")
	if taylor.Enabled || taylor.Policy != "allow" {
		t.Fatal(*taylor)
	}
}

func TestNetworkIntentOverrideClassification(t *testing.T) {
	overrides := map[string]platform.NetworkOverride{
		"alex":    {Mode: "block"},
		"taylor":  {Mode: "allow"},
		"taylor2": {Mode: "allow", Expires: 1800003600},
	}
	c := sample()
	c.Current.Profiles = append(c.Current.Profiles,
		rules.ConfigurationProfile{ID: "taylor2", Name: "T2", Enabled: "1", Devices: []string{"aa:bb:cc:00:00:02"}},
	)
	i, err := BuildNetworkIntent(c, intentNow(), overrides)
	if err != nil {
		t.Fatal(err)
	}
	if got := findIntentProfile(t, i, "alex").Override; got != "block" {
		t.Fatal("alex", got)
	}
	if got := findIntentProfile(t, i, "taylor").Override; got != "allow" {
		t.Fatal("taylor", got)
	}
	if got := findIntentProfile(t, i, "taylor2").Override; got != "none" {
		t.Fatal("taylor2", got)
	}
	if len(i.Tickets) != 1 || i.Tickets[0].Mac != "aa:bb:cc:00:00:02" || i.Tickets[0].Expires != 1800003600 {
		t.Fatal(i.Tickets)
	}
}

func TestNetworkIntentTimedAllowExpiryAtReadTime(t *testing.T) {
	cases := map[string]platform.NetworkOverride{
		"past":      {Mode: "allow", Expires: 1799999999},
		"exact-now": {Mode: "allow", Expires: 1800000000},
		"future":    {Mode: "allow", Expires: 1800000001},
	}
	for name, o := range cases {
		i, err := BuildNetworkIntent(sample(), intentNow(), map[string]platform.NetworkOverride{"alex": o})
		if err != nil {
			t.Fatal(name, err)
		}
		if got := findIntentProfile(t, i, "alex").Override; got != "none" {
			t.Fatal(name, got)
		}
		if name == "future" {
			if len(i.Tickets) != 1 || i.Tickets[0].Mac != "aa:bb:cc:00:00:01" {
				t.Fatal(name, i.Tickets)
			}
		} else if len(i.Tickets) != 0 {
			t.Fatal(name, i.Tickets)
		}
	}
}

func TestNetworkIntentTicketsRequireEnabled(t *testing.T) {
	c := sample()
	c.Current.Profiles = append(c.Current.Profiles,
		rules.ConfigurationProfile{ID: "kid", Name: "Kid", Enabled: "0", Devices: []string{"aa:bb:cc:00:00:09"}},
	)
	i, err := BuildNetworkIntent(c, intentNow(), map[string]platform.NetworkOverride{"kid": {Mode: "allow", Expires: 1800003600}})
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Tickets) != 0 {
		t.Fatal(i.Tickets)
	}

	c = sample()
	c.Current.Settings.Enabled = "0"
	i, err = BuildNetworkIntent(c, intentNow(), map[string]platform.NetworkOverride{"alex": {Mode: "allow", Expires: 1800003600}})
	if err != nil {
		t.Fatal(err)
	}
	if i.Enabled || len(i.Tickets) != 0 {
		t.Fatal(i)
	}
}

func TestNetworkIntentBoundCaptainAllowsNeverRender(t *testing.T) {
	for _, o := range []platform.NetworkOverride{{Mode: "allow"}, {Mode: "allow", Expires: 1800003600}} {
		c := sample()
		c.Pact = &rules.ConfigurationPact{
			Captain: "alex", Type: "ongoing", Until: "0",
			WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00",
			Frozen: frozen(c.Current, "alex"),
		}
		i, err := BuildNetworkIntent(c, intentNow(), map[string]platform.NetworkOverride{"alex": o})
		if err != nil {
			t.Fatal(err)
		}
		if got := findIntentProfile(t, i, "alex").Override; got != "none" {
			t.Fatal(o, got)
		}
		if len(i.Tickets) != 0 {
			t.Fatal(o, i.Tickets)
		}
	}
}

func TestNetworkIntentCaptainLockdown(t *testing.T) {
	mk := func(policy, enabled string, o platform.NetworkOverride) (platform.NetworkIntent, error) {
		c := sample()
		c.Current.Profiles[0].Policy = policy
		c.Current.Profiles[0].Enabled = enabled
		c.Pact = &rules.ConfigurationPact{
			Captain: "alex", Type: "ongoing", Until: "0",
			WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00",
			Frozen: frozen(c.Current, "alex"),
		}
		var overrides map[string]platform.NetworkOverride
		if o.Mode != "" {
			overrides = map[string]platform.NetworkOverride{"alex": o}
		}
		return BuildNetworkIntent(c, intentNow(), overrides)
	}

	cases := []struct {
		name     string
		policy   string
		enabled  string
		override platform.NetworkOverride
		locked   bool
		windows  int
	}{
		{"scheduled no override locks with windows", "scheduled", "1", platform.NetworkOverride{Mode: "none"}, true, 1},
		{"scheduled with manual block locks without windows", "scheduled", "1", platform.NetworkOverride{Mode: "block"}, true, 0},
		{"block policy locks", "block", "1", platform.NetworkOverride{Mode: "none"}, true, 0},
		{"allow policy never locks", "allow", "1", platform.NetworkOverride{Mode: "none"}, false, 0},
		{"absent policy defaults to allow", "", "1", platform.NetworkOverride{Mode: "none"}, false, 0},
		{"allow policy with manual block locks", "allow", "1", platform.NetworkOverride{Mode: "block"}, true, 0},
		{"disabled captain never locks", "block", "0", platform.NetworkOverride{Mode: "none"}, false, 0},
	}
	for _, tc := range cases {
		i, err := mk(tc.policy, tc.enabled, tc.override)
		if err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.locked {
			if i.Captain == nil {
				t.Fatal(tc.name, "expected lockdown", i)
			}
			if i.Captain.PactType != "ongoing" || i.Captain.Until != 0 || i.Captain.WindowDay != "sun" {
				t.Fatal(tc.name, *i.Captain)
			}
			if len(i.Captain.Windows) != tc.windows {
				t.Fatal(tc.name, i.Captain.Windows)
			}
		} else if i.Captain != nil {
			t.Fatal(tc.name, "unexpected lockdown", i.Captain)
		}
	}
}

func TestNetworkIntentUntilPactDeadline(t *testing.T) {
	c := sample()
	c.Pact = &rules.ConfigurationPact{
		Captain: "alex", Type: "until", Until: "1800001000",
		WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00",
		Frozen: frozen(c.Current, "alex"),
	}
	i, err := BuildNetworkIntent(c, intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.Captain == nil || i.Captain.Until != 1800001000 || i.Captain.PactType != "until" {
		t.Fatal(i.Captain)
	}

	// An expired until-pact is inactive: no lockdown, no frozen settings.
	c.Pact.Until = "1799999000"
	i, err = BuildNetworkIntent(c, intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.Captain != nil || i.DefaultPolicy != "block" {
		t.Fatal(i)
	}
}

func TestNetworkIntentPactFreezesSettingsAndStripsCaptain(t *testing.T) {
	c := sample()
	c.Current.Settings.Enabled = "0"
	c.Current.Settings.DefaultPolicy = "allow"
	// Taylor borrows the captain's MAC plus a real one.
	c.Current.Profiles[1].Devices = []string{"aa:bb:cc:00:00:01", "aa:bb:cc:00:00:02"}
	c.Pact = &rules.ConfigurationPact{
		Captain: "alex", Type: "ongoing", Until: "0",
		WindowDay: "sun", WindowStart: "10:00", WindowStop: "11:00",
		DefaultPolicy: "block",
		Frozen:        frozen(c.Current, "alex"),
	}
	i, err := BuildNetworkIntent(c, intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !i.Enabled || i.DefaultPolicy != "block" {
		t.Fatal("pact freeze not applied", i)
	}
	if len(i.AssignedMACs) != 2 {
		t.Fatal(i.AssignedMACs)
	}
	taylor := findIntentProfile(t, i, "taylor")
	if len(taylor.Devices) != 1 || taylor.Devices[0] != "aa:bb:cc:00:00:02" {
		t.Fatal("captain MAC not stripped from other profile", taylor.Devices)
	}
}

func TestNetworkIntentWindowNormalization(t *testing.T) {
	c := sample()
	c.Current.Schedules = []rules.ConfigurationWindow{
		{ID: "s1", Profile: "alex", Days: "mon FROB wed", Start: "07:00", Stop: "20:00"},
		{ID: "s2", Profile: "alex", Days: "xyz", Start: "08:00", Stop: "09:00"},
		{ID: "s3", Profile: "alex", Days: "tue", Start: "08:00", Stop: "25:00"},
		{ID: "s4", Profile: "alex", Days: "tue", Start: "bad", Stop: "09:00"},
		{ID: "s5", Profile: "alex", Days: "wed", Start: "08:00", Stop: "08:00"},
		{ID: "s6", Profile: "alex", Days: "thu", Start: "22:00", Stop: "06:00"},
	}
	i, err := BuildNetworkIntent(c, intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := findIntentProfile(t, i, "alex").Windows
	if len(w) != 3 {
		t.Fatal(w)
	}
	if len(w[0].Days) != 2 || w[0].Days[0] != "mon" || w[0].Days[1] != "wed" {
		t.Fatal(w[0])
	}
	if w[1].Days[0] != "wed" || w[1].Start != "08:00" || w[1].Stop != "08:00" {
		t.Fatal(w[1])
	}
	if w[2].Days[0] != "thu" {
		t.Fatal(w[2])
	}
}

func TestNetworkIntentOrderedRulesAndIdentity(t *testing.T) {
	c := sample()
	c.Revision = "fixture-revision"
	c.Current.Profiles[0].BlockRouter = "1"
	c.Current.Profiles = append(c.Current.Profiles, rules.ConfigurationProfile{ID: "sam", Enabled: "1", Policy: "block", Devices: []string{"aa:bb:cc:00:00:02"}})
	overrides := map[string]platform.NetworkOverride{"alex": {Mode: "block"}, "sam": {Mode: "allow"}}
	i, err := BuildNetworkIntent(c, intentNow(), overrides)
	if err != nil {
		t.Fatal(err)
	}
	// Every override precedes every policy, then scheduled accepts precede drops.
	want := []string{"override:block", "override:allow", "block", "sched-allow", "scheduled"}
	if len(i.Forward) != len(want) {
		t.Fatal(i.Forward)
	}
	for j, r := range i.Forward {
		if r.Reason != want[j] {
			t.Fatal(i.Forward)
		}
	}
	if len(i.Router) != 3 || i.Router[0].Reason != "override:allow" || i.Router[1].Reason != "sched-allow" || !i.Router[2].Recovery {
		t.Fatal(i.Router)
	}
	same, err := BuildNetworkIntent(c, intentNow(), overrides)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.ID) != 64 || i.ID != same.ID || i.Revision != c.Revision {
		t.Fatal(i.ID, same.ID, i.Revision)
	}
	overrides["alex"] = platform.NetworkOverride{Mode: "none"}
	changed, err := BuildNetworkIntent(c, intentNow(), overrides)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == i.ID || changed.Revision != i.Revision {
		t.Fatal("volatile change needs a distinct intent, not a new configuration revision")
	}
}

func TestNetworkApplicationAcknowledgementIsNotDesiredPolicy(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	first := a.applied
	if first == nil || first.State != "acknowledged" || len(first.IntentID) != 64 {
		t.Fatal(first)
	}
	okResult(t, a.API("configuration_block", []byte(`{"profile":"alex"}`)))
	second := a.applied
	if second == nil || second.IntentID == first.IntentID || second.Revision != first.Revision {
		t.Fatal(first, second)
	}
	r := a.API("configuration_status", nil)
	if r["network_application"] != second {
		t.Fatal(r)
	}
	b.networkFail = true
	if r := a.API("configuration_allow", []byte(`{"profile":"alex"}`)); r["error"] == nil {
		t.Fatal(r)
	}
	if a.applied != second {
		t.Fatal("failed intent was acknowledged")
	}
	if r := a.API("configuration_status", nil); r["error"] == nil {
		t.Fatal("uncertain enforcement reported as healthy", r)
	}
}

func TestNetworkIntentAbsentGlobalToggleKeepsLegacyDefault(t *testing.T) {
	c := sample()
	c.Current.Settings.Enabled = ""
	i, err := BuildNetworkIntent(c, intentNow(), nil)
	if err != nil || !i.Enabled || len(i.Forward) == 0 {
		t.Fatal(i, err)
	}
	row, err := profileNetworkRow(c, Effective(c, intentNow()), c.Current.Profiles[0], platform.NetworkOverride{Mode: "none"}, intentNow(), 0)
	if err != nil || row["status"] != "blocked" {
		t.Fatal(row, err)
	}
}

func TestNetworkIntentInvalidInput(t *testing.T) {
	c := sample()
	c.Current.Settings.DefaultPolicy = "weird"
	if _, err := BuildNetworkIntent(c, intentNow(), nil); err == nil {
		t.Fatal("expected invalid default policy error")
	}
	c = sample()
	if _, err := BuildNetworkIntent(c, intentNow(), map[string]platform.NetworkOverride{"alex": {Mode: "banana"}}); err == nil {
		t.Fatal("expected invalid override error")
	}
}

func TestNetworkIntentKeepsMacCaseFromConfiguration(t *testing.T) {
	c := sample()
	c.Current.Profiles[0].Devices = []string{"AA:BB:CC:00:00:01"}
	i, err := BuildNetworkIntent(c, intentNow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.AssignedMACs) != 1 || i.AssignedMACs[0] != "AA:BB:CC:00:00:01" {
		t.Fatal(i.AssignedMACs)
	}
	alex := findIntentProfile(t, i, "alex")
	if alex.Devices[0] != "AA:BB:CC:00:00:01" {
		t.Fatal(alex.Devices)
	}
}
