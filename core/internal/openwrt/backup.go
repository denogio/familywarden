package openwrt

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/denogio/familywarden/core/internal/backup"
	"github.com/denogio/familywarden/core/internal/usign"
)

// Backup holds every cooperating native writer lock in the established order.
// The host must already hold the daemon writer lease: no core socket calls or
// policy application can be made from this callback. No keys are generated.
func (a Adapter) Backup(fn func(backup.Source) error) error {
	return a.withStateLocks(func(_, _ *os.File) error {
		return a.withDNSLock(func() error {
			root := a.dnsBase()
			keyFile := a.KeyFile
			if keyFile == "" {
				keyFile = filepath.Join(root, "mast.key")
			}
			secret, err := readBackupKey(keyFile)
			if err != nil {
				return err
			}
			key, err := usign.ParseSecretKey(secret)
			if err != nil {
				return err
			}
			public, err := readBackupKey(filepath.Join(root, "mast.pub"))
			if err != nil {
				return err
			}
			lines := strings.Split(strings.TrimSpace(string(public)), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "untrusted comment:") || strings.TrimSpace(lines[1]) != key.Public() {
				return errors.New("backup signing key pair mismatch")
			}
			var versions map[string]string
			if err := a.run(&versions, "call", "version"); err != nil {
				return err
			}
			for _, name := range []string{"backend", "frontend"} {
				if versions[name] == "" || versions[name] == "unknown" {
					return errors.New("installed package versions unavailable")
				}
			}
			delete(versions, "configuration_owner")
			config := a.BackupConfigFile
			if config == "" {
				config = "/etc/config/familywarden"
			}
			return fn(backup.Source{Platform: "openwrt", Versions: versions, Roots: map[string]string{
				"household": root, "platform-config": config,
			}, Exclude: []string{"household/core/.writer.lock"}, Key: key})
		})
	})
}

func readBackupKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024 {
		return nil, errors.New("invalid backup key file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(data) > 1024 {
		return nil, errors.New("invalid backup key file")
	}
	return data, nil
}
