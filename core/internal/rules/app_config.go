package rules

import (
	"fmt"
	"sort"
	"strings"
)

// AppConfig is raw configuration, independent of UCI or any router platform.
// String values preserve legacy import semantics, including unset vs zero limits.
// Version 1 is an adapter input, not yet the authoritative configuration store.
type AppConfig struct {
	Version       int          `json:"version"`
	Enabled       string       `json:"enabled"`
	SuspendNew    string       `json:"suspend_new"`
	ScreenMinutes string       `json:"screen_minutes"`
	ScreenWeekday string       `json:"screen_weekday"`
	ScreenWeekend string       `json:"screen_weekend"`
	AlwaysAllowed []string     `json:"always_allowed"`
	Apps          []AppSetting `json:"apps"`
	Downtime      []AppWindow  `json:"downtime"`
	Bonus         string       `json:"bonus"`
}

type AppSetting struct {
	Package string `json:"package"`
	Minutes string `json:"minutes"`
	Blocked string `json:"blocked"`
}

func digits(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

// Canonical preserves legacy normalization: blocked wins, duplicate package limits
// merge to the minimum, invalid app minutes become zero, oversized values clamp,
// invalid packages/downtime/total limits are ignored. New editing APIs must validate
// user input before saving, rather than relying on these compatibility repairs.
func (c AppConfig) Canonical() (AppInput, error) {
	if c.Version != 1 {
		return AppInput{}, fmt.Errorf("unsupported app configuration version %d", c.Version)
	}
	if c.Enabled != "1" {
		return AppInput{}, nil
	}
	suspend := "1"
	if c.SuspendNew == "0" {
		suspend = "0"
	}
	lines := map[string]bool{"new|" + suspend + "||": true}
	apps := map[string]AppSetting{}
	for _, a := range c.Apps {
		if !validAppPackage(a.Package) {
			continue
		}
		if a.Minutes != "" {
			switch {
			case !digits(a.Minutes):
				a.Minutes = "0"
			case len(a.Minutes) > 4:
				a.Minutes = "1440"
			default:
				if _, err := appMinutes(a.Minutes); err != nil {
					a.Minutes = "1440"
				}
			}
		}
		if a.Blocked != "1" {
			a.Blocked = "0"
		}
		old, exists := apps[a.Package]
		if exists {
			if old.Blocked == "1" {
				a.Blocked = "1"
			}
			oldMin, _ := appMinutes(old.Minutes)
			newMin, _ := appMinutes(a.Minutes)
			if old.Minutes != "" && (a.Minutes == "" || oldMin <= newMin) {
				a.Minutes = old.Minutes
			}
		}
		apps[a.Package] = a
	}
	for _, a := range apps {
		lines["app|"+a.Package+"|"+a.Minutes+"|"+a.Blocked] = true
	}
	for _, w := range c.Downtime {
		// Match the former shell's IFS, not Unicode whitespace.
		days := strings.FieldsFunc(w.Days, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
		valid := len(days) > 0
		for _, d := range days {
			valid = valid && appDay(d)
		}
		_, startOK := minutes(w.Start)
		_, stopOK := minutes(w.Stop)
		if valid && startOK && stopOK {
			lines["down|"+strings.Join(days, " ")+"|"+w.Start+"|"+w.Stop] = true
		}
	}
	for _, day := range days {
		v := c.ScreenWeekday
		if day == "sat" || day == "sun" {
			v = c.ScreenWeekend
		}
		if v == "" {
			v = c.ScreenMinutes
		}
		if _, err := appMinutes(v); err == nil {
			lines["screen|"+day+"|"+v+"|"] = true
		}
	}
	for _, pkg := range c.AlwaysAllowed {
		if validAppPackage(pkg) {
			lines["always|"+pkg+"||"] = true
		}
	}
	out := make([]string, 0, len(lines))
	for line := range lines {
		out = append(out, line)
	}
	sort.Strings(out)
	return AppInput{Rules: strings.Join(out, "\n"), Bonus: c.Bonus}, nil
}

func (c AppConfig) Compile() (*AppPolicy, []string, error) {
	in, err := c.Canonical()
	if err != nil {
		return nil, nil, err
	}
	return CompileApps(in)
}
