package identity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"unicode"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

// Installed-app inventory (plans/android-installed-apps.md I1): a phone's own
// snapshot for parents to choose from. Observation only: it never approves, blocks,
// acknowledges or loosens anything, and is kept apart from usage counters.

// InventoryStore is the core store's inventory part (internal/store).
type InventoryStore interface {
	ApplyInventory(profile string, v store.Inventory) (bool, error)
	Inventories(profile string) (map[string]store.Inventory, error)
}

type inventoryApp struct {
	Package   string          `json:"package"`
	Label     string          `json:"label"`
	Icon      json.RawMessage `json:"icon,omitempty"`
	System    bool            `json:"system"`
	Essential bool            `json:"essential"`
}

// inventory handles op=inventory under s.mu (see uploadLocked).
func (s *Service) inventory(d Device, raw []byte, ctx platform.Context) object {
	var m struct {
		Type      string            `json:"type"`
		V         int               `json:"v"`
		Device    string            `json:"device"`
		Profile   string            `json:"profile"`
		Seq       int64             `json:"seq"`
		Installed int               `json:"installed"`
		Apps      []json.RawMessage `json:"apps"`
	}
	if !decodeStrict(raw, &m, "type v device profile seq installed apps", "") || json.Unmarshal(raw, &m) != nil ||
		m.Type != "inventory" || m.V != 1 || m.Device != d.ID || m.Seq < 1 || m.Seq > maxEpoch {
		return refuse("malformed")
	}
	out := object{{"seq", m.Seq}}
	if s.Inventory == nil {
		return append(out, refuse("unavailable")...)
	}
	if m.Profile != d.Profile {
		return append(out, refuse("wrong_profile")...)
	}
	if prof, ok := ctx.Profiles[d.Profile]; !ok || prof.AppsRev == "" {
		return append(out, refuse("app_control_off")...)
	}
	v := store.Inventory{Version: store.InventoryVersion, Device: usageKey(d), Name: nameOr(d.Name, d.ID),
		Seq: m.Seq, Installed: m.Installed, Apps: make([]store.InventoryApp, 0, len(m.Apps)), Received: s.Now().Unix()}
	iconBytes := 0
	for _, item := range m.Apps {
		var a inventoryApp
		if !decodeStrictAllowNull(item, &a, "package label system essential", "icon", "icon") || strictUnmarshal(item, &a) != nil ||
			!validTarget(a.Package) || a.Package == targetTotal {
			return append(out, refuse("malformed")...)
		}
		var icon string
		_ = json.Unmarshal(a.Icon, &icon)
		if icon != "" {
			if !store.ValidInventoryIcon(icon) {
				icon = ""
			} else {
				b, _ := base64.StdEncoding.DecodeString(icon)
				if iconBytes+len(b) > store.MaxInventoryIconsBytes {
					icon = ""
				} else {
					iconBytes += len(b)
				}
			}
		}
		v.Apps = append(v.Apps, store.InventoryApp{Icon: icon, Package: a.Package, Label: displayLabel(a.Label),
			System: a.System, Essential: a.Essential})
	}
	changed, err := s.Inventory.ApplyInventory(d.Profile, v)
	switch {
	case errors.Is(err, store.ErrInvalidID):
		return append(out, refuse("malformed")...)
	case err != nil:
		log.Printf("inventory from %s: %v", d.ID, err)
		return append(out, refuse("unavailable")...)
	}
	return append(out, field{"state", map[bool]string{true: "stored", false: "unchanged"}[changed]})
}

func strictUnmarshal(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// PackageManager labels are untrusted display text from the phone. Sanitize
// rather than reject the whole snapshot: Unicode tables differ across runtimes.
func displayLabel(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || !unicode.IsPrint(r) }), " ")
	if r := []rune(s); len(r) > 64 {
		s = strings.TrimSpace(string(r[:64]))
	}
	return s
}
