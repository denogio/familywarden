package openwrt

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/store"
)

// Native routing tests presence only. Even damaged state belongs to the core's
// validator; a missing target or non-regular entry must never select legacy UCI.
func (a Adapter) configurationOwned() (bool, error) {
	path := a.ConfigFile
	if path == "" {
		path = "/etc/familywarden/core/configuration.json"
	}
	floor := filepath.Join(filepath.Dir(path), configuration.OwnershipDocument)
	for _, name := range []string{path, floor, path + store.PresenceSuffix, floor + store.PresenceSuffix} {
		if _, err := os.Lstat(name); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
