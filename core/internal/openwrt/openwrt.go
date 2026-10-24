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
	"github.com/denogio/familywarden/core/internal/usign"
)

type Adapter struct {
	Shell   string // the familywarden plugin, e.g. /usr/libexec/rpcd/familywarden
	KeyFile string // usign secret key, e.g. /etc/familywarden/mast.key
}

const maxOutput = 1 << 20

func (a Adapter) run(v any, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, a.Shell, args...)
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

func (a Adapter) Context() (platform.Context, error) {
	var c platform.Context
	err := a.run(&c, "core-context")
	return c, err
}

func (a Adapter) Policy(profile string, version int) (platform.Policy, error) {
	var p platform.Policy
	err := a.run(&p, "core-policy", profile, strconv.Itoa(version))
	return p, err
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

func (a Adapter) RemoveLegacyDevices() error {
	var v struct{}
	return a.run(&v, "core-legacy-devices-remove")
}
