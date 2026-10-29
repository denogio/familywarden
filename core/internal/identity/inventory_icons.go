package identity

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

const maxIconJSON = 60 << 10

type appIconStore interface {
	ApplyAppIcon(profile, device string, inventorySeq, seq int64, pkg, icon string) (bool, error)
	AppIcons(profile, device string, packages []string) (map[string]string, error)
}

func (s *Service) uploadInventoryIcons(d Device, raw []byte, ctx platform.Context) object {
	var m struct {
		Type         string            `json:"type"`
		V            int               `json:"v"`
		Device       string            `json:"device"`
		Profile      string            `json:"profile"`
		InventorySeq int64             `json:"inventory_seq"`
		Seq          int64             `json:"seq"`
		Icons        []json.RawMessage `json:"icons"`
	}
	if len(raw) > maxIconJSON || !decodeStrict(raw, &m, "type v device profile inventory_seq seq icons", "") ||
		json.Unmarshal(raw, &m) != nil || m.Type != "inventory_icons" || m.V != 1 || m.Device != d.ID || m.Seq < 1 || m.Seq > maxEpoch || m.InventorySeq < 1 || m.InventorySeq > maxEpoch || len(m.Icons) < 1 || len(m.Icons) > 32 {
		return refuse("malformed")
	}
	if m.Profile != d.Profile {
		return refuse("wrong_profile")
	}
	if ctx.Profiles[d.Profile].AppsRev == "" {
		return refuse("app_control_off")
	}
	cache, ok := s.Inventory.(appIconStore)
	if !ok {
		return refuse("unavailable")
	}
	type item struct {
		Package string `json:"package"`
		Icon    string `json:"icon"`
	}
	icons := make([]item, len(m.Icons))
	snapshots, err := s.Inventory.Inventories(d.Profile)
	if err != nil {
		return refuse("unavailable")
	}
	v, ok := snapshots[usageKey(d)]
	if !ok || v.Seq != m.InventorySeq {
		return refuse("replay")
	}
	packages := map[string]bool{}
	for _, a := range v.Apps {
		packages[a.Package] = true
	}
	for i, raw := range m.Icons {
		a := &icons[i]
		if !decodeStrict(raw, a, "package icon", "") || json.Unmarshal(raw, a) != nil || !packages[a.Package] || !store.ValidAppIcon(a.Icon) || (i > 0 && icons[i-1].Package >= a.Package) {
			return refuse("malformed")
		}
	}
	changed := false
	for _, a := range icons {
		wrote, err := cache.ApplyAppIcon(d.Profile, usageKey(d), m.InventorySeq, m.Seq, a.Package, a.Icon)
		if err != nil {
			return refuse("unavailable")
		}
		changed = changed || wrote
	}
	return object{{"state", map[bool]string{true: "stored", false: "unchanged"}[changed]}}
}

// IconQuery is an administrator-only, bounded read, never phone authority.
// Offset counts requested packages, including unavailable images, so every page
// makes progress without revealing observations outside the requested profile.
type IconQuery struct {
	Profile  string   `json:"profile"`
	Packages []string `json:"packages"`
	Offset   int      `json:"offset"`
}

func DecodeIconQuery(raw []byte) (IconQuery, error) {
	var q IconQuery
	if !decodeStrict(raw, &q, "profile packages", "offset") {
		return q, errors.New("malformed icon query")
	}
	return q, nil
}

func (s *Service) InventoryIcons(q IconQuery) Reply {
	if len(q.Packages) < 1 || len(q.Packages) > 32 || q.Offset < 0 || q.Offset >= len(q.Packages) {
		return fail("malformed request")
	}
	for _, p := range q.Packages {
		if !validTarget(p) || p == targetTotal {
			return fail("malformed request")
		}
	}
	var ctx platform.Context
	if err := s.ad.WithContext(func(c platform.Context) error { ctx = c; return nil }); err != nil {
		return fail("icons unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Profiles[q.Profile].AppsRev == "" {
		return fail("unknown profile")
	}
	icons := s.inventoryIconsFor(q.Profile, ctx, q.Packages[q.Offset:])
	result := Reply{"icons": map[string]string{}, "next": q.Offset}
	selected := result["icons"].(map[string]string)
	for i := q.Offset; i < len(q.Packages); i++ {
		p := q.Packages[i]
		if icon := icons[p]; icon != "" {
			selected[p] = icon
		}
		result["next"] = i + 1
		raw, _ := json.Marshal(result)
		if len(raw)+1 > maxIconJSON {
			delete(selected, p)
			result["next"] = i
			break
		}
	}
	return result
}

// Only observations from currently enrolled keys in the requested profile.
func (s *Service) inventoryIcons(profile string, ctx platform.Context) map[string]string {
	return s.inventoryIconsFor(profile, ctx, nil)
}

func (s *Service) inventoryIconsFor(profile string, ctx platform.Context, packages []string) map[string]string {
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
		cached := map[string]string{}
		if cache, ok := s.Inventory.(appIconStore); ok {
			cached, err = cache.AppIcons(profile, key, packages)
			if err != nil {
				continue
			}
		}
		for _, a := range v.Apps {
			icon := cached[a.Package]
			if icon == "" {
				icon = a.Icon
			}
			if icon != "" && v.Received >= newest[a.Package] {
				out[a.Package], newest[a.Package] = icon, v.Received
			}
		}
	}
	return out
}
