package identity

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

// New-app approval (P10, Family Link "ask to use"). A child's phone reports apps that
// are paused because they are not approved yet; a parent in scope or the administrator
// allows or denies them. Allowed apps are added to the profile's signed `apps.approved`
// list by the core (the shell's app rules are untouched until migration step 3).
// Allowing is loosening, so it is refused for a pact-bound captain; removing an
// approval only tightens and is always possible.

const (
	newAppsFile     = "newapps.json"
	newAppsVersion  = 1
	maxNewPending   = 32
	maxNewApproved  = 256
	maxNewDismissed = 256
	pendingVisible  = 7 * 24 * time.Hour
)

type pendingApp struct {
	Label  string `json:"label,omitempty"`
	Device string `json:"device"`
	Seen   int64  `json:"seen"`
}

type approvedApp struct {
	Label string `json:"label,omitempty"`
	By    string `json:"by"`
	At    int64  `json:"at"`
}

type profileApps struct {
	Pending   map[string]pendingApp  `json:"pending,omitempty"`
	Approved  map[string]approvedApp `json:"approved,omitempty"`
	Dismissed map[string]int64       `json:"dismissed,omitempty"`
}

type newAppsDoc struct {
	Version  int                     `json:"version"`
	Profiles map[string]*profileApps `json:"profiles"`
}

// loadNewApps reads (and caches) the document; lock held.
func (s *Service) loadNewApps() (map[string]*profileApps, error) {
	if s.newApps != nil {
		return s.newApps, nil
	}
	var doc newAppsDoc
	found, err := s.st.ReadDocument(newAppsFile, &doc)
	if err != nil {
		return nil, err
	}
	if found {
		if doc.Version != newAppsVersion {
			return nil, fmt.Errorf("newapps: unsupported schema version %d", doc.Version)
		}
		if err := validateNewApps(doc.Profiles); err != nil {
			return nil, err
		}
	}
	if doc.Profiles == nil {
		doc.Profiles = map[string]*profileApps{}
	}
	s.newApps = doc.Profiles
	return s.newApps, nil
}

func validateNewApps(p map[string]*profileApps) error {
	for id, a := range p {
		if !validProfile.MatchString(id) || a == nil || len(a.Pending) > maxNewPending ||
			len(a.Approved) > maxNewApproved || len(a.Dismissed) > maxNewDismissed {
			return fmt.Errorf("newapps: invalid profile %q", id)
		}
		for pkg, e := range a.Pending {
			if _, ok := cleanText(e.Label, 64); !validTarget(pkg) || pkg == targetTotal || !ok || !validDevice.MatchString(e.Device) {
				return fmt.Errorf("newapps: invalid pending %q", pkg)
			}
		}
		for pkg, e := range a.Approved {
			if _, ok := cleanText(e.Label, 64); !validTarget(pkg) || pkg == targetTotal || !ok || e.By == "" {
				return fmt.Errorf("newapps: invalid approval %q", pkg)
			}
		}
		for pkg := range a.Dismissed {
			if !validTarget(pkg) || pkg == targetTotal {
				return fmt.Errorf("newapps: invalid dismissal %q", pkg)
			}
		}
	}
	return nil
}

// saveNewApps persists a copy; RAM changes only after the write succeeded.
func (s *Service) saveNewApps(next map[string]*profileApps) error {
	if err := validateNewApps(next); err != nil {
		return err
	}
	if err := s.st.WriteDocument(newAppsFile, newAppsDoc{Version: newAppsVersion, Profiles: next}); err != nil {
		return err
	}
	s.newApps = next
	return nil
}

func cloneNewApps(src map[string]*profileApps) map[string]*profileApps {
	b, _ := json.Marshal(src)
	out := map[string]*profileApps{}
	_ = json.Unmarshal(b, &out)
	return out
}

func (p *profileApps) ensure() {
	if p.Pending == nil {
		p.Pending = map[string]pendingApp{}
	}
	if p.Approved == nil {
		p.Approved = map[string]approvedApp{}
	}
	if p.Dismissed == nil {
		p.Dismissed = map[string]int64{}
	}
}

// reportNewApps records what one phone reports as paused-for-approval (lock held).
// Apps this phone no longer reports (installed elsewhere, approved in UCI,
// uninstalled) leave the list. Writes only on change.
func (s *Service) reportNewApps(d Device, reported map[string]string) error {
	all, err := s.loadNewApps()
	if err != nil {
		return err
	}
	now := s.Now().Unix()
	next := cloneNewApps(all)
	p := next[d.Profile]
	if p == nil {
		p = &profileApps{}
		next[d.Profile] = p
	}
	p.ensure()
	changed := false
	for pkg, e := range p.Pending {
		if _, still := reported[pkg]; e.Device == d.ID && !still {
			delete(p.Pending, pkg)
			changed = true
		}
	}
	for pkg, label := range reported {
		if _, ok := p.Approved[pkg]; ok {
			continue
		}
		if _, ok := p.Dismissed[pkg]; ok {
			continue
		}
		old, had := p.Pending[pkg]
		// "seen" is refreshed at most hourly: a report every minute must not wear flash.
		if had && old.Label == label && old.Device == d.ID && now-old.Seen < 3600 {
			continue
		}
		if !had && len(p.Pending) >= maxNewPending {
			continue
		}
		p.Pending[pkg] = pendingApp{Label: label, Device: d.ID, Seen: now}
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveNewApps(next)
}

// approvedFor is the core's extra approved packages for a profile (lock held).
func (s *Service) approvedFor(profile string) []string {
	p := s.newApps[profile]
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Approved))
	for pkg := range p.Approved {
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

func (s *Service) visiblePending(profile string, now int64) []object {
	p := s.newApps[profile]
	out := []object{}
	if p == nil {
		return out
	}
	pkgs := make([]string, 0, len(p.Pending))
	for pkg, e := range p.Pending {
		if now-e.Seen < int64(pendingVisible/time.Second) {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		e := p.Pending[pkg]
		out = append(out, object{{"package", pkg}, {"label", e.Label}, {"seen", e.Seen}})
	}
	return out
}

// parentNewApps lists apps waiting for approval in a parent phone's scope (lock held).
func (s *Service) parentNewApps(d Device, ctx platform.Context, now int64) []object {
	out := []object{}
	for _, p := range effectiveApproves(d, ctx) {
		if ctx.Profiles[p].AppsRev == "" {
			continue
		}
		for _, a := range s.visiblePending(p, now) {
			out = append(out, append(object{{"profile_id", p}, {"profile_name", ctx.Profiles[p].Name}}, a...))
		}
	}
	return out
}

// decideApp applies "approve", "deny" or "revoke" (lock held). by is "router" or "phone:<id>".
func (s *Service) decideApp(by, profile, pkg, action string, ctx platform.Context) object {
	out := object{{"profile", profile}, {"package", pkg}}
	if !validProfile.MatchString(profile) || !validTarget(pkg) || pkg == targetTotal {
		return append(out, refuse("malformed")...)
	}
	if prof, ok := ctx.Profiles[profile]; !ok || prof.AppsRev == "" {
		return append(out, refuse("app_control_off")...)
	}
	if action == "approve" && profile == ctx.PactCaptain {
		return append(out, refuse("pact_forbids")...)
	}
	all, err := s.loadNewApps()
	if err != nil {
		return append(out, refuse("unavailable")...)
	}
	next := cloneNewApps(all)
	p := next[profile]
	if p == nil {
		p = &profileApps{}
		next[profile] = p
	}
	p.ensure()
	now := s.Now().Unix()
	switch action {
	case "approve":
		if _, ok := p.Approved[pkg]; ok {
			return append(out, field{"state", "approved"})
		}
		pend, ok := p.Pending[pkg]
		if !ok {
			return append(out, refuse("unknown_app")...)
		}
		if len(p.Approved) >= maxNewApproved {
			return append(out, refuse("unavailable")...)
		}
		p.Approved[pkg] = approvedApp{Label: pend.Label, By: by, At: now}
		delete(p.Pending, pkg)
		delete(p.Dismissed, pkg)
	case "deny":
		if _, ok := p.Pending[pkg]; !ok {
			if _, done := p.Dismissed[pkg]; done {
				return append(out, field{"state", "denied"})
			}
			return append(out, refuse("unknown_app")...)
		}
		delete(p.Pending, pkg)
		if len(p.Dismissed) >= maxNewDismissed {
			oldest := ""
			for k, at := range p.Dismissed {
				if oldest == "" || at < p.Dismissed[oldest] {
					oldest = k
				}
			}
			delete(p.Dismissed, oldest)
		}
		p.Dismissed[pkg] = now
	case "revoke":
		if _, ok := p.Approved[pkg]; !ok {
			return append(out, field{"state", "revoked"})
		}
		delete(p.Approved, pkg)
	default:
		return append(out, refuse("malformed")...)
	}
	if err := s.saveNewApps(next); err != nil {
		log.Printf("saving app decision: %v", err)
		return append(out, refuse("unavailable")...)
	}
	state := map[string]string{"approve": "approved", "deny": "denied", "revoke": "revoked"}[action]
	log.Printf("app %s for %s %s by %s", pkg, profile, state, by)
	return append(out, field{"state", state})
}

type appDecisionMsg struct {
	Type    string `json:"type"`
	V       int    `json:"v"`
	Device  string `json:"device"`
	Profile string `json:"profile"`
	Package string `json:"package"`
	Action  string `json:"action"`
	Created int64  `json:"created"`
}

// appDecision handles a parent phone's signed op=app (lock held).
func (s *Service) appDecision(d Device, raw []byte, ctx platform.Context) object {
	var m appDecisionMsg
	if !decodeStrict(raw, &m, "type v device profile package action created", "") ||
		m.Type != "app_decision" || m.V != 1 || m.Device != d.ID {
		return refuse("malformed")
	}
	created := time.Unix(m.Created, 0)
	if now := s.Now(); m.Created <= 0 || m.Created > maxEpoch || now.Sub(created) >= time.Hour || created.Sub(now) > 5*time.Minute {
		return object{{"profile", m.Profile}, {"package", m.Package}, {"error", "decision_expired"}}
	}
	inScope := false
	for _, p := range effectiveApproves(d, ctx) {
		inScope = inScope || p == m.Profile
	}
	if !inScope {
		return object{{"profile", m.Profile}, {"package", m.Package}, {"error", "not_parent"}}
	}
	return s.decideApp("phone:"+d.ID, m.Profile, m.Package, m.Action, ctx)
}

// withApprovals adds core approvals (never blocked ones) to the shell's apps object.
func withApprovals(apps json.RawMessage, approved []string) (json.RawMessage, error) {
	if len(approved) == 0 {
		return apps, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(apps, &m); err != nil {
		return nil, err
	}
	var have, blocked []string
	for key, into := range map[string]*[]string{"approved": &have, "blocked": &blocked} {
		if raw, ok := m[key]; ok {
			if err := json.Unmarshal(raw, into); err != nil {
				return nil, err
			}
		}
	}
	skip := map[string]bool{}
	for _, p := range append(append([]string{}, have...), blocked...) {
		skip[p] = true
	}
	for _, p := range approved {
		if !skip[p] && len(have) < 512 {
			have = append(have, p)
			skip[p] = true
		}
	}
	m["approved"], _ = json.Marshal(have)
	return json.Marshal(m)
}

// NewApps is the administrator's view: waiting and core-approved apps per profile.
func (s *Service) NewApps() Reply {
	ctx, err := s.ad.Context()
	if err != nil {
		return fail("platform state unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.loadNewApps(); err != nil {
		return fail("new apps unavailable")
	}
	now := s.Now().Unix()
	ids := []string{}
	for id, p := range ctx.Profiles {
		if p.AppsRev != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	profiles := []Reply{}
	for _, id := range ids {
		approved := []Reply{}
		for _, pkg := range s.approvedFor(id) {
			a := s.newApps[id].Approved[pkg]
			r := Reply{"package": pkg, "label": a.Label, "by": s.nameOf(a.By), "at": a.At}
			approved = append(approved, r)
		}
		pending := s.visiblePending(id, now)
		profiles = append(profiles, Reply{"id": id, "name": ctx.Profiles[id].Name,
			"pending": pending, "approved": approved, "frozen": id == ctx.PactCaptain})
	}
	return Reply{"profiles": profiles}
}

// AppDecisionInput is the administrator's decision from LuCI (issuer "router").
type AppDecisionInput struct {
	Profile string `json:"profile"`
	Package string `json:"package"`
	Action  string `json:"action"`
}

func DecodeAppDecision(raw []byte) (AppDecisionInput, error) {
	var a AppDecisionInput
	if !decodeStrict(raw, &a, "profile package action", "") {
		return a, fmt.Errorf("malformed decision")
	}
	return a, nil
}

func (s *Service) AppDecision(a AppDecisionInput) Reply {
	return s.mutate(func(ctx platform.Context) Reply {
		r := Reply{}
		for _, f := range s.decideApp("router", a.Profile, a.Package, a.Action, ctx) {
			r[f.k] = f.v
		}
		if _, bad := r["error"]; !bad {
			r["status"] = "ok"
		}
		return r
	})
}
