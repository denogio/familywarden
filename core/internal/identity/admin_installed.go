package identity

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

// AdminInstalledQuery is a read-only administrator query, not a phone operation.
// Page numbers are zero-based; counts always describe saved rules, not UI drafts.
type AdminInstalledQuery struct {
	Profile string `json:"profile"`
	Search  string `json:"search"`
	Filter  string `json:"filter"`
	Page    int    `json:"page"`
}

func DecodeAdminInstalledQuery(raw []byte) (AdminInstalledQuery, error) {
	var q AdminInstalledQuery
	if !decodeStrict(raw, &q, "profile", "search filter page") {
		return q, errors.New("malformed inventory query")
	}
	return q, nil
}

func (s *Service) AdminInstalled(q AdminInstalledQuery) Reply {
	if !validProfile.MatchString(q.Profile) || len(q.Search) > 256 || q.Page < 0 || q.Page > 100000 ||
		(q.Filter != "" && q.Filter != "all" && q.Filter != "limit" && q.Filter != "unlimited" && q.Filter != "blocked") {
		return fail("malformed request")
	}
	var ctx platform.Context
	if err := s.ad.WithContext(func(c platform.Context) error { ctx = c; return nil }); err != nil {
		return fail("inventory unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := s.installedInventory(q.Profile, ctx)
	out := Reply{}
	for _, f := range raw {
		out[f.k] = f.v
	}
	apps, ok := out["apps"].([]Reply)
	if !ok {
		return out
	}
	pending := map[string]bool{}
	seen := map[string]bool{}
	for _, a := range apps {
		seen[a["package"].(string)] = true
	}
	currentDevices := map[string]bool{}
	for _, d := range s.devices {
		if d.Profile == q.Profile {
			currentDevices[d.ID] = true
		}
	}
	// Pending observations may precede the next full inventory snapshot. They are
	// still observations, not evidence of installation or a new approval authority.
	for _, a := range s.visiblePending(q.Profile, s.Now().Unix()) {
		r := Reply{"on": []int{}, "rule": "none"}
		for _, f := range a {
			r[f.k] = f.v
		}
		pkg := r["package"].(string)
		if !currentDevices[s.newApps[q.Profile].Pending[pkg].Device] {
			continue
		}
		pending[pkg] = true
		if !seen[pkg] {
			apps = append(apps, r)
		}
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i]["package"].(string) < apps[j]["package"].(string) })
	counts := Reply{"all": len(apps), "limit": 0, "unlimited": 0, "blocked": 0, "pending": 0}
	usage := map[string]int{}
	usageAvailable := false
	if s.Usage != nil {
		reports, err := s.Usage.UsageDay(q.Profile, s.Now().In(routerZone(ctx)).Format(time.DateOnly))
		if err == nil {
			for _, d := range s.devices {
				if d.Profile == q.Profile {
					if r, found := reports[usageKey(d)]; found {
						usageAvailable = true
						for pkg, seconds := range r.Apps {
							usage[pkg] += seconds
						}
					}
				}
			}
		}
	}
	matched := []Reply{}
	search := strings.ToLower(strings.TrimSpace(q.Search))
	for _, a := range apps {
		pkg := a["package"].(string)
		rule := a["rule"].(string)
		if rule == "always" {
			rule = "unlimited"
		}
		if pending[pkg] && (rule == "none" || rule == "allowed") {
			rule = "pending"
		}
		a["rule"] = rule
		if _, ok := counts[rule]; ok {
			counts[rule] = counts[rule].(int) + 1
		}
		if usageAvailable {
			if secs, found := usage[pkg]; found {
				a["seconds"] = secs
			}
		}
		label, _ := a["label"].(string)
		if (q.Filter == "" || q.Filter == "all" || q.Filter == rule) && (search == "" || strings.Contains(strings.ToLower(pkg), search) || strings.Contains(strings.ToLower(label), search)) {
			matched = append(matched, a)
		}
	}
	out["counts"], out["total"], out["page_size"], out["page"] = counts, len(matched), 20, q.Page
	// Build deterministic numbered pages that also respect the wire byte ceiling.
	pages := [][]Reply{{}}
	out["apps"] = []Reply{}
	base, _ := json.Marshal(out)
	used := len(base) + 128
	for _, a := range matched {
		b, _ := json.Marshal(a)
		i := len(pages) - 1
		if len(pages[i]) == 20 || used+len(b)+1 > installedPageBytes {
			if len(pages[i]) == 0 {
				return fail("inventory unavailable")
			}
			pages = append(pages, []Reply{})
			used = len(base) + 128
			i++
		}
		pages[i] = append(pages[i], a)
		used += len(b) + 1
	}
	out["pages"] = len(pages)
	out["apps"], out["start"] = []Reply{}, 0
	if q.Page < len(pages) {
		out["apps"] = pages[q.Page]
		start := 0
		for i := 0; i < q.Page; i++ {
			start += len(pages[i])
		}
		out["start"] = start
	}
	encoded, _ := json.Marshal(out)
	if len(encoded) > installedPageBytes {
		return fail("inventory unavailable")
	}
	return out
}
