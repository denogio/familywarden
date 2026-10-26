package rules

import (
	"encoding/json"
	"testing"
)

func TestConfigurationRevisionIncludesConfiguredFrozenAndReleaseState(t *testing.T) {
	c := Configuration{Version: 1, Current: ConfigurationData{
		Profiles:  []ConfigurationProfile{{ID: "alex", Name: "Alex", Enabled: "0", ClientID: "fw-existing"}},
		Schedules: []ConfigurationWindow{{ID: "cfg123456", Profile: "alex", Days: "fri", Start: "22:00", Stop: "06:00"}},
		Apps:      []ConfigurationApp{{ID: "app1", Profile: "alex", Package: "com.example.video", Minutes: "0"}},
	}, Pact: &ConfigurationPact{Captain: "alex", Type: "ongoing", DelaySeconds: "86400", ReleaseAt: "1800000000",
		Frozen: ConfigurationData{Profiles: []ConfigurationProfile{{ID: "alex", Enabled: "1", Devices: []string{"aa:bb:cc:00:00:01"}}}},
		DNS:    ConfigurationDNS{Enabled: "1", ClientID: "fw-existing", Domain: "dns.example.invalid", Protections: []string{"filter_lists"}},
	}}
	if err := c.Seal(); err != nil {
		t.Fatal(err)
	}
	original := c.Revision
	b, _ := json.Marshal(c)
	for _, change := range []func(*Configuration){
		func(c *Configuration) { c.Current.Profiles[0].Enabled = "1" },
		func(c *Configuration) { c.Current.Apps[0].Minutes = "" },
		func(c *Configuration) { c.Current.Schedules[0].ID = "cfg654321" },
		func(c *Configuration) { c.Pact.Frozen.Profiles[0].Devices = nil },
		func(c *Configuration) { c.Pact.DNS.ClientID = "different" },
		func(c *Configuration) { c.Pact.ReleaseAt = "1800000001" },
	} {
		var edited Configuration
		if err := json.Unmarshal(b, &edited); err != nil {
			t.Fatal(err)
		}
		change(&edited)
		if err := edited.Seal(); err != nil || edited.Revision == original {
			t.Fatal("revision omitted configuration state", err)
		}
	}
	if err := c.Seal(); err != nil || c.Revision != original {
		t.Fatal("unchanged configuration changed revision", err)
	}
	c.Version = 2
	if err := c.Seal(); err == nil {
		t.Fatal("future configuration schema accepted")
	}
}
