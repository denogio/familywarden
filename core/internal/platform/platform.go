// Package platform is the boundary between the router-agnostic core and the
// router it runs on (plans/app-control.md A18). The core never calls uci, ubus,
// fw4 or platform paths itself; an adapter (e.g. internal/openwrt) implements this.
package platform

import (
	"encoding/json"

	"github.com/denogio/familywarden/core/internal/usign"
)

// Profile is what the core needs to know about a profile it does not own yet.
type Profile struct {
	Name     string   `json:"name"`
	AppsRev  string   `json:"apps_rev"` // revision of the profile's app policy, "" = none
	Requires []string `json:"requires"` // phone features the profile's policy needs
}

// Context is one consistent snapshot of platform state.
type Context struct {
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
	Policy(profile string, version int) (Policy, error)
	RouterKey() (usign.SecretKey, error)
	LegacyDevices() ([]LegacyDevice, error)
	RemoveLegacyDevices() error
}
