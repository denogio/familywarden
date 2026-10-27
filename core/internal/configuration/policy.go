package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

type phonePact struct {
	Active    bool   `json:"active"`
	Type      string `json:"type"`
	Until     int64  `json:"until"`
	ReleaseAt int64  `json:"release_at"`
}

type phoneWindow struct {
	Days  string `json:"days"`
	Start string `json:"start"`
	Stop  string `json:"stop"`
}

type phoneRules struct {
	Enabled  string        `json:"enabled"`
	Policy   string        `json:"policy"`
	Override string        `json:"override"`
	Windows  []phoneWindow `json:"windows"`
}

// The document/kernel can express the exclusive endpoint 24:00; Android's
// LocalTime cannot. Normalize boundaries and kernel day tokens, preserving semantics and
// never turning the empty 24:00..00:00 interval into an all-day allow window.
func phoneWindowFor(w rules.ConfigurationWindow) (phoneWindow, bool) {
	out := phoneWindow{w.Days, w.Start, w.Stop}
	// Match kernel ASCII tokenization: broader Java whitespace must not
	// create phone allow-days that the router never generated.
	valid := []string{}
	invalid := false
	for _, day := range windowDays(w.Days) {
		if knownDay(day) {
			valid = append(valid, day)
		} else {
			invalid = true
		}
	}
	if len(valid) == 0 {
		return phoneWindow{}, false
	}
	if invalid {
		out.Days = strings.Join(valid, " ")
	}
	if w.Start == "24:00" {
		if w.Stop == "00:00" {
			return phoneWindow{}, false
		}
		out.Start = "00:00"
		if w.Stop != "24:00" {
			days := windowDays(out.Days)
			for i, day := range days {
				days[i] = nextWindowDay[day]
			}
			out.Days = strings.Join(days, " ")
		}
	}
	if w.Stop == "24:00" {
		out.Stop = "00:00"
	}
	return out, true
}

// Legacy ownership still needs mobile-valid clock values. Keep every other
// raw rule (including future fields) and leave ordinary policy bytes untouched.
func normalizePhoneWindows(p platform.Policy) (platform.Policy, error) {
	if len(p.Rules) == 0 || bytes.Equal(bytes.TrimSpace(p.Rules), []byte("null")) {
		return p, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(p.Rules, &raw); err != nil {
		return platform.Policy{}, err
	}
	var windows []json.RawMessage
	if len(raw["windows"]) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(raw["windows"], &windows); err != nil {
		return platform.Policy{}, err
	}
	out := []json.RawMessage{}
	changed := false
	for _, original := range windows {
		var w phoneWindow
		if err := json.Unmarshal(original, &w); err != nil {
			return platform.Policy{}, err
		}
		normalized, emit := phoneWindowFor(rules.ConfigurationWindow{Days: w.Days, Start: w.Start, Stop: w.Stop})
		if emit && normalized == w {
			out = append(out, original)
			continue
		}
		changed = true
		if !emit {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(original, &fields); err != nil {
			return platform.Policy{}, err
		}
		for key, value := range map[string]string{"days": normalized.Days, "start": normalized.Start, "stop": normalized.Stop} {
			fields[key], _ = json.Marshal(value)
		}
		window, err := json.Marshal(fields)
		if err != nil {
			return platform.Policy{}, err
		}
		out = append(out, window)
	}
	if !changed {
		return p, nil
	}
	var err error
	if raw["windows"], err = json.Marshal(out); err != nil {
		return platform.Policy{}, err
	}
	p.Rules, err = json.Marshal(raw)
	return p, err
}

// Captain is the profile a phone reply treats as captain right now: the one
// bound by an active pact, else the configured one.
func Captain(c rules.Configuration, now time.Time) string {
	if Active(c.Pact, now) {
		return c.Pact.Captain
	}
	return c.Current.Settings.Captain
}

// PhonePolicy replaces a platform policy's captain, "pact" and "rules" with the
// values computed from the authoritative document. The platform supplies facts
// only: the captain's manual override in Rules (and verified DNS, kept as is).
func PhonePolicy(c rules.Configuration, platformPolicy platform.Policy, id string, now time.Time) (platform.Policy, error) {
	out := platformPolicy
	captain := Captain(c, now)
	if captain != id {
		captain = ""
	}
	if captain != platformPolicy.Captain {
		return platform.Policy{}, errors.New("platform policy state diverged from the configuration")
	}
	override := "none"
	if captain != "" {
		var r struct {
			Override string `json:"override"`
		}
		if err := json.Unmarshal(platformPolicy.Rules, &r); err != nil || r.Override == "" {
			return platform.Policy{}, errors.New("platform override unavailable")
		}
		override = r.Override
	}
	pact := phonePact{}
	if captain != "" && Active(c.Pact, now) {
		pact = phonePact{true, c.Pact.Type, integer(c.Pact.Until), integer(c.Pact.ReleaseAt)}
	}
	r := phoneRules{Enabled: "0", Policy: "allow", Override: override, Windows: []phoneWindow{}}
	if captain != "" {
		eff := Effective(c, now)
		if p := profile(eff, captain); p != nil {
			r.Enabled, r.Policy = or(p.Enabled, "0"), or(p.Policy, "allow")
		}
		for _, w := range eff.Schedules {
			if w.Profile == captain && w.Days != "" && w.Start != "" && w.Stop != "" {
				if window, emit := phoneWindowFor(w); emit {
					r.Windows = append(r.Windows, window)
				}
			}
		}
	}
	var err error
	if out.Pact, err = json.Marshal(pact); err != nil {
		return platform.Policy{}, err
	}
	if out.Rules, err = json.Marshal(r); err != nil {
		return platform.Policy{}, err
	}
	return out, nil
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
