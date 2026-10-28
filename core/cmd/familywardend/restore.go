package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/identity"
	"github.com/denogio/familywarden/core/internal/openwrt"
	"github.com/denogio/familywarden/core/internal/store"
	"github.com/denogio/familywarden/core/internal/usign"
)

func trustedKey(pubFile string) (usign.PublicKey, error) {
	info, err := os.Lstat(pubFile)
	if err != nil {
		return usign.PublicKey{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024 {
		return usign.PublicKey{}, errors.New("invalid trusted public key file")
	}
	raw, err := os.ReadFile(pubFile)
	if err != nil {
		return usign.PublicKey{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "untrusted comment:") {
		return usign.PublicKey{}, errors.New("trusted usign public key file required")
	}
	return usign.ParsePublicKey(lines[1])
}

// restoreBackup admits only a complete, current-schema, core-owned household and
// only onto a cleared router. It preflights the staged copy read-only before
// activation and never starts the daemon or applies policy itself.
func restoreBackup(ad openwrt.Adapter, dir, archive, pubFile string, out io.Writer) error {
	key, err := trustedKey(pubFile)
	if err != nil {
		return err
	}
	writer, err := acquireWriter(dir)
	if err != nil {
		return err
	}
	defer writer.Close()
	m, previous, err := ad.Restore(archive, key, func(core string) error {
		st, err := store.Open(core)
		if err != nil {
			return err
		}
		if err := st.CheckPresence(); err != nil {
			return err
		}
		owned, err := configuration.CheckStorage(st)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("legacy household backups cannot be restored")
		}
		if err := st.CheckGeneration(); err != nil {
			return err
		}
		if err := identity.CheckStorage(st, true); err != nil {
			return err
		}
		// A restored household is a new epoch whose fork point freezes how new
		// it is: phones that saw later state refuse it however many writes follow.
		return st.Fork()
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]any{"status": "restored", "format": m.Format,
		"created": m.Created, "signer": m.Signer, "previous": previous,
		"next": "start familywarden-core; it reapplies policy and DNS must reconcile"})
}
