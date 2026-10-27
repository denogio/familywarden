package openwrt

import (
	"os"
	"strings"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
)

func TestNetworkApplyRejectsReceiptForDifferentIntent(t *testing.T) {
	a := networkAdapter(t)
	script, err := os.ReadFile(a.Shell)
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), `"$id" ;;`, `"wrong-intent" ;;`, 1))
	if err := os.WriteFile(a.Shell, script, 0700); err != nil {
		t.Fatal(err)
	}
	i := platform.NetworkIntent{Format: platform.NetworkIntentFormat, ID: strings.Repeat("a", 64)}
	err = a.NetworkTransaction("alex", func(_ platform.Context, _ map[string]platform.NetworkOverride, apply func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
		return apply(platform.NetworkOverride{Mode: "block"}, i, i)
	})
	if err == nil || !strings.Contains(err.Error(), "not acknowledged") {
		t.Fatal("foreign apply receipt accepted", err)
	}
	if _, err := os.Stat(a.StateFile); !os.IsNotExist(err) {
		t.Fatal("RAM not restored", err)
	}
}

func TestNetworkTransactionRejectsOldIntentBridgeBeforeCallback(t *testing.T) {
	a := networkAdapter(t)
	script, err := os.ReadFile(a.Shell)
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), `"network_intent_format":"network-intent-v1",`, "", 1))
	if err := os.WriteFile(a.Shell, script, 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	err = a.NetworkTransaction("alex", func(platform.Context, map[string]platform.NetworkOverride, func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatal("old bridge reached mutation", called, err)
	}
}
