package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/denogio/familywarden/core/internal/platform"
)

// Reply is one JSON object for LuCI/rpcd: either fields or {"error": "..."}.
type Reply map[string]any

func fail(msg string) Reply { return Reply{"error": msg} }

func ok() Reply { return Reply{"status": "ok"} }

// mutate keeps identity validation and persistence inside the platform pact gate.
// Callbacks must not call the adapter; enforcement runs after returning.
func (s *Service) mutate(fn func(platform.Context) Reply) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Reply
	err := s.ad.WithContext(func(ctx platform.Context) error {
		r = fn(ctx)
		return nil
	})
	if err != nil || r == nil {
		return fail("platform state unavailable")
	}
	return r
}

// fingerprint is SHA-256 of the full base64 key, as LuCI and the app show it.
func fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// ConfigurationExport is the actual migration reader, not an authoritative store.
// Until the reader/writer cutover, UCI and the legacy pact files still own this data.
func (s *Service) ConfigurationExport() Reply {
	c, err := s.ad.Configuration()
	if err != nil {
		return fail("configuration unavailable")
	}
	if err := c.Seal(); err != nil {
		return fail("configuration unavailable")
	}
	return Reply{"owner": "legacy", "configuration": c}
}

// ProfileDirectory lists every profile, including ones disabled for network
// enforcement. LuCI selectors must not discover profiles directly through UCI.
// Only display identity is exposed; configuration ownership migrates separately.
func (s *Service) ProfileDirectory() Reply {
	ctx, err := s.ad.Context()
	if err != nil {
		return fail("platform state unavailable")
	}
	ids := make([]string, 0, len(ctx.Profiles))
	for id := range ctx.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	profiles := make([]Reply, 0, len(ids))
	for _, id := range ids {
		profiles = append(profiles, Reply{"id": id, "name": nameOr(ctx.Profiles[id].Name, id)})
	}
	return Reply{"profiles": profiles}
}

// Devices lists approved and pending phones for the administrator.
func (s *Service) Devices() Reply {
	ctx, err := s.ad.Context()
	if err != nil {
		return fail("platform state unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now().Unix()
	if _, err := s.loadLedger(); err != nil {
		return fail("requests unavailable")
	}
	if _, err := s.loadNewApps(); err != nil {
		return fail("new apps unavailable")
	}
	devices := []Reply{}
	for _, d := range s.devices {
		seen := s.seen[d.ID]
		prof := ctx.Profiles[d.Profile]
		var grants []entry
		if d.Profile != ctx.PactCaptain {
			grants = s.grantsFor(d.Profile, d.ID, now)
		}
		var approved []string
		if prof.AppsRev != "" {
			approved = s.approvedFor(d.Profile)
		}
		rev, requires := appsView(prof, grants, approved)
		if shared, _ := s.sharedUsage(d, ctx); shared != nil {
			requires = withFeature(requires, sharedFeature)
		}
		applied, apErr := seen.applied, seen.err
		// An old acknowledgement must not look current for an incompatible phone.
		if len(requires) == 0 {
			apErr = ""
		} else if applied != "" && !covers(seen.caps, 3, requires) {
			applied, apErr = "", "unsupported_capabilities"
		}
		caps := []string{}
		for _, c := range strings.Split(seen.caps, ",") {
			if c != "" && c != "-" {
				caps = append(caps, c)
			}
		}
		removal := s.removals[d.ID]
		if removal <= now {
			removal = 0
		}
		admin := "0"
		if d.Administrator {
			admin = "1"
		}
		devices = append(devices, Reply{
			"id": d.ID, "seen": strconv.FormatInt(seen.at, 10),
			"apps_applied": applied, "apps_error": apErr, "capabilities": caps,
			"approves": strings.Join(d.Approves, " "), "approves_effective": strings.Join(effectiveApproves(d, ctx), " "),
			"name": d.Name, "profile": d.Profile, "administrator": admin,
			"apps_rev": rev, "fingerprint": fingerprint(d.PubKey),
			"remove_until": removal, "frozen": frozen(d, ctx),
		})
	}
	ids := make([]string, 0, len(s.pending))
	for id := range s.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pending := []Reply{}
	for _, id := range ids {
		p := s.pending[id]
		pending = append(pending, Reply{"id": id, "fingerprint": fingerprint(p.key), "name": p.name,
			"first_seen": p.first, "last_seen": p.last})
	}
	return Reply{"devices": devices, "pending": pending}
}

// Change is an approve/update request; nil fields keep the stored value (update).
type Change struct {
	Device        string  `json:"device"`
	Profile       string  `json:"profile"`
	Name          string  `json:"name"`
	Administrator string  `json:"administrator"`
	Fingerprint   string  `json:"fingerprint"`
	Approves      *string `json:"approves"`
}

// Approve enrolls a pending phone, or replaces an approved phone's key with the
// pending one. The fingerprint the administrator compared is sent back: a key
// that changed meanwhile is refused instead of approving an unseen key.
func (s *Service) Approve(c Change) Reply {
	s.mu.Lock()
	p, isPending := s.pending[c.Device]
	s.mu.Unlock()
	if !validDevice.MatchString(c.Device) || !isPending {
		return fail("no pending device")
	}
	if !validB64(p.key, 56) {
		return fail("invalid key")
	}
	approves := ""
	if c.Approves != nil {
		approves = *c.Approves
	}
	admin := c.Administrator
	if admin == "" {
		admin = "0"
	}
	return s.write(c.Device, c.Profile, c.Name, admin, p.key, c.Fingerprint, approves, false)
}

func (s *Service) Update(c Change) Reply {
	approves, keep := "", c.Approves == nil
	if !keep {
		approves = *c.Approves
	}
	return s.write(c.Device, c.Profile, c.Name, c.Administrator, "", "", approves, keep)
}

// write is shared by approve/update and refuses changes to a pact-frozen phone.
func (s *Service) write(id, profile, name, admin, key, fpr, approves string, keepApproves bool) Reply {
	if !validDevice.MatchString(id) {
		return fail("invalid device")
	}
	return s.mutate(func(ctx platform.Context) Reply {
		i := s.find(id)
		// An update's omitted fields come from the current row under the same lock
		// as validation and persistence, never from an earlier snapshot.
		if key == "" && i >= 0 {
			d := s.devices[i]
			if profile == "" {
				profile = d.Profile
			}
			if name == "" {
				name = d.Name
			}
			if admin == "" && d.Administrator {
				admin = "1"
			}
		}
		if admin == "" {
			admin = "0"
		}
		if admin != "0" && admin != "1" {
			return fail("administrator must be 0 or 1")
		}
		name = cleanName(name)
		if !validProfile.MatchString(profile) {
			return fail("invalid profile")
		}
		if _, ok := ctx.Profiles[profile]; !ok {
			return fail("profile not found")
		}
		if i >= 0 && frozen(s.devices[i], ctx) {
			return fail("bound by Ulysses pact")
		}
		if key != "" {
			if fpr != fingerprint(key) || s.pending[id].key != key {
				return fail("pending device changed; refresh and compare fingerprint")
			}
		} else {
			if i < 0 {
				return fail("device not found")
			}
			key = s.devices[i].PubKey
		}
		var old []string
		if i >= 0 {
			old = s.devices[i].Approves
		}
		list := strings.Fields(approves)
		if keepApproves {
			list = old
		}
		scope, msg := validateApproves(profile, list, old, ctx, keepApproves)
		if msg != "" {
			return fail(msg)
		}
		if i >= 0 && (s.devices[i].Profile != profile || s.devices[i].PubKey != key) {
			if err := s.invalidateInventory(id); err != nil {
				return fail("invalidating inventory failed")
			}
		}
		d := Device{ID: id, Name: name, Profile: profile, PubKey: key, Administrator: admin == "1", Approves: scope}
		prev := append([]Device(nil), s.devices...)
		if i >= 0 {
			s.devices[i] = d
		} else {
			s.devices = append(s.devices, d)
		}
		if err := s.save(); err != nil {
			s.devices = prev
			log.Printf("saving device %s: %v", id, err)
			return fail("saving device failed")
		}
		delete(s.pending, id)
		log.Printf("device %s approved for profile %s (administrator=%s, approves=%s)", id, profile, admin, strings.Join(scope, " "))
		return ok()
	})
}

// validateApproves: existing profiles, not the phone's own, at most 16. New power
// over a pact-bound captain is refused; an entry already stored may be resent
// (forms save everything) and stays unsigned while the pact lasts.
func validateApproves(own string, list, old []string, ctx platform.Context, keep bool) ([]string, string) {
	stored := map[string]bool{}
	for _, p := range old {
		stored[p] = true
	}
	for _, p := range list {
		if !validProfile.MatchString(p) {
			return nil, "invalid profile in approves"
		}
		if _, ok := ctx.Profiles[p]; !ok {
			return nil, "approves: profile not found"
		}
		if p == own {
			return nil, "a phone cannot approve for its own profile"
		}
		if !keep && !stored[p] && ctx.PactCaptain != "" && p == ctx.PactCaptain {
			return nil, "bound by Ulysses pact"
		}
	}
	if len(list) > maxApproves {
		return nil, "too many approved profiles"
	}
	return canonical(list), ""
}

// Reject drops a pending key; it never deletes an approved phone with the same id.
func (s *Service) Reject(id string) Reply {
	if !validDevice.MatchString(id) {
		return fail("invalid device")
	}
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
	return ok()
}

func (s *Service) Delete(id string) Reply {
	if !validDevice.MatchString(id) {
		return fail("invalid device")
	}
	return s.mutate(func(ctx platform.Context) Reply {
		delete(s.pending, id)
		if i := s.find(id); i >= 0 {
			if frozen(s.devices[i], ctx) {
				return fail("bound by Ulysses pact")
			}
			if err := s.invalidateInventory(id); err != nil {
				return fail("invalidating inventory failed")
			}
			prev := append([]Device(nil), s.devices...)
			s.devices = append(s.devices[:i:i], s.devices[i+1:]...)
			if err := s.save(); err != nil {
				s.devices = prev
				return fail("saving device failed")
			}
			log.Printf("device %s removed", id)
		}
		delete(s.removals, id)
		return ok()
	})
}

// AllowRemoval lets one managed phone remove FamilyWarden itself within the hour.
func (s *Service) AllowRemoval(id string) Reply {
	if !validDevice.MatchString(id) {
		return fail("invalid device")
	}
	return s.mutate(func(ctx platform.Context) Reply {
		i := s.find(id)
		if i < 0 {
			return fail("device not found")
		}
		if frozen(s.devices[i], ctx) {
			return fail("bound by Ulysses pact")
		}
		until := s.Now().Add(removalWindow).Unix()
		s.removals[id] = until
		log.Printf("device %s may remove FamilyWarden until %d", id, until)
		return Reply{"status": "ok", "remove_until": until}
	})
}

func (s *Service) invalidateInventory(device string) error {
	if cache, ok := s.Inventory.(interface{ InvalidateInventory(string) error }); ok {
		return cache.InvalidateInventory(device)
	}
	return nil
}
