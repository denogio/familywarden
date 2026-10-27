package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

const bonusFormat = "bonus-state-v1"

// Bonus is runtime state, not editable configuration: CAS rollback must not
// revoke time already given. A non-nil state is also the one-time import marker.
type bonusState struct {
	Entries []bonusEntry `json:"entries"`
}

type bonusEntry struct {
	Profile string `json:"profile"`
	Day     string `json:"day"`
	Target  string `json:"target"`
	Minutes int    `json:"minutes"`
}

func (b *bonusState) validate() error {
	seen := map[string]bool{}
	for _, e := range b.Entries {
		day, err := time.Parse(time.DateOnly, e.Day)
		key := e.Profile + "|" + e.Day + "|" + e.Target
		if !identifier.MatchString(e.Profile) || err != nil || day.Format(time.DateOnly) != e.Day ||
			(e.Target != "total" && !rules.ValidAppPackage(e.Target)) || e.Minutes < 1 || e.Minutes > 1440 || seen[key] {
			return errors.New("invalid bonus state")
		}
		seen[key] = true
	}
	return nil
}

func importBonus(facts platform.Context) (*bonusState, error) {
	if facts.BonusFormat != bonusFormat {
		return nil, errors.New("incompatible bonus adapter: upgrade core and backend together")
	}
	b := &bonusState{}
	for _, line := range strings.Split(strings.TrimSuffix(facts.LegacyBonus, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != 4 || !decimal(f[3]) {
			return nil, errors.New("invalid legacy bonus state")
		}
		n, err := strconv.Atoi(f[3])
		if err != nil {
			return nil, errors.New("invalid legacy bonus minutes")
		}
		b.Entries = append(b.Entries, bonusEntry{f[0], f[1], f[2], n})
	}
	return b, b.validate()
}

func (b *bonusState) lines(day string) map[string]string {
	rows := map[string][]string{}
	for _, e := range b.Entries {
		if e.Day == day {
			rows[e.Profile] = append(rows[e.Profile], fmt.Sprintf("bonus|%s|%d|%s", e.Target, e.Minutes, e.Day))
		}
	}
	out := map[string]string{}
	for id, lines := range rows {
		sort.Strings(lines) // Preserve the existing signed app revision's canonical bytes.
		out[id] = strings.Join(lines, "\n")
	}
	return out
}

// Called with the authority mutex held; shell compatibility writers share the
// backend transaction lock. No firewall apply is needed for phone-only bonus.
func (a *Authority) addBonus(d document, raw []byte) (map[string]any, error) {
	if d.Pending != nil {
		return nil, errors.New("confirmation pending")
	}
	var req struct {
		Profile string          `json:"profile"`
		Minutes json.RawMessage `json:"minutes"`
		Package string          `json:"package"`
	}
	if err := decode(raw, &req); err != nil {
		return nil, err
	}
	minutes := string(req.Minutes)
	if len(minutes) > 0 && minutes[0] == '"' {
		if err := json.Unmarshal(req.Minutes, &minutes); err != nil {
			return nil, err
		}
	}
	n, err := strconv.Atoi(minutes)
	if !decimal(minutes) || err != nil || n < 1 || n > 240 {
		return nil, errors.New("minutes must be 1..240")
	}
	target := req.Package
	if target == "" {
		target = "total"
	}
	if target != "total" && !rules.ValidAppPackage(target) {
		return nil, errors.New("invalid package")
	}
	total := n
	err = a.backend.ConfigurationTransaction(func(_ rules.Configuration, facts platform.Context, _ map[string]platform.NetworkOverride, _ func(rules.Configuration, platform.NetworkIntent) error) error {
		if facts.BonusFormat != bonusFormat {
			return errors.New("incompatible bonus adapter")
		}
		now := a.at(facts)
		p := profile(Effective(d.Configuration, now), req.Profile)
		if p == nil {
			return errors.New("profile not found")
		}
		if p.Apps != "1" {
			return errors.New("app control is off for this profile")
		}
		if Active(d.Configuration.Pact, now) && d.Configuration.Pact.Captain == req.Profile {
			return errors.New("profile is bound by Ulysses pact")
		}
		day := now.Format(time.DateOnly)
		kept := []bonusEntry{}
		for _, e := range d.Bonus.Entries {
			if e.Profile == req.Profile && e.Day == day && e.Target == target {
				total += e.Minutes
			} else if e.Day >= day {
				kept = append(kept, e)
			}
		}
		total = min(total, 1440)
		d.Bonus.Entries = append(kept, bonusEntry{req.Profile, day, target, total})
		if err := a.save(d); err != nil {
			// A write/read-back error can leave uncertain durable state. Never
			// pretend it succeeded or silently read the old shell-owned file.
			a.blocked = err
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "ok", "profile": req.Profile, "target": target, "minutes_today": total}, nil
}
