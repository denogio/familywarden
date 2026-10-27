package openwrt

import (
	"strings"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestRenderNetworkIntentSchedulesTicketsAndRouterScope(t *testing.T) {
	now := time.Unix(1800000000, 0)
	c := rules.Configuration{Current: rules.ConfigurationData{
		Settings: rules.ConfigurationSettings{Enabled: "1", DefaultPolicy: "block"},
		Profiles: []rules.ConfigurationProfile{{ID: "alex", Enabled: "1", Policy: "scheduled", BlockRouter: "1", Devices: []string{"aa:bb:cc:00:00:01"}}},
		Schedules: []rules.ConfigurationWindow{
			{Profile: "alex", Days: "sat", Start: "22:00", Stop: "06:00"},
			{Profile: "alex", Days: "mon", Start: "00:00", Stop: "24:00"},
			{Profile: "alex", Days: "tue", Start: "08:00", Stop: "08:00"},
		},
	}}
	i, err := configuration.BuildNetworkIntent(c, now, map[string]platform.NetworkOverride{"alex": {Mode: "allow", Expires: now.Unix() + 60}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := renderNetworkIntent(i, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		"aa:bb:cc:00:00:01 timeout 50s",
		"meta hour >= 22:00 meta day { 6 }",
		"meta hour < 06:00 meta day { 0 }",
		"meta hour >= 00:00 meta day { 1 }",
		"meta day { 2 }",
		"tcp dport != 7700 counter drop",
		"iifname { @FAMILYWARDEN_LAN@ } ether saddr != { aa:bb:cc:00:00:01 }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "24:00") || strings.Contains(out, "override:allow") {
		t.Fatal(out)
	}
	raw, err = renderNetworkIntent(i, now.Add(60*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "timeout 0s") || strings.Contains(string(raw), "elements =") {
		t.Fatal(string(raw))
	}
}

func TestRenderNetworkIntentConsumesRulesNotProfilePolicy(t *testing.T) {
	i := platform.NetworkIntent{Format: platform.NetworkIntentFormat, Enabled: true,
		Profiles: []platform.ProfileIntent{{ID: "untrusted", Enabled: true, Policy: "block", Devices: []string{"aa:bb:cc:00:00:02"}}},
		Forward:  []platform.NetworkRule{{Profile: "alex", Devices: []string{"aa:bb:cc:00:00:01"}, Action: "drop", Reason: "override:block"}},
	}
	raw, err := renderNetworkIntent(i, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "untrusted") || !strings.Contains(string(raw), "alex:override:block") {
		t.Fatal(string(raw))
	}
	i.Format = "future"
	if _, err := renderNetworkIntent(i, time.Now()); err == nil {
		t.Fatal("foreign format accepted")
	}
}

func TestRenderNetworkIntentLockdown(t *testing.T) {
	i := platform.NetworkIntent{Format: platform.NetworkIntentFormat, Captain: &platform.CaptainIntent{ID: "alex", PactType: "ongoing", WindowDay: "sun", WindowStart: "10:00", WindowStop: "24:00"}}
	raw, err := renderNetworkIntent(i, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"iifname \"lo\" accept", "ct state established,related accept", "tcp dport { 53, 7700 } accept", "meta hour >= 10:00 meta day { 0 }", "meta l4proto tcp counter drop"} {
		if !strings.Contains(string(raw), want) {
			t.Fatal(want, string(raw))
		}
	}
	i.Captain.PactType = "until"
	i.Captain.Until = 1800001000
	raw, err = renderNetworkIntent(i, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "meta time < 1800001000") || strings.Contains(string(raw), "ulysses:window") {
		t.Fatal(string(raw))
	}
}
