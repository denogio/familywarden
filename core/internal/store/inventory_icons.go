package store

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// AppIcon is a bounded observation tied to one uninterrupted installation.
// Separate files keep each durable write/read bounded even for 512 noisy PNGs.
type AppIcon struct {
	Version int    `json:"version"`
	Device  string `json:"device"`
	Package string `json:"package"`
	Since   int64  `json:"since"`
	Seq     int64  `json:"seq"`
	Icon    string `json:"icon"`
}

func (s *Store) iconPath(profile, device, pkg string) (string, error) {
	if _, err := s.inventoryPath(profile, device); err != nil {
		return "", err
	}
	if !validPkg.MatchString(pkg) || len(pkg) > 255 {
		return "", ErrInvalidID
	}
	return filepath.Join(s.dir, "inventory-icons", profile, device, fmt.Sprintf("%x.icon", sha256.Sum256([]byte(pkg)))), nil
}

func inventorySince(v Inventory, pkg string) int64 {
	for _, a := range v.Apps {
		if a.Package == pkg {
			if a.IconSince > 0 {
				return a.IconSince
			}
			return v.Seq
		}
	}
	return 0
}

// ApplyAppIcon requires the exact current snapshot; delayed batches cannot revive
// an uninstalled app. Replays never replace a newer image. Even identical images
// persist a newer seq: otherwise restart could admit an intermediate-seq replay.
func (s *Store) ApplyAppIcon(profile, device string, inventorySeq, seq int64, pkg, icon string) (bool, error) {
	path, err := s.iconPath(profile, device, pkg)
	if err != nil || seq < 1 || !ValidAppIcon(icon) {
		return false, ErrInvalidID
	}
	invPath, _ := s.inventoryPath(profile, device)
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Inventory
	if err = s.readRecorded(invPath, &v); err != nil {
		return false, err
	}
	since := inventorySince(v, pkg)
	if v.Seq != inventorySeq || since == 0 {
		return false, ErrInvalidID
	}
	var prev AppIcon
	if err = readJSON(path, &prev); err == nil {
		if prev.Version == 1 && prev.Device == device && prev.Package == pkg && ValidAppIcon(prev.Icon) && prev.Since == since && prev.Seq >= seq {
			return false, nil
		}
	} // Missing or corrupt cache is regenerable; metadata remains authoritative.
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	if err := writeJSON(path, AppIcon{1, device, pkg, since, seq, icon}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) AppIcons(profile, device string, packages []string) (map[string]string, error) {
	path, err := s.inventoryPath(profile, device)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Inventory
	if err = s.readRecorded(path, &v); err != nil {
		return nil, err
	}
	out := map[string]string{}
	requested := map[string]bool{}
	for _, pkg := range packages {
		requested[pkg] = true
	}
	for _, a := range v.Apps {
		if packages != nil && !requested[a.Package] {
			continue
		}

		p, _ := s.iconPath(profile, device, a.Package)
		var icon AppIcon
		if err = readJSON(p, &icon); err != nil {
			continue
		}
		if icon.Version != 1 || icon.Device != device || icon.Package != a.Package || !ValidAppIcon(icon.Icon) {
			continue
		}
		if icon.Since == inventorySince(v, a.Package) {
			out[a.Package] = icon.Icon
		}
	}
	return out, nil
}

// InvalidateInventory removes regenerable observations before a binding/key is
// changed or revoked, so returning to an earlier scope cannot revive old icons.
func (s *Store) InvalidateInventory(device string) error {
	if !validDevKey.MatchString(device + "@0000000000000000") {
		return ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	paths, err := filepath.Glob(filepath.Join(s.dir, "inventory", "*", device+"@*.json"))
	if err != nil {
		return err
	}
	icons, err := filepath.Glob(filepath.Join(s.dir, "inventory-icons", "*", device+"@*", "*.icon"))
	if err != nil {
		return err
	}
	for _, path := range append(paths, icons...) {
		if err := s.removeRecordedLocked(path); err != nil {
			return err
		}
	}
	return nil
}
