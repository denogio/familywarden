// Package openwrt implements platform.Adapter on OpenWrt. It is the only core
// package that knows OpenWrt: interim, it asks the shell plugin for what the shell
// still owns (profiles, pacts, policy content; plans/core-migration.md step 1).
package openwrt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
	"github.com/denogio/familywarden/core/internal/usign"
)

type Adapter struct {
	Shell      string // the familywarden plugin, e.g. /usr/libexec/rpcd/familywarden
	KeyFile    string // usign secret key, e.g. /etc/familywarden/mast.key
	LockFile   string // interim state transaction lock; empty uses the OpenWrt default
	ConfigFile string // core household document (OpenWrt currently uses the default layout)
}

const maxOutput = 1 << 20

func (a Adapter) run(v any, args ...string) error {
	return a.runLocked(v, nil, args...)
}

func (a Adapter) runLocked(v any, lock *os.File, args ...string) error {
	var files []*os.File
	if lock != nil {
		files = []*os.File{lock}
	}
	return a.runFiles(v, files, args...)
}

func (a Adapter) runFiles(v any, files []*os.File, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, a.Shell, args...)
	cmd.WaitDelay = time.Second // bound pipe draining if a timed-out shell leaves children
	cmd.ExtraFiles = files      // fd 3 = state lock; configuration apply also inherits ticket fd 4
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", a.Shell, args, err, bytes.TrimSpace(errb.Bytes()))
	}
	if out.Len() > maxOutput {
		return fmt.Errorf("%s %v: output too large", a.Shell, args)
	}
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(out.Bytes(), &e) == nil && e.Error != "" {
		return fmt.Errorf("%s %v: %s", a.Shell, args, e.Error)
	}
	return json.Unmarshal(out.Bytes(), v)
}

const appFormat = "go-apps-v3"

// compileContext is shared by policy and administrator/request paths, so they
// agree on revisions and requirements even before a phone fetches its policy.
func compileContext(c *platform.Context) error {
	if c.AppFormat != appFormat {
		return fmt.Errorf("incompatible policy adapter format %q", c.AppFormat)
	}
	for id, prof := range c.Profiles {
		if prof.AppInput == nil {
			return fmt.Errorf("missing app input for %s", id)
		}
		apps, requires, err := prof.AppInput.Compile()
		if err != nil {
			return fmt.Errorf("apps of %s: %w", id, err)
		}
		prof.AppsRev, prof.Requires = "", requires
		if apps != nil {
			prof.AppsRev = apps.Rev
		}
		c.Profiles[id] = prof
	}
	return nil
}

func (a Adapter) Configuration() (rules.Configuration, error) {
	var c rules.Configuration
	err := a.run(&c, "core-configuration", "configuration-v1")
	return c, err
}

func (a Adapter) Context() (platform.Context, error) {
	var c platform.Context
	if err := a.run(&c, "core-context", appFormat); err != nil {
		return c, err
	}
	if err := compileContext(&c); err != nil {
		return c, err
	}
	return c, nil
}

func (a Adapter) Policy(profile string, version int) (platform.Policy, error) {
	var p platform.Policy
	if err := a.run(&p, "core-policy", profile, strconv.Itoa(version), appFormat); err != nil {
		return p, err
	}
	if err := compileContext(&p.Context); err != nil {
		return p, err
	}
	p.Apps = nil // only the portable compiler may supply an app policy
	if prof, ok := p.Context.Profiles[profile]; ok && version == 3 {
		apps, _, err := prof.AppInput.Compile()
		if err != nil {
			return p, err
		}
		if apps != nil {
			p.Apps, err = json.Marshal(apps)
			if err != nil {
				return p, err
			}
		}
	}
	return p, nil
}

// RouterKey makes sure the shell created the key (first boot), then reads it.
func (a Adapter) RouterKey() (usign.SecretKey, error) {
	if _, err := os.Stat(a.KeyFile); err != nil {
		if out, err := exec.Command(a.Shell, "mast-pubkey").CombinedOutput(); err != nil {
			return usign.SecretKey{}, fmt.Errorf("creating router key: %v: %s", err, out)
		}
	}
	b, err := os.ReadFile(a.KeyFile)
	if err != nil {
		return usign.SecretKey{}, err
	}
	return usign.ParseSecretKey(b)
}

func (a Adapter) LegacyDevices() ([]platform.LegacyDevice, error) {
	var v struct {
		Devices []platform.LegacyDevice `json:"devices"`
	}
	err := a.run(&v, "core-legacy-devices")
	return v.Devices, err
}

func (a Adapter) EnsureTicket(profile string, until int64) (bool, error) {
	var v struct {
		Opened bool `json:"opened"`
	}
	err := a.run(&v, "core-ticket", profile, strconv.FormatInt(until, 10))
	return v.Opened, err
}

func (a Adapter) RemoveLegacyDevices() error {
	var v struct{}
	return a.run(&v, "core-legacy-devices-remove")
}
