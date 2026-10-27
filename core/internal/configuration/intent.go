package configuration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// BuildNetworkIntent computes the versioned, renderer-ready network state from
// the authoritative document and the platform's volatile overrides. Every
// product decision the native renderer used to make from UCI, RAM and pact
// files now lives here: the effective (pact-frozen) configuration, which
// overrides become permanent rules versus expiring tickets, and whether the
// router needs a Ulysses lockdown chain. The adapter translates and applies
// native rules under its transaction contract; it never re-derives eligibility.
func BuildNetworkIntent(c rules.Configuration, now time.Time, overrides map[string]platform.NetworkOverride) (platform.NetworkIntent, error) {
	eff := Effective(c, now)
	nowUnix := now.Unix()

	defaultPolicy := eff.Settings.DefaultPolicy
	if defaultPolicy == "" {
		defaultPolicy = "allow"
	}
	if defaultPolicy != "allow" && defaultPolicy != "block" {
		return platform.NetworkIntent{}, fmt.Errorf("invalid default policy %q", defaultPolicy)
	}

	pactActive := c.Pact != nil && Active(c.Pact, now)
	captainID := ""
	if pactActive {
		captainID = c.Pact.Captain
	}

	intent := platform.NetworkIntent{
		Format:        platform.NetworkIntentFormat,
		Revision:      c.Revision,
		Enabled:       or(eff.Settings.Enabled, "1") == "1",
		DefaultPolicy: defaultPolicy,
		Profiles:      []platform.ProfileIntent{},
		Tickets:       []platform.TicketIntent{},
	}
	// assigned maps lower-case MACs to the spelling the effective
	// configuration uses, so rendered rules keep the historical case.
	assigned := map[string]string{}

	for _, p := range eff.Profiles {
		// The bound captain's allows are refused by the core; a stale one must
		// never reach the renderer (mirrors the legacy _is_captain_bound).
		bound := pactActive && p.ID == captainID
		o, err := EffectiveNetworkOverride(normalizeOverride(overrides[p.ID]), now)
		if err != nil {
			return platform.NetworkIntent{}, err
		}
		override := "none"
		switch o.Mode {
		case "block":
			override = "block"
		case "allow":
			if !bound {
				if o.Expires > 0 {
					// A timed allow is a ticket the platform expires natively.
					// Expired tickets never appear: the kernel already dropped
					// the element and the core reports the profile as unoverridden.
					if intent.Enabled && p.Enabled == "1" && o.Expires > nowUnix {
						for _, mac := range p.Devices {
							intent.Tickets = append(intent.Tickets, platform.TicketIntent{Mac: mac, Expires: o.Expires})
						}
					}
				} else {
					// Permanent allow (no deadline) is a static rule.
					override = "allow"
				}
			}
		}
		intent.Profiles = append(intent.Profiles, platform.ProfileIntent{
			ID:          p.ID,
			Enabled:     p.Enabled == "1",
			Policy:      normalizePolicy(p.Policy),
			BlockRouter: p.BlockRouter == "1",
			Devices:     devices(p.Devices),
			Override:    override,
			Windows:     profileWindows(eff.Schedules, p.ID),
		})
		for _, mac := range p.Devices {
			low := strings.ToLower(mac)
			if _, ok := assigned[low]; !ok {
				assigned[low] = mac
			}
		}
	}

	macs := make([]string, 0, len(assigned))
	for _, mac := range assigned {
		macs = append(macs, mac)
	}
	sort.Slice(macs, func(i, j int) bool { return strings.ToLower(macs[i]) < strings.ToLower(macs[j]) })
	intent.AssignedMACs = macs
	intent.Forward, intent.Router = networkRules(intent)

	if pactActive {
		cap, err := captainLockdown(c, eff, overrides, captainID, now)
		if err != nil {
			return platform.NetworkIntent{}, err
		}
		intent.Captain = cap
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return platform.NetworkIntent{}, err
	}
	intent.ID = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return intent, nil
}

// Compile precedence once in the core, including the historical router-access
// scope (manual block affects forwarding; block_router follows the profile).
func networkRules(i platform.NetworkIntent) (forward, router []platform.NetworkRule) {
	forward, router = []platform.NetworkRule{}, []platform.NetworkRule{}
	if !i.Enabled {
		return
	}
	for _, p := range i.Profiles {
		if !p.Enabled || p.Override == "none" {
			continue
		}
		action := "drop"
		if p.Override == "allow" {
			action = "accept"
		}
		r := platform.NetworkRule{Profile: p.ID, Devices: p.Devices, Action: action, Reason: "override:" + p.Override}
		forward = append(forward, r)
		if p.Override == "allow" {
			router = append(router, r)
		}
	}
	for _, policy := range []string{"block", "scheduled"} {
		for _, p := range i.Profiles {
			if !p.Enabled || p.Policy != policy {
				continue
			}
			if policy == "scheduled" && len(p.Windows) > 0 {
				r := platform.NetworkRule{Profile: p.ID, Devices: p.Devices, Action: "accept", Windows: p.Windows, Reason: "sched-allow"}
				forward = append(forward, r)
				if p.BlockRouter {
					router = append(router, r)
				}
			}
			r := platform.NetworkRule{Profile: p.ID, Devices: p.Devices, Action: "drop", Reason: policy}
			forward = append(forward, r)
			if p.BlockRouter {
				r.Recovery = true
				router = append(router, r)
			}
		}
	}
	return
}

func normalizeOverride(o platform.NetworkOverride) platform.NetworkOverride {
	if o.Mode == "" {
		o.Mode = "none"
	}
	return o
}

// normalizePolicy matches the legacy renderer: only "block" and "scheduled"
// produce rules; anything else (including an unknown value) is plain allow.
func normalizePolicy(policy string) string {
	if policy == "block" || policy == "scheduled" {
		return policy
	}
	return "allow"
}

func devices(in []string) []string {
	out := []string{}
	out = append(out, in...)
	return out
}

// profileWindows returns one profile's allow windows in renderer-ready form.
// Windows with no valid day or malformed boundaries are dropped: they could
// never produce a native rule.
func profileWindows(schedules []rules.ConfigurationWindow, profileID string) []platform.Window {
	out := []platform.Window{}
	for _, w := range schedules {
		if w.Profile != profileID {
			continue
		}
		days := []string{}
		for _, d := range windowDays(w.Days) {
			d = strings.ToLower(d)
			if knownDay(d) {
				days = append(days, d)
			}
		}
		if len(days) == 0 {
			continue
		}
		if _, ok := minute(w.Start); !ok {
			continue
		}
		if _, ok := minute(w.Stop); !ok {
			continue
		}
		out = append(out, platform.Window{Days: days, Start: w.Start, Stop: w.Stop})
	}
	return out
}

// captainLockdown decides whether the router must lock down TCP access to
// itself outside its allow windows: the bound captain's profile is enabled
// and can be blocked by its policy or manual override. Inside the
// lockdown the renderer accepts the captain's allow windows (any source) plus
// the ongoing amendment window; the until-pact drop is bounded by Until.
func captainLockdown(c rules.Configuration, eff rules.ConfigurationData, overrides map[string]platform.NetworkOverride, captainID string, now time.Time) (*platform.CaptainIntent, error) {
	cap := profile(eff, captainID)
	if cap == nil {
		return nil, nil
	}
	o, err := EffectiveNetworkOverride(normalizeOverride(overrides[cap.ID]), now)
	if err != nil {
		return nil, err
	}
	if cap.Enabled != "1" || (normalizePolicy(cap.Policy) == "allow" && o.Mode != "block") {
		return nil, nil
	}

	until := int64(0)
	if c.Pact.Type == "until" {
		until, err = strconv.ParseInt(c.Pact.Until, 10, 64)
		if err != nil || until < 0 {
			return nil, fmt.Errorf("invalid pact deadline %q", c.Pact.Until)
		}
	}

	out := &platform.CaptainIntent{
		ID:          cap.ID,
		Devices:     devices(cap.Devices),
		Windows:     []platform.Window{},
		PactType:    c.Pact.Type,
		Until:       until,
		WindowDay:   c.Pact.WindowDay,
		WindowStart: c.Pact.WindowStart,
		WindowStop:  c.Pact.WindowStop,
	}
	// The legacy renderer emitted the captain's schedule accepts inside the
	// lockdown only for a scheduled profile without a manual block.
	if normalizePolicy(cap.Policy) == "scheduled" && o.Mode != "block" {
		out.Windows = profileWindows(eff.Schedules, cap.ID)
	}
	return out, nil
}
