// Package platform is the boundary between the router-agnostic core and the
// router it runs on (plans/app-control.md A18). The core never calls uci, ubus,
// fw4 or platform paths itself; an adapter (e.g. internal/openwrt) implements this.
package platform

import (
	"encoding/json"

	"github.com/denogio/familywarden/core/internal/rules"
	"github.com/denogio/familywarden/core/internal/usign"
)

// Profile is what the core needs to know about a profile it does not own yet.
type Profile struct {
	Name     string           `json:"name"`
	AppsRev  string           `json:"apps_rev"`            // computed by the core, "" = app control off
	Requires []string         `json:"requires"`            // computed by the core
	AppInput *rules.AppConfig `json:"app_input,omitempty"` // interim effective configuration
}

// Context is one consistent snapshot of platform state.
type Context struct {
	// Legacy bonus import is a versioned snapshot under the configuration lock.
	// Empty input is meaningful only when BonusFormat explicitly matches.
	BonusFormat string             `json:"bonus_format,omitempty"`
	LegacyBonus string             `json:"legacy_bonus,omitempty"`
	AppFormat   string             `json:"app_format"`   // versioned interim compiler input
	TimeZone    string             `json:"timezone"`     // IANA name or ""
	PactCaptain string             `json:"pact_captain"` // profile bound by an active pact, or ""
	Profiles    map[string]Profile `json:"profiles"`     // effective profiles
}

// Policy is the policy content of one profile for a phone reply. Interim: it
// comes from the platform until profiles and pacts move to the core (step 3).
type Policy struct {
	Context Context         `json:"context"`
	Captain string          `json:"captain"` // the profile id if it is the captain, else ""
	Pact    json.RawMessage `json:"pact"`
	Rules   json.RawMessage `json:"rules"`
	DNS     json.RawMessage `json:"dns,omitempty"`
	Apps    json.RawMessage `json:"apps,omitempty"`
}

// LegacyDevice is an approved phone from the platform's old configuration,
// imported once into the core's own store (A19).
type LegacyDevice struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Profile       string `json:"profile"`
	PubKey        string `json:"pubkey"`
	Administrator string `json:"administrator"`
	Approves      string `json:"approves"`
}

type Adapter interface {
	Context() (Context, error)
	// Configuration exports configured and frozen state together for migration.
	// It must not replace a frozen pact with the current editable profile.
	Configuration() (rules.Configuration, error)
	// WithContext serializes a core mutation with participating platform pact/state
	// writers. The callback runs exactly once after a successful snapshot, while
	// the platform lock is held; errors before it runs must not mutate anything.
	// Callers take their core mutex first. Callbacks must not call the adapter;
	// enforcement/apply work runs after this method releases the platform lock.
	WithContext(func(Context) error) error
	Policy(profile string, version int) (Policy, error)
	RouterKey() (usign.SecretKey, error)
	LegacyDevices() ([]LegacyDevice, error)
	RemoveLegacyDevices() error
	// EnsureTicket keeps the profile's home internet open at least until the epoch
	// (an approved grant, D3). It never shortens a ticket and never overrides a
	// manual block or a pact; ok=false reports that nothing was opened.
	EnsureTicket(profile string, until int64) (ok bool, err error)
}
