package configuration

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

type memory struct {
	data []byte
	dns  []byte
	fail bool
}

func (s *memory) ReadDocument(name string, v any) (bool, error) {
	if name == dnsDocument {
		if len(s.dns) == 0 {
			return false, nil
		}
		return true, json.Unmarshal(s.dns, v)
	}
	if len(s.data) == 0 {
		return false, nil
	}
	return true, json.Unmarshal(s.data, v)
}
func (s *memory) WriteDocument(name string, v any) error {
	if s.fail {
		return errors.New("disk full")
	}
	b, e := json.Marshal(v)
	if name == dnsDocument {
		s.dns = b
		return e
	}
	s.data = b
	return e
}

type fakeBackend struct {
	platform.Adapter
	current        rules.Configuration
	fail           int
	applies        int
	network        map[string]platform.NetworkOverride
	networkApplies int
	networkFail    bool
	mastSync       int64
	dnsRuntime     platform.DNSRuntime
}

func (f *fakeBackend) DNSTransaction(fn func(platform.DNSRuntime) error) error {
	return fn(f.dnsRuntime)
}
func (f *fakeBackend) SetDNSCredentials(username, password string) error {
	f.dnsRuntime.Auth = username + ":" + password
	return nil
}

func (f *fakeBackend) Configuration() (rules.Configuration, error) { return clone(f.current), nil }
func (f *fakeBackend) Context() (platform.Context, error) {
	p := map[string]platform.Profile{}
	for _, v := range f.current.Current.Profiles {
		p[v.ID] = platform.Profile{Name: v.Name, AppInput: &rules.AppConfig{Version: 1}}
	}
	return platform.Context{Profiles: p, TimeZone: "UTC", BonusFormat: bonusFormat, NetworkFormat: networkFormat, MastLastSync: f.mastSync}, nil
}

// Facts is the owned-mode read: no policy content, only facts. The fake's
// volatile network map doubles as the raw manual-override fact.
func (f *fakeBackend) Facts() (platform.Context, error) {
	p := map[string]platform.Profile{}
	for _, v := range f.current.Current.Profiles {
		row := platform.Profile{Override: "none"}
		if s, ok := f.network[v.ID]; ok {
			row.Override = s.Mode
		}
		p[v.ID] = row
	}
	captain := ""
	if c := f.current.Current.Settings.Captain; c != "" {
		captain = c
	}
	return platform.Context{RuntimeFormat: "runtime-facts-v1", Profiles: p, TimeZone: "UTC",
		BonusFormat: bonusFormat, NetworkFormat: networkFormat, MastCaptain: captain}, nil
}
func (f *fakeBackend) FactsContext(fn func(platform.Context) error) error {
	c, _ := f.Facts()
	return fn(c)
}
func (f *fakeBackend) WithContext(fn func(platform.Context) error) error {
	c, _ := f.Context()
	return fn(c)
}
func (f *fakeBackend) ConfigurationTransaction(fn func(rules.Configuration, platform.Context, map[string]platform.NetworkOverride, func(rules.Configuration, platform.NetworkIntent) error) error) error {
	c, _ := f.Context()
	return fn(clone(f.current), c, f.networkMap(), func(next rules.Configuration, _ platform.NetworkIntent) error {
		f.applies++
		if f.fail > 0 {
			f.fail--
			return errors.New("apply failed")
		}
		f.current = clone(next)
		return nil
	})
}
func (f *fakeBackend) networkMap() map[string]platform.NetworkOverride {
	m := map[string]platform.NetworkOverride{}
	for id, s := range f.network {
		m[id] = s
	}
	return m
}
func (f *fakeBackend) NetworkTransaction(id string, fn func(platform.Context, map[string]platform.NetworkOverride, func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error) error {
	c, _ := f.Context()
	m := f.networkMap()
	return fn(c, m, func(next platform.NetworkOverride, _, _ platform.NetworkIntent) error {
		f.networkApplies++
		if f.networkFail {
			return errors.New("network apply failed")
		}
		if f.network == nil {
			f.network = map[string]platform.NetworkOverride{}
		}
		f.network[id] = next
		return nil
	})
}

func (f *fakeBackend) NetworkSnapshot(fn func(platform.Context, map[string]platform.NetworkOverride) error) error {
	c, _ := f.Context()
	state := map[string]platform.NetworkOverride{}
	for id, s := range f.network {
		state[id] = s
	}
	return fn(c, state)
}

func sample() rules.Configuration {
	return rules.Configuration{Version: 1, Current: rules.ConfigurationData{Settings: rules.ConfigurationSettings{Enabled: "1", Captain: "alex", DefaultPolicy: "block", DelayHours: "24"}, Profiles: []rules.ConfigurationProfile{{ID: "alex", Name: "Alex", Enabled: "1", Policy: "scheduled", Devices: []string{"aa:bb:cc:00:00:01"}}, {ID: "taylor", Name: "Taylor", Enabled: "0", ClientID: "fw-preserved"}}, Schedules: []rules.ConfigurationWindow{{ID: "cfg123456", Profile: "alex", Days: "mon", Start: "07:00", Stop: "20:00"}}}}
}
func setup(t *testing.T) (*Authority, *fakeBackend, *memory) {
	t.Helper()
	b := &fakeBackend{current: sample()}
	_ = b.current.Seal()
	s := &memory{}
	a, e := New(b, b, s)
	if e != nil {
		t.Fatal(e)
	}
	a.Now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	t.Cleanup(func() {
		if a.timer != nil {
			a.timer.Stop()
		}
	})
	return a, b, s
}
func request(v any) []byte { b, _ := json.Marshal(v); return b }
func okResult(t *testing.T, r map[string]any) {
	t.Helper()
	if r["error"] != nil {
		t.Fatal(r)
	}
}
func activate(t *testing.T, a *Authority, b *fakeBackend) {
	t.Helper()
	okResult(t, a.API("configuration_import", request(map[string]any{"revision": b.current.Revision})))
}
func candidate(a *Authority, name string) []byte {
	d, _, _ := a.load()
	next := clone(d.Configuration)
	next.Current.Profiles[1].Name = name
	return request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current})
}

func TestExplicitImportCASConfirmAndNoReimport(t *testing.T) {
	a, b, s := setup(t)
	if a.API("configuration_export", nil)["owner"] != "legacy" || len(s.data) > 0 {
		t.Fatal("implicit activation")
	}
	if a.API("configuration_import", request(map[string]any{"revision": "stale"}))["error"] == nil {
		t.Fatal("stale import accepted")
	}
	activate(t, a, b)
	old, _, _ := a.load()
	r := a.API("configuration_apply", candidate(a, "Robin"))
	okResult(t, r)
	if r["status"] != "confirmation_required" {
		t.Fatal(r)
	}
	if a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": "wrong"}))["error"] == nil {
		t.Fatal("bad confirmation accepted")
	}
	okResult(t, a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": r["token"]})))
	if a.API("configuration_apply", request(map[string]any{"revision": old.Configuration.Revision, "current": old.Configuration.Current}))["error"] == nil {
		t.Fatal("stale edit accepted")
	}
	b.current = sample() // legacy edits must not re-import over the durable owner
	okResult(t, a.API("configuration_import", []byte(`{}`)))
	d, _, _ := a.load()
	if d.Configuration.Current.Profiles[1].Name != "Robin" {
		t.Fatal("re-import replaced core edits")
	}
}

func TestPendingRollsBackOnDeadlineAndRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "restart"}[restart], func(t *testing.T) {
			a, b, s := setup(t)
			activate(t, a, b)
			okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
			if restart {
				var e error
				a, e = New(b, b, s)
				if e != nil {
					t.Fatal(e)
				}
			} else {
				a.Now = func() time.Time { return time.Unix(1800000200, 0).UTC() }
				if _, e := a.Context(); e != nil {
					t.Fatal(e)
				}
			}
			d, _, e := a.load()
			if e != nil || d.Pending != nil || d.Configuration.Current.Profiles[1].Name != "Taylor" || b.current.Current.Profiles[1].Name != "Taylor" {
				t.Fatal("rollback failed", d, e)
			}
		})
	}
}

func TestFailedApplyRestoresPreviousStateAndFailsClosedUntilRecovery(t *testing.T) {
	a, b, s := setup(t)
	activate(t, a, b)
	b.fail = 1
	if a.API("configuration_apply", candidate(a, "Robin"))["error"] == nil {
		t.Fatal("failed apply accepted")
	}
	if b.current.Current.Profiles[1].Name != "Taylor" {
		t.Fatal("old projection not restored")
	}
	if _, e := a.Context(); e == nil {
		t.Fatal("uncertain transaction served policy")
	}
	recovered, e := New(b, b, s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := recovered.Context(); e != nil {
		t.Fatal(e)
	}
}

func TestFutureOrCorruptDocumentNeverFallsBack(t *testing.T) {
	a, b, s := setup(t)
	activate(t, a, b)
	for _, data := range []string{`{"version":99}`, `not-json`, `{"version":1,"version":99}`} {
		s.data = []byte(data)
		if _, e := a.Context(); e == nil {
			t.Fatal("corrupt store served legacy state", data)
		}
		if _, e := New(b, b, s); e == nil {
			t.Fatal("corrupt store accepted on restart", data)
		}
	}
}

func TestPactAndReleaseStateShareConfigurationOwner(t *testing.T) {
	a, b, s := setup(t)
	activate(t, a, b)
	okResult(t, a.API("configuration_pact", []byte(`{"until":1800003600}`)))
	d, _, _ := a.load()
	if d.Configuration.Pact == nil || d.Configuration.Pact.Frozen.Profiles[0].ID != "alex" {
		t.Fatal(d)
	}
	next := clone(d.Configuration)
	next.Current.Profiles[0].Devices = nil
	if a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))["error"] == nil {
		t.Fatal("frozen edit accepted")
	}
	okResult(t, a.API("configuration_sail_request", []byte(`{}`)))
	d, _, _ = a.load()
	deadline := d.Configuration.Pact.ReleaseAt
	if integer(deadline) != 1800000000+86400 {
		t.Fatal("waiting period changed", deadline)
	}
	recovered, e := New(b, b, s)
	if e != nil {
		t.Fatal(e)
	}
	d, _, _ = recovered.load()
	if d.Configuration.Pact.ReleaseAt != deadline {
		t.Fatal("release lost across restart")
	}
}
