package identity

import (
	"errors"
	"log"
	"sort"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

// Usage reports (protocol §5): a phone uploads its cumulative, router-local-day counts.
// They are information for parents and the administrator, never authority: no limit,
// grant or pact decision on any phone reads them (yet), so a lying phone can only
// misreport itself.

// UsageStore is the core store's usage part (internal/store).
type UsageStore interface {
	ApplyUsage(profile string, r store.Report) (bool, error)
	UsageDay(profile, day string) (map[string]store.Report, error)
}

type usageMsg struct {
	Type       string            `json:"type"`
	V          int               `json:"v"`
	Device     string            `json:"device"`
	Profile    string            `json:"profile"`
	Day        string            `json:"day"`
	Seq        int64             `json:"seq"`
	Total      int               `json:"total"`
	Apps       map[string]int    `json:"apps"`
	Labels     map[string]string `json:"labels"`
	Grants     map[string]int    `json:"grants_used"`
	Incomplete bool              `json:"incomplete"`
	NewApps    map[string]string `json:"new_apps"`
}

func usageKey(d Device) string { return d.ID + "@" + fingerprint(d.PubKey)[:16] }

func routerZone(ctx platform.Context) *time.Location {
	if loc, err := time.LoadLocation(ctx.TimeZone); err == nil && ctx.TimeZone != "" {
		return loc
	}
	return time.Local
}

// usage handles op=usage under s.mu (see uploadLocked).
func (s *Service) usage(d Device, raw []byte, ctx platform.Context) object {
	var m usageMsg
	if !decodeStrict(raw, &m, "type v device profile day seq total apps", "labels grants_used incomplete new_apps") ||
		m.Type != "usage" || m.V != 1 || m.Device != d.ID {
		return refuse("malformed")
	}
	out := object{{"day", m.Day}, {"seq", m.Seq}}
	if s.Usage == nil {
		return append(out, refuse("unavailable")...)
	}
	if m.Profile != d.Profile {
		return append(out, refuse("wrong_profile")...)
	}
	if prof, ok := ctx.Profiles[d.Profile]; !ok || prof.AppsRev == "" {
		return append(out, refuse("app_control_off")...)
	}
	today := s.Now().In(routerZone(ctx))
	day, err := time.ParseInLocation(time.DateOnly, m.Day, today.Location())
	if err != nil || day.Format(time.DateOnly) != m.Day || day.After(today) ||
		day.Before(time.Date(today.Year(), today.Month(), today.Day()-(store.Retention-1), 0, 0, 0, 0, today.Location())) {
		return append(out, refuse("day_out_of_range")...)
	}
	if len(m.Grants) > 0 {
		ledger, err := s.loadLedger()
		if err != nil {
			return append(out, refuse("unavailable")...)
		}
		for id, secs := range m.Grants {
			e := s.grantByID(ledger, id)
			if e == nil || e.Device != d.ID || e.DeviceKey != d.PubKey || secs > e.Granted*60 {
				return append(out, refuse("unknown_grant")...)
			}
		}
	}
	if m.NewApps != nil {
		if len(m.NewApps) > maxNewPending {
			return append(out, refuse("malformed")...)
		}
		for pkg, label := range m.NewApps {
			if _, ok := cleanText(label, 64); !ok || !validTarget(pkg) || pkg == targetTotal {
				return append(out, refuse("malformed")...)
			}
		}
		// Today's report only: yesterday's report must not resurrect apps.
		if m.Day == today.Format(time.DateOnly) {
			if err := s.reportNewApps(d, m.NewApps); err != nil {
				log.Printf("new apps from %s: %v", d.ID, err)
				return append(out, refuse("unavailable")...)
			}
		}
	}
	changed, err := s.Usage.ApplyUsage(d.Profile, store.Report{Device: usageKey(d), Name: nameOr(d.Name, d.ID),
		Day: m.Day, Seq: m.Seq, Total: m.Total, Apps: m.Apps, Labels: m.Labels, Grants: m.Grants,
		Incomplete: m.Incomplete, Received: s.Now().Unix()})
	switch {
	case errors.Is(err, store.ErrRegressed):
		return append(out, refuse("usage_regressed")...)
	case errors.Is(err, store.ErrFull), errors.Is(err, store.ErrInvalidID):
		return append(out, refuse("malformed")...)
	case err != nil:
		log.Printf("usage from %s: %v", d.ID, err)
		return append(out, refuse("unavailable")...)
	}
	return append(out, field{"state", map[bool]string{true: "stored", false: "unchanged"}[changed]})
}

func (s *Service) grantByID(ledger []entry, id string) *entry {
	for i := range ledger {
		if ledger[i].GrantID == id {
			return &ledger[i]
		}
	}
	return nil
}

type appUse struct {
	Package string `json:"package"`
	Label   string `json:"label,omitempty"`
	Seconds int    `json:"seconds"`
}

// dayUsage aggregates a profile's phones for one day. Totals are summed across phones
// (separate screens); apps are summed per package and sorted by time, then name.
func (s *Service) dayUsage(profile, day string, topApps int) (total int, devices []Reply, apps []appUse, grants map[string]int, err error) {
	reports, err := s.Usage.UsageDay(profile, day)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	byApp, labels, grants := map[string]int{}, map[string]string{}, map[string]int{}
	keys := make([]string, 0, len(reports))
	for k := range reports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := reports[k]
		total += r.Total
		devices = append(devices, Reply{"name": r.Name, "total": r.Total, "incomplete": r.Incomplete, "received": r.Received})
		for pkg, secs := range r.Apps {
			byApp[pkg] += secs
			if r.Labels[pkg] != "" {
				labels[pkg] = r.Labels[pkg]
			}
		}
		for id, secs := range r.Grants {
			grants[id] = max(grants[id], secs)
		}
	}
	for pkg, secs := range byApp {
		if secs > 0 {
			apps = append(apps, appUse{pkg, labels[pkg], secs})
		}
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Seconds != apps[j].Seconds {
			return apps[i].Seconds > apps[j].Seconds
		}
		return apps[i].Package < apps[j].Package
	})
	if len(apps) > topApps {
		apps = apps[:topApps]
	}
	return total, devices, apps, grants, nil
}

// parentUsage is today's usage of the profiles a parent phone may approve for (lock held).
func (s *Service) parentUsage(d Device, ctx platform.Context) []object {
	if s.Usage == nil {
		return nil
	}
	day := s.Now().In(routerZone(ctx)).Format(time.DateOnly)
	out := []object{}
	for _, p := range effectiveApproves(d, ctx) {
		if ctx.Profiles[p].AppsRev == "" {
			continue
		}
		total, devices, apps, _, err := s.dayUsage(p, day, 5)
		if err != nil {
			continue
		}
		phones := []object{}
		for _, dev := range devices {
			phones = append(phones, object{{"name", dev["name"]}, {"total", dev["total"]}, {"incomplete", dev["incomplete"]}})
		}
		out = append(out, object{{"profile_id", p}, {"day", day}, {"total", total}, {"devices", phones}, {"apps", nonNilApps(apps)}})
	}
	return out
}

func nonNilApps(a []appUse) []appUse {
	if a == nil {
		return []appUse{}
	}
	return a
}

// UsageReport is the administrator's view: today per phone and app, plus 7 daily totals.
func (s *Service) UsageReport() Reply {
	if s.Usage == nil {
		return fail("usage unavailable")
	}
	ctx, err := s.ad.Context()
	if err != nil {
		return fail("platform state unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now().In(routerZone(ctx))
	ids := make([]string, 0, len(ctx.Profiles))
	for id, p := range ctx.Profiles {
		if p.AppsRev != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	profiles := []Reply{}
	for _, id := range ids {
		total, devices, apps, _, err := s.dayUsage(id, now.Format(time.DateOnly), 20)
		if err != nil {
			return fail("usage unavailable")
		}
		week := []Reply{}
		for back := 6; back >= 0; back-- {
			day := time.Date(now.Year(), now.Month(), now.Day()-back, 12, 0, 0, 0, now.Location()).Format(time.DateOnly)
			t, _, _, _, err := s.dayUsage(id, day, 0)
			if err != nil {
				return fail("usage unavailable")
			}
			week = append(week, Reply{"day": day, "total": t})
		}
		if devices == nil {
			devices = []Reply{}
		}
		profiles = append(profiles, Reply{"id": id, "name": ctx.Profiles[id].Name, "total": total,
			"devices": devices, "apps": nonNilApps(apps), "week": week})
	}
	return Reply{"day": now.Format(time.DateOnly), "profiles": profiles}
}

const sharedFeature = "shared-v1"

// sharedUsage is today's usage of the profile's OTHER phones (D4 shared budget), for
// the phone's signed policy (lock held). Only tightens: the phone adds it to its own
// counters. nil when there is nothing to add. Not part of the app revision, so it
// does not churn acknowledgements; it is required (shared-v1) whenever present.
func (s *Service) sharedUsage(d Device, ctx platform.Context) (object, error) {
	if s.Usage == nil || ctx.Profiles[d.Profile].AppsRev == "" {
		return nil, nil
	}
	day := s.Now().In(routerZone(ctx)).Format(time.DateOnly)
	reports, err := s.Usage.UsageDay(d.Profile, day)
	if err != nil {
		return nil, err
	}
	own, total, apps := usageKey(d), 0, map[string]int{}
	for key, r := range reports {
		if key == own {
			continue
		}
		total += r.Total
		for pkg, secs := range r.Apps {
			apps[pkg] += secs
		}
	}
	list := make([]appUse, 0, len(apps))
	for pkg, secs := range apps {
		if secs > 0 {
			list = append(list, appUse{Package: pkg, Seconds: secs})
		}
	}
	if total == 0 && len(list) == 0 {
		return nil, nil
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Seconds != list[j].Seconds {
			return list[i].Seconds > list[j].Seconds
		}
		return list[i].Package < list[j].Package
	})
	top := map[string]int{}
	for _, a := range list[:min(len(list), 64)] {
		top[a.Package] = a.Seconds
	}
	return object{{"day", day}, {"total", total}, {"apps", top}}, nil
}

func withFeature(list []string, feature string) []string {
	for _, f := range list {
		if f == feature {
			return list
		}
	}
	out := append(append([]string{}, list...), feature)
	sort.Strings(out)
	return out
}

// grantUse is how many seconds of a grant the requesting phone reported (lock held).
func (s *Service) grantUse(e entry, ctx platform.Context) (int, bool) {
	if s.Usage == nil || e.GrantID == "" {
		return 0, false
	}
	zone := routerZone(ctx)
	used, seen := 0, false
	for _, at := range []int64{e.DecidedAt, e.NotAfter, s.Now().Unix()} {
		day := time.Unix(at, 0).In(zone).Format(time.DateOnly)
		reports, err := s.Usage.UsageDay(e.Profile, day)
		if err != nil {
			continue
		}
		for _, r := range reports {
			if secs, ok := r.Grants[e.GrantID]; ok {
				used, seen = max(used, secs), true
			}
		}
	}
	return used, seen
}
