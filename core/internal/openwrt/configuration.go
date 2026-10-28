package openwrt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

const pactNetworkFormat = "pact-network-v1"

// ConfigurationTransaction retains ticket/state descriptors across import,
// durable core writes and the stateless projection/apply command.
func (a Adapter) ConfigurationTransaction(fn func(rules.Configuration, platform.Context, map[string]platform.NetworkOverride, func(rules.Configuration, platform.NetworkIntent) error) error) error {
	// ponytail: the OpenWrt projection uses its procd layout. Refuse custom
	// data paths rather than let a second ownership marker silently diverge.
	if a.ConfigFile != "" && a.ConfigFile != "/etc/familywarden/core/configuration.json" {
		return fmt.Errorf("configuration ownership requires the default OpenWrt data layout")
	}
	return a.withStateLocks(func(state, ticket *os.File) error {
		var current rules.Configuration
		owned, err := a.configurationOwned()
		if err != nil {
			return err
		}
		if !owned {
			if err := a.runLocked(&current, state, "core-configuration", "configuration-v1", "held-v1"); err != nil {
				return err
			}
		}
		var facts platform.Context
		if err := a.runLocked(&facts, state, "core-configuration-facts"); err != nil {
			return err
		}
		if facts.PactNetworkFormat != pactNetworkFormat || facts.NetworkIntentFormat != platform.NetworkIntentFormat {
			return fmt.Errorf("incompatible pact network adapter: upgrade core and backend together")
		}
		overrides, err := a.stateOverrides()
		if err != nil {
			return err
		}
		apply := func(c rules.Configuration, intent platform.NetworkIntent) error {
			undo, err := a.projectPactPermit(c, time.Now())
			if err != nil {
				return err
			}
			err = func() error {
				if err := a.materialize(c, facts.TimeZone); err != nil {
					return err
				}
				if err := a.stageIntent(intent); err != nil {
					return err
				}
				var reply struct {
					Status   string `json:"status"`
					IntentID string `json:"intent_id"`
				}
				if err := a.runFiles(&reply, []*os.File{state, ticket}, "core-apply-configuration", pactNetworkFormat); err != nil {
					return err
				}
				if reply.Status != "ok" || reply.IntentID != intent.ID {
					return fmt.Errorf("configuration apply not acknowledged")
				}
				return nil
			}()
			if err != nil && undo != nil {
				if restore := undo(); restore != nil {
					return fmt.Errorf("configuration apply failed (%v); permit restore failed: %w", err, restore)
				}
			}
			return err
		}
		return fn(current, facts, overrides, apply)
	})
}

// stateOverrides reads the adapter's own volatile RAM under the held state
// lock. It is platform access, not a product decision: the core computes the
// intent from this raw map.
func (a Adapter) stateOverrides() (map[string]platform.NetworkOverride, error) {
	raw, absent, err := readNetworkFile(a.networkStateFile())
	if err != nil {
		return nil, err
	}
	if absent {
		return map[string]platform.NetworkOverride{}, nil
	}
	return readAllOverrides(raw)
}

// stageIntent atomically publishes the core's renderer-ready intent next to
// the materialized projection. The owned native apply renders it; a missing
// or foreign-format file fails the compile closed.
func (a Adapter) stageIntent(intent platform.NetworkIntent) error {
	if intent.Format != platform.NetworkIntentFormat {
		return fmt.Errorf("invalid network intent format %q", intent.Format)
	}
	data, err := json.MarshalIndent(&intent, "", "  ")
	if err != nil {
		return err
	}
	native, err := renderNetworkIntent(intent, time.Now())
	if err != nil {
		return err
	}
	if err := replaceFile(a.networkIntentFile(), append(data, '\n')); err != nil {
		return err
	}
	return replaceFile(a.networkIntentFile()+".nft", native)
}

func q(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func renderConfiguration(d rules.ConfigurationData, derived bool) []byte {
	var b bytes.Buffer
	section := func(kind, id string) { fmt.Fprintf(&b, "config %s %s\n", kind, q(id)) }
	option := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "\toption %s %s\n", key, q(value))
		}
	}
	list := func(key string, values []string) {
		for _, v := range values {
			fmt.Fprintf(&b, "\tlist %s %s\n", key, q(v))
		}
	}
	section("global", "settings")
	for _, kv := range [][2]string{{"enabled", d.Settings.Enabled}, {"default_policy", d.Settings.DefaultPolicy}, {"ulysses_profile", d.Settings.Captain}, {"ulysses_delay", d.Settings.DelayHours}, {"dns_domain", d.Settings.DNSDomain}, {"agh_url", d.Settings.ResolverURL}} {
		option(kv[0], kv[1])
	}
	used := map[string]bool{"settings": true}
	for _, p := range d.Profiles {
		used[p.ID] = true
	}
	serial := 0
	id := func(original string) string {
		if !derived {
			return original
		}
		for {
			serial++
			v := fmt.Sprintf("fw_derived_%d", serial)
			if !used[v] {
				used[v] = true
				return v
			}
		}
	}
	for _, p := range d.Profiles {
		section("profile", p.ID)
		for _, kv := range [][2]string{{"name", p.Name}, {"enabled", p.Enabled}, {"policy", p.Policy}, {"block_router", p.BlockRouter}, {"dns", p.DNS}, {"dns_client_id", p.ClientID}, {"apps", p.Apps}, {"apps_suspend_new", p.SuspendNew}, {"screen_minutes", p.ScreenMinutes}, {"screen_weekday", p.ScreenWeekday}, {"screen_weekend", p.ScreenWeekend}} {
			option(kv[0], kv[1])
		}
		list("device", p.Devices)
		list("dns_protection", p.Protections)
		list("dns_service", p.Services)
		list("always_allowed", p.AlwaysAllowed)
	}
	for _, w := range d.Schedules {
		section("schedule", id(w.ID))
		option("profile", w.Profile)
		option("days", w.Days)
		option("start", w.Start)
		option("stop", w.Stop)
	}
	for _, v := range d.Apps {
		section("app", id(v.ID))
		option("profile", v.Profile)
		option("package", v.Package)
		option("minutes", v.Minutes)
		option("blocked", v.Blocked)
	}
	for _, w := range d.Downtime {
		section("app_downtime", id(w.ID))
		option("profile", w.Profile)
		option("days", w.Days)
		option("start", w.Start)
		option("stop", w.Stop)
	}
	for _, p := range d.LegacyPhones {
		section("device", id("legacy_"+p.ID))
		option("id", p.ID)
		option("name", p.Name)
		option("profile", p.Profile)
		option("pubkey", p.PublicKey)
		option("administrator", p.Administrator)
		option("approves", p.Approves)
	}
	return b.Bytes()
}

func replaceFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".configuration-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (a Adapter) materialize(c rules.Configuration, zone string) error {
	base := filepath.Dir(a.KeyFile)
	if base == "." {
		return fmt.Errorf("router key path required for configuration projection")
	}
	// Preserve untouched legacy files once; the core document is the authority,
	// while these files are merely the OpenWrt adapter's materialized projection.
	backup := filepath.Join(base, "core", "migration-backup")
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		staged, err := os.MkdirTemp(filepath.Dir(backup), ".migration-backup-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(staged)
		for name, path := range map[string]string{"familywarden": "/etc/config/familywarden", "pact": filepath.Join(base, "pact"), "pact.snap": filepath.Join(base, "pact.snap"), "pact.dns": filepath.Join(base, "pact.dns"), "release": filepath.Join(base, "release")} {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if err := replaceFile(filepath.Join(staged, name), data); err != nil {
				return err
			}
		}
		if err := os.Rename(staged, backup); err != nil {
			return err
		}
	}
	if err := replaceFile("/etc/config/familywarden", renderConfiguration(c.Current, false)); err != nil {
		return err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	if err := replaceFile("/var/run/familywarden/familywarden", renderConfiguration(configuration.Effective(c, time.Now().In(loc)), true)); err != nil {
		return err
	}
	if c.Pact == nil {
		for _, name := range []string{"pact", "pact.snap", "pact.dns", "release"} {
			if err := os.Remove(filepath.Join(base, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	} else {
		p := c.Pact
		meta := fmt.Sprintf("captain=%s\ntype=%s\nuntil=%s\nwindow_day=%s\nwindow_start=%s\nwindow_stop=%s\ndelay=%s\ndefault_policy=%s\n", p.Captain, p.Type, p.Until, p.WindowDay, p.WindowStart, p.WindowStop, p.DelaySeconds, p.DefaultPolicy)
		dns := fmt.Sprintf("dns=%s\nid=%s\ndomain=%s\n", p.DNS.Enabled, p.DNS.ClientID, p.DNS.Domain)
		for _, v := range p.DNS.Protections {
			dns += "prot=" + v + "\n"
		}
		for _, v := range p.DNS.Services {
			dns += "svc=" + v + "\n"
		}
		for name, data := range map[string][]byte{"pact": []byte(meta), "pact.snap": renderConfiguration(p.Frozen, false), "pact.dns": []byte(dns)} {
			if err := replaceFile(filepath.Join(base, name), data); err != nil {
				return err
			}
		}
	}
	if c.Pact != nil {
		if c.Pact.ReleaseAt == "" || c.Pact.ReleaseAt == "0" {
			if err := os.Remove(filepath.Join(base, "release")); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if err := replaceFile(filepath.Join(base, "release"), []byte(c.Pact.ReleaseAt+"\n")); err != nil {
			return err
		}
	}
	// Owned DNS reconciliation detects the desired-state fingerprint itself.
	return nil
}
