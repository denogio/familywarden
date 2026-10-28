package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/identity"
	"github.com/denogio/familywarden/core/internal/openwrt"
	"github.com/denogio/familywarden/core/internal/store"
	"github.com/denogio/familywarden/core/internal/usign"
)

func exportBackup(ad openwrt.Adapter, dir, dst, coreVersion string) (m backup.Manifest, err error) {
	writer, err := acquireWriter(dir)
	if err != nil {
		return m, err
	}
	defer writer.Close()
	err = ad.Backup(func(source backup.Source) error {
		actual, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		expected, err := filepath.EvalSymlinks(filepath.Join(source.Roots["household"], "core"))
		if err != nil {
			return err
		}
		actual, _ = filepath.Abs(actual)
		expected, _ = filepath.Abs(expected)
		if actual != expected {
			return errors.New("backup requires the native household/core data layout")
		}
		st, err := store.Open(dir)
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
		if err := identity.CheckStorage(st, owned); err != nil {
			return err
		}
		// No constructor, adoption, legacy import or native apply: export must not
		// repair, confirm, expire or otherwise mutate the household being copied.
		source.Versions["core"] = coreVersion
		m, err = backup.Create(dst, source)
		return err
	})
	return m, err
}

func inspectBackup(file, pubFile string, out io.Writer) error {
	info, err := os.Lstat(pubFile)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024 {
		return errors.New("invalid trusted public key file")
	}
	raw, err := os.ReadFile(pubFile)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "untrusted comment:") {
		return errors.New("trusted usign public key file required")
	}
	key, err := usign.ParsePublicKey(lines[1])
	if err != nil {
		return err
	}
	m, err := backup.Inspect(file, key)
	if err != nil {
		return err
	}
	return backupSummary(out, m)
}

func backupSummary(out io.Writer, m backup.Manifest) error {
	// Never print file contents, paths, names, DNS identities or credentials.
	return json.NewEncoder(out).Encode(map[string]any{"status": "ok", "format": m.Format,
		"platform": m.Platform, "created": m.Created, "signer": m.Signer, "files": len(m.Files),
		"restore_authorized": false})
}
