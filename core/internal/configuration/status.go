package configuration

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// EffectiveNetworkOverride applies expiry at read time without changing RAM or
// recompiling the firewall. Native timeout elements enforce their own deadlines.
func EffectiveNetworkOverride(s platform.NetworkOverride, now time.Time) (platform.NetworkOverride, error) {
	if !validOverride(s) {
		return s, errors.New("invalid network state")
	}
	if s.Mode == "none" || (s.Expires > 0 && s.Expires <= now.Unix()) {
		return platform.NetworkOverride{Mode: "none"}, nil
	}
	return s, nil
}

// internetWindowActive differs from app downtime: equal endpoints allow the
// whole day, and 24:00 is a valid exclusive boundary. Match the native
// schedule compiler (see the shell's _now_match_schedule).
func internetWindowActive(w rules.ConfigurationWindow, now time.Time) bool {
	s, ok := minute(w.Start)
	e, ok2 := minute(w.Stop)
	if !ok || !ok2 {
		return false
	}
	day := strings.ToLower(now.Weekday().String()[:3])
	previous := strings.ToLower(now.AddDate(0, 0, -1).Weekday().String()[:3])
	m := now.Hour()*60 + now.Minute()
	for _, d := range windowDays(w.Days) {
		if !knownDay(d) {
			continue
		}
		switch {
		case s < e:
			if d == day && m >= s && m < e {
				return true
			}
		case s > e:
			// Overnight: start..24:00 on the day, 00:00..stop the next morning.
			if (d == day && m >= s) || (d == previous && m < e) {
				return true
			}
		default:
			if d == day {
				return true
			}
		}
	}
	return false
}

// profileNetworkRow is the shared status decision: manual override first, then
// the profile policy (block, scheduled windows or allow). A globally disabled
// router is always allowed. The bound captain additionally carries the pact
// details the administrator UI renders.
func profileNetworkRow(c rules.Configuration, effective rules.ConfigurationData, p rules.ConfigurationProfile, raw platform.NetworkOverride, now time.Time, lastSync int64) (map[string]any, error) {
	s, err := PactNetworkOverride(c, p.ID, raw, now)
	if err != nil {
		return nil, err
	}
	s, err = EffectiveNetworkOverride(s, now)
	if err != nil {
		return nil, err
	}
	allowed := or(effective.Settings.Enabled, "1") != "1"
	if !allowed {
		switch s.Mode {
		case "allow":
			allowed = true
		case "block":
			allowed = false
		default:
			switch or(p.Policy, "allow") {
			case "block":
				allowed = false
			case "scheduled":
				for _, w := range effective.Schedules {
					if w.Profile == p.ID && internetWindowActive(w, now) {
						allowed = true
						break
					}
				}
			default:
				allowed = true
			}
		}
	}
	status := "blocked"
	if allowed {
		status = "allowed"
	}
	bound := Active(c.Pact, now) && c.Pact.Captain == p.ID
	row := map[string]any{"name": or(p.Name, p.ID), "policy": or(p.Policy, "allow"),
		"status": status, "override": s.Mode, "override_expires": s.Expires,
		"block_router": p.BlockRouter == "1", "captain": bound,
		"devices": append([]string{}, p.Devices...)}
	if bound {
		row["pact_type"], row["pact_until"] = c.Pact.Type, integer(c.Pact.Until)
		row["pact_window"] = fmt.Sprintf("%s %s-%s", c.Pact.WindowDay, c.Pact.WindowStart, c.Pact.WindowStop)
		row["pact_in_window"], row["release_at"] = InWindow(c.Pact, now), integer(c.Pact.ReleaseAt)
		// A fetch, never confirmed phone enforcement.
		row["mast_last_sync"] = lastSync
	}
	return row, nil
}

// networkReport runs under the authority mutex; the backend supplies one locked
// runtime snapshot. No platform policy computation or socket callback is needed.
func (a *Authority) networkReport(d document, single *string) (map[string]any, error) {
	var result map[string]any
	err := a.backend.NetworkSnapshot(func(facts platform.Context, state map[string]platform.NetworkOverride) error {
		if facts.NetworkFormat != networkFormat {
			return errors.New("incompatible network adapter: upgrade core and backend together")
		}
		now := a.at(facts)
		effective := Effective(d.Configuration, now)
		profiles := map[string]any{}
		for _, p := range effective.Profiles {
			if p.Enabled != "1" || (single != nil && p.ID != *single) {
				continue
			}
			raw := state[p.ID]
			if raw.Mode == "" {
				raw = platform.NetworkOverride{Mode: "none"}
			}
			row, err := profileNetworkRow(d.Configuration, effective, p, raw, now, facts.MastLastSync)
			if err != nil {
				return err
			}
			if single != nil {
				result = row
				return nil
			}
			row["apps"] = p.Apps == "1"
			row["bonus_today"] = 0
			today := now.Format(time.DateOnly)
			for _, e := range d.Bonus.Entries {
				if e.Profile == p.ID && e.Target == "total" && e.Day == today {
					row["bonus_today"] = e.Minutes
				}
			}
			windows := []map[string]string{}
			for _, w := range effective.Schedules {
				if w.Profile == p.ID {
					windows = append(windows, map[string]string{"days": w.Days, "start": w.Start, "stop": w.Stop})
				}
			}
			row["schedules"] = windows
			profiles[p.ID] = row
		}
		if single != nil {
			return errors.New("profile not found or disabled")
		}
		assigned := []string{}
		for _, p := range effective.Profiles {
			assigned = append(assigned, p.Devices...)
		}
		configuration := map[string]any{"enabled": or(effective.Settings.Enabled, "1") == "1",
			"default_policy": or(effective.Settings.DefaultPolicy, "allow"), "captain": Captain(d.Configuration, now),
			"delay_hours": or(effective.Settings.DelayHours, "24"), "assigned_devices": assigned}
		if Active(d.Configuration.Pact, now) {
			configuration["pact_delay_seconds"] = integer(d.Configuration.Pact.DelaySeconds)
		}
		result = map[string]any{"profiles": profiles, "configuration": configuration}
		return nil
	})
	return result, err
}
