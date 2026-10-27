package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
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
// LocalTime cannot. Normalize only those windows, preserving day semantics and
// never turning the empty 24:00..00:00 interval into an all-day allow window.
func phoneWindowFor(w rules.ConfigurationWindow) (phoneWindow, bool) {
	out := phoneWindow{w.Days, w.Start, w.Stop}
	if w.Start == "24:00" {
		if w.Stop == "00:00" {
			return phoneWindow{}, false
		}
		out.Start = "00:00"
		if w.Stop != "24:00" {
			next := map[string]string{"sun": "mon", "mon": "tue", "tue": "wed", "wed": "thu", "thu": "fri", "fri": "sat", "sat": "sun"}
			days := strings.Fields(w.Days)
			for i, day := range days {
				days[i] = next[day]
			}
			out.Days = strings.Join(days, " ")
		}
	}
	if w.Stop == "24:00" {
		out.Stop = "00:00"
	}
	return out, true
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
// values computed from the authoritative document. The platform only supplies
// runtime facts: the captain's manual override (and DNS/app input, kept as is).
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
	if !sameJSON(out.Pact, platformPolicy.Pact) || !sameJSON(out.Rules, platformPolicy.Rules) {
		log.Printf("phone policy for %s: platform projection differs from the configuration; using the configuration", id)
	}
	return out, nil
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}
