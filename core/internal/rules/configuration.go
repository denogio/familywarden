package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Configuration is the complete portable configuration import/export boundary.
// Raw string options preserve absent versus explicit zero values during migration.
// Runtime counters, grants, credentials and router signing keys are not configuration.
type Configuration struct {
	Version  int                `json:"version"`
	Revision string             `json:"revision"`
	Current  ConfigurationData  `json:"current"`
	Pact     *ConfigurationPact `json:"pact"`
}

type ConfigurationData struct {
	Settings  ConfigurationSettings  `json:"settings"`
	Profiles  []ConfigurationProfile `json:"profiles"`
	Schedules []ConfigurationWindow  `json:"schedules"`
	Apps      []ConfigurationApp     `json:"apps"`
	Downtime  []ConfigurationWindow  `json:"downtime"`
	// Legacy snapshot associations are migration data, never a second registry.
	LegacyPhones []ConfigurationPhone `json:"legacy_phones"`
}

type ConfigurationPhone struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Profile       string `json:"profile"`
	PublicKey     string `json:"public_key"`
	Administrator string `json:"administrator"`
	Approves      string `json:"approves"`
}

type ConfigurationSettings struct {
	Enabled       string `json:"enabled"`
	DefaultPolicy string `json:"default_policy"`
	Captain       string `json:"captain"`
	DelayHours    string `json:"delay_hours"`
	DNSDomain     string `json:"dns_domain"`
	ResolverURL   string `json:"resolver_url"`
}

type ConfigurationProfile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Enabled       string   `json:"enabled"`
	Policy        string   `json:"policy"`
	BlockRouter   string   `json:"block_router"`
	Devices       []string `json:"devices"`
	DNS           string   `json:"dns"`
	ClientID      string   `json:"client_id"`
	Protections   []string `json:"protections"`
	Services      []string `json:"services"`
	Apps          string   `json:"apps"`
	SuspendNew    string   `json:"suspend_new"`
	ScreenMinutes string   `json:"screen_minutes"`
	ScreenWeekday string   `json:"screen_weekday"`
	ScreenWeekend string   `json:"screen_weekend"`
	AlwaysAllowed []string `json:"always_allowed"`
}

type ConfigurationWindow struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Days    string `json:"days"`
	Start   string `json:"start"`
	Stop    string `json:"stop"`
}

type ConfigurationApp struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Package string `json:"package"`
	Minutes string `json:"minutes"`
	Blocked string `json:"blocked"`
}

type ConfigurationDNS struct {
	Enabled     string   `json:"enabled"`
	ClientID    string   `json:"client_id"`
	Domain      string   `json:"domain"`
	Protections []string `json:"protections"`
	Services    []string `json:"services"`
}

type ConfigurationPact struct {
	Captain       string            `json:"captain"`
	Type          string            `json:"type"`
	Until         string            `json:"until"`
	WindowDay     string            `json:"window_day"`
	WindowStart   string            `json:"window_start"`
	WindowStop    string            `json:"window_stop"`
	DelaySeconds  string            `json:"delay_seconds"`
	DefaultPolicy string            `json:"default_policy"`
	ReleaseAt     string            `json:"release_at"`
	Frozen        ConfigurationData `json:"frozen"`
	DNS           ConfigurationDNS  `json:"dns"`
}

// Seal identifies the exact complete snapshot, including frozen rules and the
// release deadline. It never depends on time or rewrites compatibility values.
func (c *Configuration) Seal() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported configuration version %d", c.Version)
	}
	c.Revision = ""
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	c.Revision = hex.EncodeToString(h[:])
	return nil
}
