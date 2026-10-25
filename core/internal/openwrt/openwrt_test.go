package openwrt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestAdapterCompilesConsistentContextAndPhonePolicy(t *testing.T) {
	c := platform.Context{AppFormat: appFormat, TimeZone: "UTC", Profiles: map[string]platform.Profile{
		"alex": {Name: "Alex", AppsRev: "ignore-shell-revision", Requires: []string{"ignore-shell-features"},
			AppInput: &rules.AppInput{Rules: "app|com.example.video|10|0\nnew|1||"}},
		"sam": {Name: "Sam", AppInput: &rules.AppInput{}},
	}}
	ctx, _ := json.Marshal(c)
	pol, _ := json.Marshal(platform.Policy{Context: c, Captain: "alex", Pact: json.RawMessage(`{"active":true}`), Apps: json.RawMessage(`{"untrusted":"legacy apps must not leak"}`)})
	path := filepath.Join(t.TempDir(), "adapter")
	// Assert the explicit format handshake and version on the real subprocess seam.
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"'core-context go-apps-v1') printf '%s' '" + string(ctx) + "';;\n" +
		"'core-policy alex 3 go-apps-v1'|'core-policy alex 2 go-apps-v1') printf '%s' '" + string(pol) + "';;\n" +
		"*) exit 1;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a := Adapter{Shell: path}
	got, err := a.Context()
	if err != nil || got.Profiles["alex"].AppsRev == "" || got.Profiles["alex"].Requires[0] != "apps-v1" || got.Profiles["sam"].AppsRev != "" {
		t.Fatal(got, err)
	}
	p, err := a.Policy("alex", 3)
	if err != nil {
		t.Fatal(err)
	}
	var apps rules.AppPolicy
	if err := json.Unmarshal(p.Apps, &apps); err != nil || apps.Rev != got.Profiles["alex"].AppsRev || apps.Limits[0].Minutes != 10 {
		t.Fatal(string(p.Apps), err)
	}
	if p.Captain != "alex" || string(p.Pact) != `{"active":true}` {
		t.Fatal("pact changed", p)
	}
	p, err = a.Policy("alex", 2)
	if err != nil || len(p.Apps) != 0 || p.Context.Profiles["alex"].AppsRev != apps.Rev {
		t.Fatal("v2 must get requirements but never an app policy", p, err)
	}
}

func TestAdapterRefusesIncompleteOrIncompatibleAppInputs(t *testing.T) {
	for _, raw := range []string{
		`{"profiles":{}}`,
		`{"app_format":"future","profiles":{}}`,
		`{"app_format":"go-apps-v1","profiles":{"alex":{"name":"Alex"}}}`,
		`{"app_format":"go-apps-v1","profiles":{"alex":{"app_input":{"rules":"new|2||"}}}}`,
	} {
		var c platform.Context
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		if err := compileContext(&c); err == nil {
			t.Fatal("accepted incomplete context", raw)
		}
	}
}
