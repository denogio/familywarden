package configuration

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

func TestPactNetworkOverrideRevokesOnlyBoundCaptainAllows(t *testing.T) {
	now := time.Unix(1000, 0)
	allow := platform.NetworkOverride{Mode: "allow"}
	for _, tc := range []struct {
		name, id, kind, until, release string
		current, want                  platform.NetworkOverride
		fail                           bool
	}{
		{"permanent captain allow", "alex", "ongoing", "0", "", allow, platform.NetworkOverride{Mode: "none"}, false},
		{"timed captain allow", "alex", "ongoing", "0", "", platform.NetworkOverride{Mode: "allow", Expires: 2000}, platform.NetworkOverride{Mode: "none"}, false},
		{"elapsed captain allow", "alex", "ongoing", "0", "", platform.NetworkOverride{Mode: "allow", Expires: 500}, platform.NetworkOverride{Mode: "none"}, false},
		{"block retained", "alex", "ongoing", "0", "", platform.NetworkOverride{Mode: "block"}, platform.NetworkOverride{Mode: "block"}, false},
		{"no override retained", "alex", "ongoing", "0", "", platform.NetworkOverride{Mode: "none"}, platform.NetworkOverride{Mode: "none"}, false},
		{"other profile retained", "taylor", "ongoing", "0", "", allow, allow, false},
		{"expired pact", "alex", "until", "1000", "", allow, allow, false},
		{"live until pact", "alex", "until", "1001", "", allow, platform.NetworkOverride{Mode: "none"}, false},
		{"released pact", "alex", "ongoing", "0", "1000", allow, allow, false},
		{"waiting for release", "alex", "ongoing", "0", "1001", allow, platform.NetworkOverride{Mode: "none"}, false},
		{"invalid bound state", "alex", "ongoing", "0", "", platform.NetworkOverride{Mode: "bad"}, platform.NetworkOverride{Mode: "bad"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sample()
			c.Current.Settings.Captain = "taylor" // The bound captain wins over ignored edits.
			c.Pact = &rules.ConfigurationPact{Captain: "alex", Type: tc.kind, Until: tc.until, ReleaseAt: tc.release}
			got, err := PactNetworkOverride(c, tc.id, tc.current, now)
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatal(got, err)
			}
		})
	}
	if got, err := PactNetworkOverride(sample(), "alex", allow, now); err != nil || got != allow {
		t.Fatal("configured captain without a pact lost permission", got, err)
	}
}

func TestNetworkDecisionPrecedenceAndDeadlines(t *testing.T) {
	none := platform.NetworkOverride{Mode: "none"}
	block := platform.NetworkOverride{Mode: "block"}
	allow := platform.NetworkOverride{Mode: "allow"}
	timed := platform.NetworkOverride{Mode: "allow", Expires: 1100}
	for _, tc := range []struct {
		name, action        string
		current, want       platform.NetworkOverride
		value               int64
		bound, opened, fail bool
	}{
		{"block replaces ticket", "block", timed, block, 0, false, true, false},
		{"reset removes block", "reset", block, none, 0, true, true, false},
		{"captain can tighten", "block", none, block, 0, true, true, false},
		{"captain cannot allow", "allow", block, block, 0, true, false, true},
		{"captain cannot ticket", "ticket", none, none, 1, true, false, true},
		{"ticket replaces explicit block", "ticket", block, platform.NetworkOverride{Mode: "allow", Expires: 1060}, 1, false, true, false},
		{"ticket replaces permanent allow", "ticket", allow, platform.NetworkOverride{Mode: "allow", Expires: 1060}, 1, false, true, false},
		{"ticket is additive", "ticket", timed, platform.NetworkOverride{Mode: "allow", Expires: 1160}, 1, false, true, false},
		{"elapsed ticket starts now", "ticket", platform.NetworkOverride{Mode: "allow", Expires: 1000}, platform.NetworkOverride{Mode: "allow", Expires: 1060}, 1, false, true, false},
		{"grant cannot replace block", "grant", block, block, 1200, false, false, false},
		{"grant cannot open pact", "grant", none, none, 1200, true, false, false},
		{"past grant opens nothing", "grant", none, none, 1000, false, false, false},
		{"grant keeps permanent allow", "grant", allow, allow, 1200, false, true, false},
		{"grant never shortens", "grant", timed, timed, 1050, false, true, false},
		{"grant extends to absolute deadline", "grant", timed, platform.NetworkOverride{Mode: "allow", Expires: 1200}, 1200, false, true, false},
		{"zero ticket refused", "ticket", none, none, 0, false, false, true},
		{"oversize ticket refused", "ticket", none, none, 2147483648, false, false, true},
		{"unknown state refused", "block", platform.NetworkOverride{Mode: "bad"}, platform.NetworkOverride{Mode: "bad"}, 0, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, opened, err := decideNetwork(tc.current, tc.action, 1000, tc.value, tc.bound)
			if got != tc.want || opened != tc.opened || (err != nil) != tc.fail {
				t.Fatal(got, opened, err)
			}
		})
	}
}

func TestNetworkTicketDeadlineOverflowIsRefused(t *testing.T) {
	if _, _, err := decideNetwork(platform.NetworkOverride{Mode: "none"}, "ticket", math.MaxInt64-1, 1, false); err == nil {
		t.Fatal("overflowing ticket deadline accepted")
	}
}

type oldNetworkBackend struct{ *fakeBackend }

func (b *oldNetworkBackend) NetworkTransaction(id string, fn func(platform.Context, map[string]platform.NetworkOverride, func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error) error {
	return b.fakeBackend.NetworkTransaction(id, func(c platform.Context, overrides map[string]platform.NetworkOverride, apply func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
		c.NetworkFormat = ""
		return fn(c, overrides, apply)
	})
}

func TestOwnedNetworkRefusesIncompatibleBackendBeforeApplying(t *testing.T) {
	_, f, s := setup(t)
	b := &oldNetworkBackend{f}
	a, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	activate(t, a, f)
	if r := a.API("configuration_block", []byte(`{"profile":"alex"}`)); r["error"] == nil || f.networkApplies != 0 {
		t.Fatal("incompatible adapter applied a network change", r)
	}
}

func TestOwnedNetworkActionsShareRAMWithGrantRepair(t *testing.T) {
	a, b, s := setup(t)
	activate(t, a, b)
	before := string(s.data)
	call := func(method, input string) map[string]any { return a.API("configuration_"+method, []byte(input)) }
	okResult(t, call("block", `{"profile":"alex"}`))
	if opened, err := a.EnsureTicket("alex", a.Now().Unix()+600); err != nil || opened {
		t.Fatal(opened, err)
	}
	okResult(t, call("reset", `{"profile":"alex"}`))
	first := call("ticket", `{"profile":"alex"}`) // Existing default: 30 minutes.
	okResult(t, first)
	deadline, err := time.Parse(time.RFC3339, first["expires"].(string))
	if err != nil || deadline.Unix() != a.Now().Unix()+1800 {
		t.Fatal(first, err)
	}
	second := call("ticket", `{"profile":"alex","minutes":"0005"}`)
	okResult(t, second)
	if b.network["alex"].Expires != deadline.Unix()+300 {
		t.Fatal(b.network)
	}
	if opened, err := a.EnsureTicket("alex", deadline.Unix()+100); err != nil || !opened || b.network["alex"].Expires != deadline.Unix()+300 {
		t.Fatal(opened, err, b.network)
	}
	if opened, err := a.EnsureTicket("alex", deadline.Unix()+500); err != nil || !opened || b.network["alex"].Expires != deadline.Unix()+500 {
		t.Fatal(opened, err, b.network)
	}
	applies := b.networkApplies
	if opened, err := a.EnsureTicket("alex", deadline.Unix()+500); err != nil || !opened || b.networkApplies != applies {
		t.Fatal("retry extended or reapplied the same grant", opened, err)
	}
	okResult(t, call("allow", `{"profile":"alex"}`))
	if opened, err := a.EnsureTicket("alex", deadline.Unix()+900); err != nil || !opened || b.network["alex"].Expires != 0 {
		t.Fatal(opened, err)
	}
	okResult(t, call("block", `{"profile":"taylor"}`)) // Legacy permits actions on disabled profiles.
	restarted, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = a.Now
	okResult(t, restarted.API("configuration_reset", []byte(`{"profile":"alex"}`)))
	if string(s.data) != before || b.network["taylor"].Mode != "block" {
		t.Fatal("network action changed durable config or unrelated RAM", b.network)
	}
}

func TestOwnedNetworkRefusalsAndApplyFailure(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	for _, input := range []string{
		`{"profile":"alex","minutes":0}`, `{"profile":"alex","minutes":-1}`,
		`{"profile":"alex","minutes":1.5}`, `{"profile":"alex","minutes":null}`,
		`{"profile":"alex","minutes":2147483648}`, `{"profile":"alex","minutes":"+1"}`,
		`{"profile":"alex","minutes":1,"minutes":2}`, `{"profile":"missing"}`,
		`{"profile":"alex","unknown":1}`, `{"profile":"../alex"}`,
	} {
		if r := a.API("configuration_ticket", []byte(input)); r["error"] == nil || b.networkApplies != 0 {
			t.Fatal(input, r)
		}
	}
	okResult(t, a.API("configuration_pact", []byte(`{"until":0,"window_day":"sun","window_start":"10:00","window_stop":"11:00"}`)))
	for _, action := range []string{"allow", "ticket"} {
		if r := a.API("configuration_"+action, []byte(`{"profile":"alex"}`)); r["error"] != "bound by Ulysses pact" || r["pact_until"] != int64(0) {
			t.Fatal(r)
		}
	}
	okResult(t, a.API("configuration_block", []byte(`{"profile":"alex"}`)))
	if opened, err := a.EnsureTicket("alex", a.Now().Unix()+600); err != nil || opened {
		t.Fatal(opened, err)
	}
	okResult(t, a.API("configuration_reset", []byte(`{"profile":"alex"}`)))
	okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
	if r := a.API("configuration_block", []byte(`{"profile":"taylor"}`)); r["error"] != "confirmation pending" {
		t.Fatal(r)
	}
	if a.timer != nil {
		a.timer.Stop()
	}
	// A separate household isolates a failed application from the pending edit.
	a, b, _ = setup(t)
	activate(t, a, b)
	b.networkFail = true
	if r := a.API("configuration_block", []byte(`{"profile":"alex"}`)); r["error"] == nil {
		t.Fatal(r)
	}
	if _, err := a.Context(); err == nil {
		t.Fatal("uncertain network application served a policy")
	}
}

func TestOwnedNetworkConcurrentTicketsDoNotLoseTime(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := a.API("configuration_ticket", []byte(`{"profile":"alex","minutes":1}`)); r["error"] != nil {
				t.Error(r)
			}
		}()
	}
	wg.Wait()
	if b.network["alex"].Expires != a.Now().Unix()+600 {
		t.Fatal(b.network)
	}
}

type legacyNetworkBackend struct {
	*fakeBackend
	called bool
}

func (b *legacyNetworkBackend) EnsureTicket(string, int64) (bool, error) {
	b.called = true
	return true, nil
}

func TestLegacyGrantBridgeStaysGatedByOwnership(t *testing.T) {
	_, f, s := setup(t)
	b := &legacyNetworkBackend{fakeBackend: f}
	a, err := New(b, b, s)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := a.EnsureTicket("alex", 1800000600); err != nil || !opened || !b.called {
		t.Fatal(opened, err)
	}
	activate(t, a, f)
	b.called = false
	a.Now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	if opened, err := a.EnsureTicket("alex", 1800000600); err != nil || !opened || b.called || !reflect.DeepEqual(b.network["alex"], platform.NetworkOverride{Mode: "allow", Expires: 1800000600}) {
		t.Fatal(opened, err, b.called, b.network)
	}
}
