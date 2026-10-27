package rules

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AppInput is the interim adapter seam for effective app rules. Rules are sorted,
// unique canonical lines; Bonus contains today's sorted bonus lines. The OpenWrt
// adapter still resolves UCI/pact snapshots; the portable core owns the wire policy,
// revision and feature requirements. Configuration ownership is migration step 3b.
type AppInput struct {
	Rules string `json:"rules"`
	Bonus string `json:"bonus"`
}

type AppLimit struct {
	Package string `json:"package"`
	Minutes int    `json:"minutes"`
}

type AppWindow struct {
	Days  string `json:"days"`
	Start string `json:"start"`
	Stop  string `json:"stop"`
}

type AppBonus struct {
	Day   string     `json:"day"`
	Total int        `json:"total"`
	Apps  []AppLimit `json:"apps"`
}

type AppPolicy struct {
	Rev           string         `json:"rev"`
	SuspendNew    bool           `json:"suspend_new"`
	Limits        []AppLimit     `json:"limits"`
	Blocked       []string       `json:"blocked"`
	Approved      []string       `json:"approved"`
	Downtime      []AppWindow    `json:"downtime"`
	Screen        map[string]int `json:"screen,omitempty"`
	AlwaysAllowed []string       `json:"always_allowed"`
	Bonus         *AppBonus      `json:"bonus,omitempty"`
}

// Keep the existing canonical package grammar during migration; changing accepted
// legacy configuration is separate from moving its compiler.
var appPackage = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.]*\.[A-Za-z0-9_]+$`)

// ValidAppPackage preserves the legacy package grammar for rules and bonus.
func ValidAppPackage(s string) bool {
	return len(s) <= 255 && appPackage.MatchString(s) && !strings.Contains(s, "..")
}

func appMinutes(s string) (int, error) {
	if s == "" || len(s) > 4 || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("invalid app minutes")
	}
	v, err := strconv.Atoi(s)
	if err != nil || v > 1440 {
		return 0, fmt.Errorf("invalid app minutes")
	}
	return v, nil
}

func appDay(s string) bool {
	for _, d := range days {
		if s == d {
			return true
		}
	}
	return false
}

// CompileApps preserves the deployed canonical revision byte-for-byte. Invalid
// input fails the entire policy, never silently dropping a restriction.
func CompileApps(in AppInput) (*AppPolicy, []string, error) {
	if in.Rules == "" {
		if in.Bonus != "" {
			return nil, nil, fmt.Errorf("bonus without app control")
		}
		return nil, nil, nil
	}
	p := &AppPolicy{Limits: []AppLimit{}, Blocked: []string{}, Approved: []string{},
		Downtime: []AppWindow{}, AlwaysAllowed: []string{}}
	fail := func() (*AppPolicy, []string, error) { return nil, nil, fmt.Errorf("invalid canonical app policy") }
	screenTime, newSeen, previous := false, false, ""
	packages := map[string]bool{}
	for _, line := range strings.Split(in.Rules, "\n") {
		f := strings.Split(line, "|")
		if len(f) != 4 || (previous != "" && line <= previous) {
			return fail()
		}
		previous = line
		a, b, c := f[1], f[2], f[3]
		switch f[0] {
		case "new":
			if newSeen || (a != "0" && a != "1") || b != "" || c != "" {
				return fail()
			}
			p.SuspendNew, newSeen = a == "1", true
		case "app":
			if !ValidAppPackage(a) || packages[a] || (c != "0" && c != "1") {
				return fail()
			}
			packages[a] = true
			var n int
			if b != "" {
				var err error
				n, err = appMinutes(b)
				if err != nil {
					return fail()
				}
			}
			if c == "1" {
				p.Blocked = append(p.Blocked, a)
			} else {
				p.Approved = append(p.Approved, a)
				if b != "" {
					p.Limits = append(p.Limits, AppLimit{a, n})
				}
			}
		case "down":
			if len(strings.Fields(a)) == 0 {
				return fail()
			}
			for _, d := range strings.Fields(a) {
				if !appDay(d) {
					return fail()
				}
			}
			_, startOK := minutes(b)
			_, stopOK := minutes(c)
			if !startOK || !stopOK {
				return fail()
			}
			p.Downtime = append(p.Downtime, AppWindow{a, b, c})
			screenTime = true
		case "screen":
			n, err := appMinutes(b)
			if err != nil || !appDay(a) || c != "" {
				return fail()
			}
			if p.Screen == nil {
				p.Screen = map[string]int{}
			}
			if _, duplicate := p.Screen[a]; duplicate {
				return fail()
			}
			p.Screen[a], screenTime = n, true
		case "always":
			if !ValidAppPackage(a) || b != "" || c != "" {
				return fail()
			}
			p.AlwaysAllowed = append(p.AlwaysAllowed, a)
			screenTime = true
		default:
			return fail()
		}
	}
	if !newSeen {
		return fail()
	}
	if in.Bonus != "" {
		p.Bonus = &AppBonus{Apps: []AppLimit{}}
		seen := map[string]bool{}
		previous = ""
		for _, line := range strings.Split(in.Bonus, "\n") {
			f := strings.Split(line, "|")
			if len(f) != 4 || f[0] != "bonus" || (previous != "" && line <= previous) {
				return fail()
			}
			previous = line
			target, day := f[1], f[3]
			n, err := appMinutes(f[2])
			t, dateErr := time.Parse(time.DateOnly, day)
			if err != nil || dateErr != nil || t.Format(time.DateOnly) != day || seen[target] ||
				(p.Bonus.Day != "" && p.Bonus.Day != day) {
				return fail()
			}
			seen[target], p.Bonus.Day = true, day
			if target == "total" {
				p.Bonus.Total = n
			} else if ValidAppPackage(target) {
				p.Bonus.Apps = append(p.Bonus.Apps, AppLimit{target, n})
			} else {
				return fail()
			}
		}
		screenTime = true
	}
	sum := sha256.Sum256([]byte(in.Rules + "\n" + in.Bonus + "\n"))
	p.Rev = fmt.Sprintf("%x", sum[:8])
	requires := []string{"apps-v1"}
	if screenTime {
		requires = append(requires, "screen-time-v1")
	}
	return p, requires, nil
}
