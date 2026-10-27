package configuration

import (
	"fmt"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// AppContext takes configured app rules from the authoritative document and
// core-owned bonus and runtime timezone facts. Phone replies, request
// authorization and administrator views must use this same compilation.
func AppContext(c rules.Configuration, facts platform.Context, now time.Time, bonus map[string]string) (platform.Context, error) {
	if facts.BonusFormat != bonusFormat {
		return platform.Context{}, fmt.Errorf("incompatible bonus adapter")
	}
	effective := Effective(c, now)
	captain := ""
	if Active(c.Pact, now) {
		captain = c.Pact.Captain
	}
	if captain != facts.PactCaptain {
		return platform.Context{}, fmt.Errorf("platform pact state diverged from the configuration")
	}
	out := facts
	out.PactCaptain = captain
	out.Profiles = make(map[string]platform.Profile, len(effective.Profiles))
	for _, p := range effective.Profiles {
		if _, ok := facts.Profiles[p.ID]; !ok {
			return platform.Context{}, fmt.Errorf("runtime app facts unavailable for %s", p.ID)
		}
		input := rules.AppConfig{Version: 1, Enabled: p.Apps, SuspendNew: p.SuspendNew,
			ScreenMinutes: p.ScreenMinutes, ScreenWeekday: p.ScreenWeekday, ScreenWeekend: p.ScreenWeekend,
			AlwaysAllowed: append([]string{}, p.AlwaysAllowed...), Bonus: bonus[p.ID]}
		for _, a := range effective.Apps {
			if a.Profile == p.ID {
				input.Apps = append(input.Apps, rules.AppSetting{Package: a.Package, Minutes: a.Minutes, Blocked: a.Blocked})
			}
		}
		for _, w := range effective.Downtime {
			if w.Profile == p.ID {
				input.Downtime = append(input.Downtime, rules.AppWindow{Days: w.Days, Start: w.Start, Stop: w.Stop})
			}
		}
		apps, requires, err := input.Compile()
		if err != nil {
			return platform.Context{}, fmt.Errorf("apps of %s: %w", p.ID, err)
		}
		prof := platform.Profile{Name: or(p.Name, p.ID), Requires: requires, AppInput: &input}
		if apps != nil {
			prof.AppsRev = apps.Rev
		}
		out.Profiles[p.ID] = prof
	}
	return out, nil
}
