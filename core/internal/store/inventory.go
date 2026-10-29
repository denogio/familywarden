package store

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Inventory is one enrolled phone key's latest installed-app snapshot. It is an
// observation for parents, never authority: nothing is enforced or approved from it,
// and writing it does not advance the household generation.
type Inventory struct {
	Version   int            `json:"version"`
	Device    string         `json:"device"` // storage key: device id + key fingerprint
	Name      string         `json:"name,omitempty"`
	Seq       int64          `json:"seq"`
	Installed int            `json:"installed"` // apps the phone counted; > len(Apps) = partial
	Apps      []InventoryApp `json:"apps"`
	Received  int64          `json:"received"`
}

type InventoryApp struct {
	Package   string `json:"package"`
	IconSince int64  `json:"icon_since,omitempty"`
	Label     string `json:"label,omitempty"`
	Icon      string `json:"icon,omitempty"`
	System    bool   `json:"system,omitempty"`
	Essential bool   `json:"essential,omitempty"`
}

const (
	MaxInventoryIconBytes  = 2048
	MaxInventoryIconPixels = 48
	MaxInventoryIconsBytes = 16 << 10
	InventoryVersion       = 1
	MaxInventoryApps       = 512
	maxInventoryCount      = 4096
)

// Complete is false when the phone could not send every app it counted.
func (v Inventory) Complete() bool { return v.Installed == len(v.Apps) }

func (v Inventory) valid() bool {
	if v.Version != InventoryVersion || !validDevKey.MatchString(v.Device) || v.Seq < 1 || v.Received < 1 ||
		len(v.Apps) > MaxInventoryApps || v.Installed < len(v.Apps) || v.Installed > maxInventoryCount ||
		!utf8.ValidString(v.Name) || len([]rune(v.Name)) > 64 {
		return false
	}
	iconBytes := 0
	for i, a := range v.Apps {
		if a.Icon != "" {
			if !ValidInventoryIcon(a.Icon) {
				return false
			}
			b, _ := base64.StdEncoding.DecodeString(a.Icon)
			iconBytes += len(b)
		}
		// Strictly sorted: unique packages, one canonical form per snapshot.
		if a.IconSince < 0 || a.IconSince > v.Seq || !validPkg.MatchString(a.Package) || len(a.Package) > 255 || (i > 0 && v.Apps[i-1].Package >= a.Package) ||
			!utf8.ValidString(a.Label) || len([]rune(a.Label)) > 64 {
			return false
		}
	}
	return iconBytes <= MaxInventoryIconsBytes
}

func (s *Store) inventoryPath(profile, device string) (string, error) {
	if !validID.MatchString(profile) || !validDevKey.MatchString(device) {
		return "", ErrInvalidID
	}
	return filepath.Join(s.dir, "inventory", profile, device+".json"), nil
}

// ApplyInventory keeps only a higher seq for this phone key. The same device id
// under another key or profile is removed, so re-keying or reassignment cannot
// accumulate snapshots. Identical content is not rewritten except for freshness.
func (s *Store) ApplyInventory(profile string, v Inventory) (changed bool, err error) {
	if !v.valid() {
		return false, ErrInvalidID
	}
	path, err := s.inventoryPath(profile, v.Device)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var prev Inventory
	if err := s.readRecorded(path, &prev); err == nil {
		if v.Seq <= prev.Seq {
			return false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	// Preserve continuity only while a package stays installed in this scope.
	previous := map[string]int64{}
	for _, a := range prev.Apps {
		previous[a.Package] = a.IconSince
		if previous[a.Package] == 0 {
			previous[a.Package] = prev.Seq
		}
	}
	for i := range v.Apps {
		v.Apps[i].IconSince = previous[v.Apps[i].Package]
		if v.Apps[i].IconSince == 0 {
			v.Apps[i].IconSince = v.Seq
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := s.writeRecorded(path, v); err != nil {
		return false, err
	}
	id := v.Device[:strings.LastIndexByte(v.Device, '@')]
	others, err := filepath.Glob(filepath.Join(s.dir, "inventory", "*", id+"@*.json"))
	if err != nil {
		return true, err
	}
	for _, other := range others {
		if other != path {
			if err := s.removeRecordedLocked(other); err != nil {
				return true, err
			}
		}
	}
	// Remove obsolete cache files only after metadata is durably committed.
	// Continuity tokens also invalidate them if cleanup is interrupted.
	icons, err := filepath.Glob(filepath.Join(s.dir, "inventory-icons", "*", id+"@*", "*.icon"))
	if err != nil {
		return true, err
	}
	keep := map[string]bool{}
	for _, a := range v.Apps {
		p, _ := s.iconPath(profile, v.Device, a.Package)
		keep[p] = true
	}
	for _, p := range icons {
		if !keep[p] {
			if err := s.removeRecordedLocked(p); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

// Inventories returns every stored snapshot for a profile, keyed by storage key.
// Callers filter by the current registry: a stored key is not current authority.
func (s *Store) Inventories(profile string) (map[string]Inventory, error) {
	if !validID.MatchString(profile) {
		return nil, ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, s.blocked
	}
	files, err := filepath.Glob(filepath.Join(s.dir, "inventory", profile, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]Inventory{}
	for _, f := range files {
		var v Inventory
		if err := s.readRecorded(f, &v); err != nil {
			return nil, err
		}
		if !v.valid() || filepath.Base(f) != v.Device+".json" {
			return nil, errors.New("invalid inventory document")
		}
		out[v.Device] = v
	}
	return out, nil
}

// Witness first, then data (as retention pruning does): a power cut cannot leave
// a witnessed-but-deleted document that blocks startup.
func (s *Store) removeRecordedLocked(path string) error {
	if s.blocked != nil {
		return s.blocked
	}
	for _, p := range []string{path + PresenceSuffix, path} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := syncDir(filepath.Dir(p)); err != nil {
			return err
		}
	}
	return nil
}

// ValidInventoryIcon bounds decoding before inspecting untrusted PNG dimensions and pixels.
// Only canonical base64 of a complete, small PNG is accepted, never URLs or SVG.
func ValidInventoryIcon(icon string) bool {
	return validPNG(icon, MaxInventoryIconBytes, MaxInventoryIconPixels)
}

const (
	MaxAppIconBytes  = 40 << 10
	MaxAppIconPixels = 96
)

func ValidAppIcon(icon string) bool { return validPNG(icon, MaxAppIconBytes, MaxAppIconPixels) }

func validPNG(icon string, maxBytes, maxPixels int) bool {
	if len(icon) > base64.StdEncoding.EncodedLen(maxBytes) {
		return false
	}
	b, err := base64.StdEncoding.Strict().DecodeString(icon)
	if err != nil || len(b) == 0 || len(b) > maxBytes || base64.StdEncoding.EncodeToString(b) != icon {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxPixels || cfg.Height > maxPixels {
		return false
	}
	r := bytes.NewReader(b)
	_, err = png.Decode(r)
	return err == nil && r.Len() == 0
}
