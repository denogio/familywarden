package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
)

// installed is a bounded, scoped read of a profile's reported apps and the rule the
// core currently compiles for each (plans/android-installed-apps.md I2). Information
// only: it changes nothing, and a missing phone report is reported as missing.
// Authentication and current registry/context are locked by uploadLocked.

const installedPageBytes = 48 << 10 // the phone accepts 64 KiB replies

func (s *Service) installed(d Device, raw []byte, ctx platform.Context) object {
	var m struct {
		Type    string `json:"type"`
		V       int    `json:"v"`
		Device  string `json:"device"`
		Profile string `json:"profile"`
		After   string `json:"after"`
		Nonce   string `json:"nonce"`
		Created int64  `json:"created"`
	}
	if !decodeStrict(raw, &m, "type v device profile after nonce created", "") || json.Unmarshal(raw, &m) != nil ||
		m.Type != "installed" || m.V != 1 || m.Device != d.ID || !validNonce.MatchString(m.Nonce) ||
		!validProfile.MatchString(m.Profile) || (m.After != "" && (!validTarget(m.After) || m.After == targetTotal)) {
		return refuse("malformed")
	}
	now := s.Now()
	if m.Created <= 0 || m.Created > maxEpoch || now.Sub(time.Unix(m.Created, 0)) >= protocol.ClockSkew || time.Unix(m.Created, 0).Sub(now) > protocol.ClockSkew {
		return refuse("request_expired")
	}
	if m.Profile != d.Profile && !slices.Contains(effectiveApproves(d, ctx), m.Profile) {
		return refuse("not_parent")
	}
	prof, ok := ctx.Profiles[m.Profile]
	if !ok || prof.AppsRev == "" {
		return refuse("app_control_off")
	}
	if s.Inventory == nil || prof.AppInput == nil {
		return refuse("unavailable")
	}
	policy, _, err := prof.AppInput.Compile()
	if err != nil || policy == nil {
		return refuse("unavailable")
	}
	snapshots, err := s.Inventory.Inventories(m.Profile)
	if err != nil {
		return refuse("unavailable")
	}
	if _, err := s.loadNewApps(); err != nil {
		return refuse("unavailable")
	}

	// Current registry phones of this profile, in a stable order. A snapshot from an
	// old key or a phone moved elsewhere is not a current source.
	type entry struct {
		label             string
		labelAt           int64
		on, system, essen []int
	}
	apps := map[string]*entry{}
	sources := []Reply{}
	digest := sha256.New()
	var phones []Device
	for _, p := range s.devices {
		if p.Profile == m.Profile {
			phones = append(phones, p)
		}
	}
	sort.Slice(phones, func(i, j int) bool { return phones[i].ID < phones[j].ID })
	for i, p := range phones {
		src := Reply{"name": nameOr(p.Name, p.ID), "reported": false}
		v, found := snapshots[usageKey(p)]
		if found {
			src = Reply{"name": nameOr(p.Name, p.ID), "reported": true, "received": v.Received,
				"installed": v.Installed, "sent": len(v.Apps)}
			for _, a := range v.Apps {
				e := apps[a.Package]
				if e == nil {
					e = &entry{}
					apps[a.Package] = e
				}
				if a.Label != "" && v.Received >= e.labelAt { // newest report names it
					e.label, e.labelAt = a.Label, v.Received
				}
				e.on = append(e.on, i)
				if a.System {
					e.system = append(e.system, i)
				}
				if a.Essential {
					e.essen = append(e.essen, i)
				}
			}
		}
		fmt.Fprintf(digest, "%s %s %d %d\n", p.ID, usageKey(p), v.Seq, v.Received)
		sources = append(sources, src)
	}
	rule := map[string]Reply{}
	set := func(pkg string, r Reply) {
		if rule[pkg] == nil {
			rule[pkg] = r
		}
	}
	// Precedence follows the phone: blocked, always allowed, own limit, approved.
	for _, pkg := range policy.Blocked {
		set(pkg, Reply{"rule": "blocked"})
	}
	for _, pkg := range policy.AlwaysAllowed {
		set(pkg, Reply{"rule": "always"})
	}
	for _, l := range policy.Limits {
		set(l.Package, Reply{"rule": "limit", "minutes": l.Minutes})
	}
	for _, pkg := range append(slices.Clone(policy.Approved), s.approvedFor(m.Profile)...) {
		set(pkg, Reply{"rule": "allowed"})
	}
	for pkg := range rule {
		if apps[pkg] == nil {
			apps[pkg] = &entry{} // configured but not reported by any current phone
		}
	}
	fmt.Fprintf(digest, "%s %t\n", prof.AppsRev, policy.SuspendNew)
	names := make([]string, 0, len(apps))
	for pkg := range apps {
		if pkg > m.After {
			names = append(names, pkg)
		}
	}
	sort.Strings(names)
	page, used, more := []Reply{}, 0, false
	for _, pkg := range names {
		e := apps[pkg]
		r := Reply{"package": pkg, "label": e.label, "on": nonNilInts(e.on), "system": nonNilInts(e.system),
			"essential": nonNilInts(e.essen), "rule": "none"}
		if x := rule[pkg]; x != nil {
			for k, v := range x {
				r[k] = v
			}
		}
		b, _ := json.Marshal(r)
		if used+len(b)+1 > installedPageBytes {
			more = true
			break
		}
		page, used = append(page, r), used+len(b)+1
	}
	return object{{"profile_id", m.Profile}, {"after", m.After}, {"snapshot", hex.EncodeToString(digest.Sum(nil))[:32]},
		{"apps_rev", prof.AppsRev}, {"suspend_new", policy.SuspendNew}, {"sources", sources}, {"apps", page}, {"more", more}}
}

func nonNilInts(v []int) []int {
	if v == nil {
		return []int{}
	}
	return v
}
