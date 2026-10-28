package openwrt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/rules"
	"github.com/denogio/familywarden/core/internal/usign"
)

// Restore places one verified, core-owned household onto a cleared router. It
// never merges or overlays: any existing household evidence refuses the restore,
// because the router cannot prove which state is newer. UCI and pact files are
// not restored; core startup materializes them from the restored journal.
// The host must already hold the daemon writer lease for <base>/core.
func (a Adapter) Restore(archive string, trusted usign.PublicKey, admit func(coreDir string) error) (m backup.Manifest, previous string, err error) {
	err = a.withStateLocks(func(state, _ *os.File) error {
		return a.withDNSLock(func() error {
			base := a.dnsBase()
			if err := a.clearedTarget(base, state); err != nil {
				return err
			}
			parent := filepath.Dir(base)
			staging, err := os.MkdirTemp(parent, ".familywarden-restore-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(staging)
			if m, err = backup.Extract(archive, trusted, staging); err != nil {
				return err
			}
			if m.Platform != "openwrt" {
				return errors.New("backup is for another platform")
			}
			household := filepath.Join(staging, "household")
			for _, name := range []string{"core/authority.json", "core/configuration.json", "core/devices.json"} {
				if info, err := os.Lstat(filepath.Join(household, name)); err != nil || !info.Mode().IsRegular() {
					return errors.New("only complete core-owned household backups can be restored")
				}
			}
			// The archive signer must be the identity being restored, or phones
			// pinned to that key would not recognize the router afterward.
			secret, err := readBackupKey(filepath.Join(household, "mast.key"))
			if err != nil {
				return err
			}
			key, err := usign.ParseSecretKey(secret)
			if err != nil {
				return err
			}
			public, err := readBackupKey(filepath.Join(household, "mast.pub"))
			if err != nil {
				return err
			}
			lines := strings.Split(strings.TrimSpace(string(public)), "\n")
			if len(lines) != 2 || strings.TrimSpace(lines[1]) != key.Public() {
				return errors.New("restored signing key pair mismatch")
			}
			if pub, _ := usign.ParsePublicKey(key.Public()); pub.Fingerprint() != trusted.Fingerprint() {
				return errors.New("backup signer is not the restored router identity")
			}
			if err := admit(filepath.Join(household, "core")); err != nil {
				return err
			}
			previous = fmt.Sprintf("%s.pre-restore-%d", base, time.Now().UnixNano())
			if err := os.Rename(base, previous); err != nil {
				return err
			}
			if err := os.Rename(household, base); err != nil {
				if back := os.Rename(previous, base); back != nil {
					return fmt.Errorf("restore failed (%v) and the cleared household is at %s (%v)", err, previous, back)
				}
				previous = ""
				return err
			}
			return syncPath(parent)
		})
	})
	return m, previous, err
}

func syncPath(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// A cleared target has no household evidence: at most an unpaired fresh router
// key, the lifetime lock, an empty imported registry and empty usage directories,
// and a legacy projection without profiles, phones or pact.
func (a Adapter) clearedTarget(base string, state *os.File) error {
	info, err := os.Lstat(base)
	if err != nil {
		return err // the host's writer lease creates <base>/core first
	}
	if !info.IsDir() {
		return errors.New("household path is not a directory")
	}
	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		if d.Type()&fs.ModeSymlink != 0 {
			return errors.New("household contains links")
		}
		if d.IsDir() {
			if rel == "." || rel == "core" || rel == "core/usage" || strings.HasPrefix(rel, "core/usage/") {
				return nil
			}
			return fmt.Errorf("household is not cleared: %s", rel)
		}
		switch rel {
		case "mast.key", "mast.pub", "core/.writer.lock", "core/devices.json.present":
			return nil
		case "core/devices.json":
			var doc struct {
				Devices []json.RawMessage `json:"devices"`
			}
			data, err := os.ReadFile(path)
			if err != nil || json.Unmarshal(data, &doc) != nil || len(doc.Devices) != 0 {
				return errors.New("household is not cleared: approved phones exist")
			}
			return nil
		}
		return fmt.Errorf("household is not cleared: %s", rel)
	})
	if err != nil {
		return err
	}
	var legacy rules.Configuration
	if err := a.runLocked(&legacy, state, "core-configuration", "configuration-v1", "held-v1"); err != nil {
		return err
	}
	if len(legacy.Current.Profiles) != 0 || len(legacy.Current.LegacyPhones) != 0 || legacy.Pact != nil {
		return errors.New("household is not cleared: platform configuration has profiles, phones or a pact")
	}
	return nil
}
