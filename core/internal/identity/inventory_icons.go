package identity

import (
	"sort"

	"github.com/denogio/familywarden/core/internal/platform"
)

// Inventory icons are observations from current enrolled keys, never authority.
// Ignore unavailable observations: administration still works with neutral tiles.
func (s *Service) inventoryIcons(profile string, ctx platform.Context) map[string]string {
	out := map[string]string{}
	if s.Inventory == nil || ctx.Profiles[profile].AppsRev == "" {
		return out
	}
	snapshots, err := s.Inventory.Inventories(profile)
	if err != nil {
		return out
	}
	keys := []string{}
	for _, d := range s.devices {
		if d.Profile == profile {
			keys = append(keys, usageKey(d))
		}
	}
	sort.Strings(keys)
	newest := map[string]int64{}
	for _, key := range keys {
		v, ok := snapshots[key]
		if !ok {
			continue
		}
		for _, a := range v.Apps {
			if a.Icon != "" && v.Received >= newest[a.Package] {
				out[a.Package], newest[a.Package] = a.Icon, v.Received
			}
		}
	}
	return out
}

// One budget per administrator response, including repeated packages/rows.
type iconBudget int

func (b *iconBudget) take(icon string) string {
	if len(icon) > int(*b) {
		return ""
	}
	*b -= iconBudget(len(icon))
	return icon
}

const adminIconBudget iconBudget = 16 << 10
