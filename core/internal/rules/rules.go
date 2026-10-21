// Package rules decides whether an app may run now. Pure logic, no I/O: the same
// precedence as plans/app-control-protocol.md ("Precedence"), shared by the router
// (for reporting) and mirrored by the phone (for enforcement).
package rules

import (
	"slices"
	"time"
)

// Window is a weekly time range in router-local time. Days name the start day;
// an overnight window (start > stop) continues into the next day, as in the
// router's internet schedules.
type Window struct {
	Days  []string `json:"days"`
	Start string   `json:"start"`
	Stop  string   `json:"stop"`
}

type Rule struct {
	ID           string   `json:"id"`
	Packages     []string `json:"packages,omitempty"`
	Category     string   `json:"category,omitempty"`
	DailyMinutes int      `json:"daily_minutes"`
}

type Policy struct {
	Rules      []Rule   `json:"rules"`
	Downtime   []Window `json:"downtime"`
	Blocked    []string `json:"blocked"`
	Essential  []string `json:"essential"`
	Approved   []string `json:"approved"`
	SuspendNew bool     `json:"suspend_new"`
}

// State is what has been used: shared budget left per rule (seconds, D4) and
// seconds left on valid grants per package.
type State struct {
	Remaining map[string]int
	Granted   map[string]int
}

type Reason string

const (
	Essential   Reason = "essential"
	Blocked     Reason = "blocked"
	NotApproved Reason = "not_approved"
	Grant       Reason = "grant"
	Downtime    Reason = "downtime"
	Budget      Reason = "budget"
	Allowed     Reason = "allowed"
)

type Decision struct {
	Allowed bool
	Reason  Reason
	Rule    string // the exhausted rule, for Budget
}

// Decide applies the precedence: essential, blocked, not approved, grant,
// downtime, exhausted budget, otherwise allowed. local must be router-local time.
func Decide(p Policy, s State, pkg, category string, local time.Time) Decision {
	switch {
	case slices.Contains(p.Essential, pkg):
		return Decision{true, Essential, ""}
	case slices.Contains(p.Blocked, pkg):
		return Decision{false, Blocked, ""}
	case p.SuspendNew && !slices.Contains(p.Approved, pkg):
		return Decision{false, NotApproved, ""}
	case s.Granted[pkg] > 0:
		return Decision{true, Grant, ""}
	}
	for _, w := range p.Downtime {
		if w.Active(local) {
			return Decision{false, Downtime, ""}
		}
	}
	for _, r := range p.Rules {
		if r.Matches(pkg, category) && s.Remaining[r.ID] <= 0 {
			return Decision{false, Budget, r.ID}
		}
	}
	return Decision{true, Allowed, ""}
}

func (r Rule) Matches(pkg, category string) bool {
	return slices.Contains(r.Packages, pkg) || (r.Category != "" && r.Category == category)
}

var days = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

func minutes(hhmm string) (int, bool) {
	var h, m int
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return 0, false
	}
	for i, c := range hhmm {
		if i == 2 {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	h, m = int(hhmm[0]-'0')*10+int(hhmm[1]-'0'), int(hhmm[3]-'0')*10+int(hhmm[4]-'0')
	return h*60 + m, h < 24 && m < 60
}

// Active reports whether local falls inside the window. Malformed or empty
// windows (start == stop) are never active, so a typo cannot lock everything.
func (w Window) Active(local time.Time) bool {
	start, ok1 := minutes(w.Start)
	stop, ok2 := minutes(w.Stop)
	if !ok1 || !ok2 || start == stop {
		return false
	}
	now := local.Hour()*60 + local.Minute()
	today := days[local.Weekday()]
	if start < stop {
		return slices.Contains(w.Days, today) && now >= start && now < stop
	}
	yesterday := days[(local.Weekday()+6)%7]
	return (slices.Contains(w.Days, today) && now >= start) || (slices.Contains(w.Days, yesterday) && now < stop)
}
