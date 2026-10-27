package openwrt

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
)

func (a Adapter) dnsBase() string {
	if a.KeyFile == "" {
		return "/etc/familywarden"
	}
	return filepath.Dir(a.KeyFile)
}

func (a Adapter) withDNSLock(fn func() error) error {
	path := a.DNSLockFile
	if path == "" {
		path = "/var/lock/familywarden.dns.lock"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := acquireLock(ctx, lock, false); err != nil {
		return err
	}
	return fn()
}

// Only import compatibility data and access secrets here. The owned core never
// projects acknowledgements back into the shell's legacy files.
func (a Adapter) DNSTransaction(fn func(platform.DNSRuntime) error) error {
	return a.withDNSLock(func() error {
		read := func(name string) (string, error) {
			raw, err := os.ReadFile(filepath.Join(a.dnsBase(), name))
			if os.IsNotExist(err) {
				return "", nil
			}
			if err != nil {
				return "", err
			}
			if len(raw) > 1<<20 {
				return "", fmt.Errorf("oversized native DNS data")
			}
			return strings.TrimSpace(string(raw)), nil
		}
		auth, err := read("agh.auth")
		if err != nil {
			return err
		}
		owned, err := read("agh.owned")
		if err != nil {
			return err
		}
		acks, err := read("dns.ack")
		if err != nil {
			return err
		}
		lines := func(s string) []string {
			if s == "" {
				return []string{}
			}
			return strings.Split(s, "\n")
		}
		return fn(platform.DNSRuntime{Auth: auth, LegacyOwned: lines(owned), LegacyAcks: lines(acks)})
	})
}

func (a Adapter) SetDNSCredentials(username, password string) error {
	if username == "" || password == "" || strings.ContainsAny(username, ":\x00\r\n") || strings.ContainsAny(password, "\x00\r\n") {
		return fmt.Errorf("invalid credentials")
	}
	return a.withDNSLock(func() error {
		if err := os.MkdirAll(a.dnsBase(), 0700); err != nil {
			return err
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		return replaceFile(filepath.Join(a.dnsBase(), "agh.auth"), []byte(encoded+"\n"))
	})
}
