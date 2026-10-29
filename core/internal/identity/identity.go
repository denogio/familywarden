// Package identity owns phone enrollment, the device registry and the signed
// phone sync endpoint (plans/core-migration.md, step 1). It is router-agnostic:
// platform state comes only through platform.Adapter.
//
// The wire format is exactly what the shell plugin served before (protocol
// §2a/§2b), so enrolled phones notice nothing.
package identity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
	"github.com/denogio/familywarden/core/internal/usign"
)

const (
	devicesFile   = "devices.json"
	schemaVersion = 1
	maxPending    = 10
	maxApproves   = 16
	maxParents    = 16
	removalWindow = time.Hour
)

var (
	validDevice  = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)
	validProfile = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	validNonce   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	validRev     = regexp.MustCompile(`^[0-9a-f]{16}$`)
	validCaps    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}(,[a-z][a-z0-9-]{0,31}){0,15}$`)
	b64Chars     = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
	validTZ      = regexp.MustCompile(`^[A-Za-z0-9_+/-]+$`)
)

// Device is an approved phone. Its key is the identity; display names never grant anything.
type Device struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Profile       string   `json:"profile"`
	PubKey        string   `json:"pubkey"`
	Administrator bool     `json:"administrator"`
	Approves      []string `json:"approves,omitempty"` // child profiles it may approve for
}

type devicesDoc struct {
	Version int      `json:"version"`
	Devices []Device `json:"devices"`
}

// Storage is the core store (internal/store) as far as identity needs it.
type Storage interface {
	ReadDocument(name string, v any) (bool, error)
	WriteDocument(name string, v any) error
}

type pendingPhone struct {
	key, name   string
	first, last int64
}

// seenPhone is RAM only, as before: last fetch, signed acknowledgement,
// authenticated features and the last refusal.
type seenPhone struct {
	at      int64
	applied string
	caps    string
	err     string
}

type Service struct {
	mu         sync.Mutex
	ad         platform.Adapter
	st         Storage
	Now        func() time.Time
	devices    []Device
	pending    map[string]pendingPhone
	seen       map[string]seenPhone
	removals   map[string]int64
	key        *usign.SecretKey
	ledger     []entry          // requests/grants, loaded lazily (requests.go)
	ledgerTime int64            // last durable lifecycle transition (clock rollback guard)
	delivered  map[string]int64 // grant id -> first acknowledgement (RAM)
	// Usage stores phone usage reports; nil disables them (set by main before serving).
	Usage UsageStore
	// Inventory stores installed-app snapshots; nil disables them.
	Inventory InventoryStore
	// Generation vouches for signed policy freshness (store.Generation); nil omits it.
	Generation func() (store.Generation, error)
	newApps    map[string]*profileApps // new-app approvals, loaded lazily (newapps.go)
}

// New loads the registry. On first start it imports the platform's legacy phones;
// the legacy copy is removed only after the import is persisted and re-read equal.
func New(ad platform.Adapter, st Storage) (*Service, error) {
	s := &Service{ad: ad, st: st, Now: time.Now, pending: map[string]pendingPhone{},
		seen: map[string]seenPhone{}, removals: map[string]int64{}, delivered: map[string]int64{}}
	var doc devicesDoc
	found, err := st.ReadDocument(devicesFile, &doc)
	if err != nil {
		return nil, fmt.Errorf("devices: %w", err)
	}
	if found {
		if doc.Version != schemaVersion {
			return nil, fmt.Errorf("devices: unsupported schema version %d", doc.Version)
		}
		s.devices = doc.Devices
		return s, nil
	}
	legacy, err := ad.LegacyDevices()
	if err != nil {
		return nil, fmt.Errorf("legacy devices: %w", err)
	}
	for _, l := range legacy {
		d, err := fromLegacy(l)
		if err != nil {
			log.Printf("legacy device %q not imported: %v", l.ID, err)
			continue
		}
		s.devices = append(s.devices, d)
	}
	if err := s.save(); err != nil {
		return nil, fmt.Errorf("import devices: %w", err)
	}
	var check devicesDoc
	if ok, err := st.ReadDocument(devicesFile, &check); err != nil || !ok || !sameDevices(check.Devices, s.devices) {
		return nil, fmt.Errorf("import devices: read-back mismatch (%v)", err)
	}
	if err := ad.RemoveLegacyDevices(); err != nil {
		// Harmless leftovers: nothing reads them, and the import never runs twice.
		log.Printf("legacy devices imported but not removed: %v", err)
	}
	log.Printf("imported %d phone(s) into the core store", len(s.devices))
	return s, nil
}

func fromLegacy(l platform.LegacyDevice) (Device, error) {
	if !validDevice.MatchString(l.ID) || !validProfile.MatchString(l.Profile) {
		return Device{}, fmt.Errorf("invalid id or profile")
	}
	if _, err := usign.ParsePublicKey(l.PubKey); err != nil {
		return Device{}, err
	}
	d := Device{ID: l.ID, Name: cleanName(l.Name), Profile: l.Profile, PubKey: l.PubKey,
		Administrator: l.Administrator == "1"}
	for _, p := range strings.Fields(l.Approves) {
		if validProfile.MatchString(p) && p != l.Profile {
			d.Approves = append(d.Approves, p)
		}
	}
	d.Approves = canonical(d.Approves)
	return d, nil
}

func sameDevices(a, b []Device) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (s *Service) save() error {
	return s.st.WriteDocument(devicesFile, devicesDoc{Version: schemaVersion, Devices: s.devices})
}

func (s *Service) find(id string) int {
	for i, d := range s.devices {
		if d.ID == id {
			return i
		}
	}
	return -1
}

func (s *Service) routerKey() (usign.SecretKey, error) {
	if s.key == nil {
		k, err := s.ad.RouterKey()
		if err != nil {
			return usign.SecretKey{}, err
		}
		s.key = &k
	}
	return *s.key, nil
}

// ---- helpers shared with the shell's former rules ----

func cleanName(n string) string {
	n = strings.NewReplacer("\r", "", "\n", "").Replace(n)
	if r := []rune(n); len(r) > 64 {
		n = string(r[:64])
	}
	return n
}

func canonical(list []string) []string {
	set := map[string]bool{}
	for _, p := range list {
		set[p] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func validB64(s string, n int) bool { return len(s) == n && b64Chars.MatchString(s) }

// coverage: an empty list from a v3 phone that predates capability signatures
// means the original app-control implementation only (protocol §2b).
func covers(caps string, version int, required []string) bool {
	if caps == "" && version == 3 {
		caps = "apps-v1"
	}
	have := map[string]bool{}
	for _, c := range strings.Split(caps, ",") {
		have[c] = true
	}
	for _, r := range required {
		if !have[r] {
			return false
		}
	}
	return true
}

func canonicalCaps(c string) bool {
	if c == "-" {
		return true
	}
	if !validCaps.MatchString(c) {
		return false
	}
	parts := strings.Split(c, ",")
	return strings.Join(canonical(parts), ",") == c && len(canonical(parts)) == len(parts)
}

// effectiveApproves is the scope signed now: existing profiles, never the phone's
// own and never a pact-bound captain (A4/A17: no way out of a pact).
func effectiveApproves(d Device, ctx platform.Context) []string {
	var out []string
	for _, p := range d.Approves {
		if _, ok := ctx.Profiles[p]; ok && p != d.Profile && p != ctx.PactCaptain {
			out = append(out, p)
		}
	}
	return out
}

func frozen(d Device, ctx platform.Context) bool {
	return ctx.PactCaptain != "" && d.Profile == ctx.PactCaptain
}

// ---- ordered JSON for replies (field order is cosmetic, but stable output helps diffs) ----

type field struct {
	k string
	v any
}

type object []field

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.k)
		b.Write(k)
		b.WriteByte(':')
		v, err := json.Marshal(f.v)
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
