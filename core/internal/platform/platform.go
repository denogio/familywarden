// Package platform is the boundary between the router-agnostic core and the
// router it runs on (plans/app-control.md A18). The core never calls uci, ubus,
// fw4 or platform paths itself; an adapter (e.g. internal/openwrt) implements this.
package platform

import (
	"encoding/json"

	"github.com/denogio/familywarden/core/internal/rules"
	"github.com/denogio/familywarden/core/internal/usign"
)

// NetworkOverride is platform-backed volatile state. Expires=0 means no deadline;
// eligibility and additive-ticket semantics belong to core, not the adapter.
type NetworkOverride struct {
	Mode    string `json:"mode"`
	Expires int64  `json:"expires"`
}

// NetworkIntent is the versioned, renderer-ready description of the desired
// firewall state. The core computes it from its authoritative document plus
// the platform's volatile overrides; the adapter renders and applies it
// (native rule syntax, atomic apply, validation) without re-deriving
// eligibility. Every product decision the native renderer used to make from
// UCI/RAM/pact files lives in the core; a renderer may only translate these
// fields into its platform's rule syntax.
const NetworkIntentFormat = "network-intent-v1"

type NetworkIntent struct {
	Format   string `json:"format"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	// Enabled is the global toggle after any pact freeze; false keeps the
	// platform's chains but with no rules.
	Enabled bool `json:"enabled"`
	// DefaultPolicy is "allow" or "block" (never empty): what to do with
	// forwarded LAN traffic from MACs not assigned to any profile.
	DefaultPolicy string          `json:"default_policy"`
	AssignedMACs  []string        `json:"assigned_macs"`
	Profiles      []ProfileIntent `json:"profiles"`
	// Ordered decisions; adapters translate these without policy eligibility checks.
	Forward []NetworkRule `json:"forward"`
	Router  []NetworkRule `json:"router"`
	// Tickets are the only active timed allows; the platform enforces them
	// with native expiry (e.g. a timeout set) and derives the remaining
	// seconds from Expires at render time.
	Tickets []TicketIntent `json:"tickets"`
	// Captain is set only when the core has decided the router must be in
	// Ulysses lockdown right now; nil means an empty lockdown chain.
	Captain *CaptainIntent `json:"captain,omitempty"`
}

// ProfileIntent is one profile's renderer slice. Override is the permanent
// classification decided by the core: "none", "allow" (no deadline) or
// "block"; timed allows are Tickets, never here. Windows are the profile's
// allow windows; a scheduled profile with no windows is blocked 24/7.
type ProfileIntent struct {
	ID          string   `json:"id"`
	Enabled     bool     `json:"enabled"`
	Policy      string   `json:"policy"`
	BlockRouter bool     `json:"block_router"`
	Devices     []string `json:"devices"`
	Override    string   `json:"override"`
	Windows     []Window `json:"windows"`
}

// NetworkRule is an ordered decision for a set of source devices. Windows
// constrain an accept; no windows means unconditional. Router rules use TCP
// only; Recovery exempts the protected-release service from a router drop.
type NetworkRule struct {
	Profile  string   `json:"profile"`
	Devices  []string `json:"devices"`
	Windows  []Window `json:"windows,omitempty"`
	Action   string   `json:"action"`
	Reason   string   `json:"reason"`
	Recovery bool     `json:"recovery,omitempty"`
}

type TicketIntent struct {
	Mac     string `json:"mac"`
	Expires int64  `json:"expires"`
}

// CaptainIntent is the renderer slice of an active Ulysses lockdown. Devices
// lists the captain's MACs; Windows (possibly empty) are the allow windows the
// renderer must accept inside the lockdown chain for any source. PactType is
// "until" or "ongoing": Until (epoch, 0 for ongoing) bounds the until-pact
// drop, and WindowDay/WindowStart/WindowStop are the ongoing amendment window.
type CaptainIntent struct {
	ID          string   `json:"id"`
	Devices     []string `json:"devices"`
	Windows     []Window `json:"windows"`
	PactType    string   `json:"pact_type"`
	Until       int64    `json:"until"`
	WindowDay   string   `json:"window_day"`
	WindowStart string   `json:"window_start"`
	WindowStop  string   `json:"window_stop"`
}

// Window is one schedule entry in renderer-ready form: lowercase three-letter
// day names, HH:MM boundaries. start==stop allows the whole listed days;
// start>stop crosses midnight onto the following days; 24:00 is a valid
// exclusive stop. Native boundary translation is the renderer's job.
type Window struct {
	Days  []string `json:"days"`
	Start string   `json:"start"`
	Stop  string   `json:"stop"`
}

// Profile is what the core needs to know about a profile it does not own yet.
type Profile struct {
	Name     string           `json:"name"`
	AppsRev  string           `json:"apps_rev"`            // computed by the core, "" = app control off
	Requires []string         `json:"requires"`            // computed by the core
	AppInput *rules.AppConfig `json:"app_input,omitempty"` // interim effective configuration (legacy mode)
	// Owned-mode runtime facts: the profile's volatile manual override and its
	// verified DNS data. Absent in the legacy (policy-shaped) context.
	Override string          `json:"override,omitempty"`
	DNS      json.RawMessage `json:"dns,omitempty"`
}

// Context is one consistent snapshot of platform state.
type Context struct {
	NetworkIntentFormat string `json:"network_intent_format,omitempty"`
	NetworkFormat       string `json:"network_format,omitempty"`
	PactNetworkFormat   string `json:"pact_network_format,omitempty"`
	// Legacy bonus import is a versioned snapshot under the configuration lock.
	// Empty input is meaningful only when BonusFormat explicitly matches.
	BonusFormat string `json:"bonus_format,omitempty"`
	LegacyBonus string `json:"legacy_bonus,omitempty"`
	AppFormat   string `json:"app_format"`   // versioned interim compiler input
	TimeZone    string `json:"timezone"`     // IANA name or ""
	PactCaptain string `json:"pact_captain"` // profile bound by an active pact, or ""
	// MastCaptain is the platform's current captain: an active pact's captain,
	// else the platform's configured one. Owned-mode facts only.
	MastCaptain string             `json:"mast_captain,omitempty"`
	Profiles    map[string]Profile `json:"profiles"` // effective profiles
	// MastLastSync is the router's last signed-payload fetch epoch: informational
	// only, never confirmed phone enforcement. Core fills it from platform state.
	MastLastSync int64 `json:"mast_last_sync,omitempty"`
	// RuntimeFormat marks the owned-mode facts envelope (no policy content).
	RuntimeFormat string `json:"runtime_format,omitempty"`
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

// DNSRuntime contains native secrets and one-time legacy import data, not
// desired policy. The core owns acknowledgements, ownership and retry state.
type DNSRuntime struct {
	Auth        string
	LegacyOwned []string
	LegacyAcks  []string
}

// DNSPlatform serializes legacy cutover/secret access with native DNS writers.
// The callback must not call the adapter again; no socket callbacks are allowed.
type DNSPlatform interface {
	DNSTransaction(func(DNSRuntime) error) error
	SetDNSCredentials(username, password string) error
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
	// Facts is the owned-mode read: versioned handshakes and platform facts
	// (timezone, captains) plus per-profile volatile facts (manual override,
	// verified DNS). It carries no policy content: the authoritative
	// configuration document supplies that.
	Facts() (Context, error)
	// FactsContext serializes an owned-mode mutation with the platform's writers:
	// it holds the platform state lock, reads Facts under it and runs the
	// callback exactly once while the lock is held. The callback must not call
	// the adapter again. Callers take their core mutex first.
	FactsContext(func(Context) error) error
	RouterKey() (usign.SecretKey, error)
	LegacyDevices() ([]LegacyDevice, error)
	RemoveLegacyDevices() error
	// EnsureTicket keeps the profile's home internet open at least until the epoch
	// (an approved grant, D3). It never shortens a ticket and never overrides a
	// manual block or a pact; ok=false reports that nothing was opened.
	EnsureTicket(profile string, until int64) (ok bool, err error)
}
