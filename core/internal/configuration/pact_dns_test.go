package configuration

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/rules"
)

func dnsPact(t *testing.T) (*Authority, *fakeBackend, *memory) {
	t.Helper()
	a, b, st := setup(t)
	c := importedDivergence()
	c.Current.Settings.DNSDomain = "dns.example.net"
	c.Current.Settings.ResolverURL = "http://192.0.2.1:3000"
	p := profile(c.Current, "alex")
	p.DNS, p.ClientID = "1", "fwalex"
	p.Protections, p.Services = []string{"filter_lists"}, []string{"youtube"}
	snap := profile(c.Pact.Frozen, "alex")
	snap.DNS, snap.ClientID = "1", "fwalex"
	snap.Protections, snap.Services = []string{"filter_lists"}, []string{"youtube"}
	c.Pact.DNS = rules.ConfigurationDNS{Enabled: "1", ClientID: "fwalex", Domain: "dns.example.net", Protections: []string{"filter_lists"}, Services: []string{"youtube"}}
	_ = c.Seal()
	b.current = c
	activate(t, a, b)
	return a, b, st
}

func updatePactDNS(t *testing.T, a *Authority, change func(*rules.ConfigurationProfile)) map[string]any {
	t.Helper()
	d, _, err := a.load()
	if err != nil {
		t.Fatal(err)
	}
	next := clone(d.Configuration)
	change(profile(next.Current, "alex"))
	return a.API("configuration_dns_update", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))
}

func TestPactDNSChangesAccumulateWithoutPromotingOtherConfiguredRules(t *testing.T) {
	a, b, st := dnsPact(t)
	if InWindow(b.current.Pact, a.Now()) {
		t.Fatal("fixture must be outside the amendment window")
	}
	r := updatePactDNS(t, a, func(p *rules.ConfigurationProfile) {
		p.Protections = append(p.Protections, "safe_search")
		p.Services = append(p.Services, "tiktok")
	})
	okResult(t, r)
	d, _, _ := a.load()
	if d.Pending != nil || r["token"] != nil || d.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "10" || d.Configuration.Pact.Frozen.Schedules[0].Stop != "20:00" {
		t.Fatal("DNS edit armed rollback or changed frozen app/schedule", d)
	}
	okResult(t, updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.DNS = "0"; p.Protections = nil; p.Services = nil }))
	d, _, _ = a.load()
	snap := d.Configuration.Pact.DNS
	if snap.Enabled != "1" || snap.ClientID != "fwalex" || snap.Domain != "dns.example.net" || !slices.Contains(snap.Protections, "safe_search") || !slices.Contains(snap.Services, "tiktok") {
		t.Fatal("removal escaped a restriction added after pact creation", snap)
	}
	_, _, profiles := DNS(d.Configuration, a.Now())
	if len(profiles) != 1 || profiles[0].ID != "alex" || !slices.Contains(profiles[0].Protections, "safe_search") {
		t.Fatal("desired DNS ignored its accumulated snapshot", profiles)
	}
	recovered, err := New(b, b, st)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, _ := recovered.load()
	if !reflect.DeepEqual(restored.Configuration.Pact.DNS, snap) {
		t.Fatal("restart lost accumulated restrictions")
	}
	// A real explicit amendment inside the window can still loosen DNS.
	a.Now = func() time.Time { return time.Date(2027, 1, 3, 10, 30, 0, 0, time.UTC) }
	okResult(t, a.API("configuration_amend", request(map[string]any{"revision": d.Configuration.Revision, "current": d.Configuration.Current})))
	d, _, _ = a.load()
	if d.Configuration.Pact.DNS.Enabled != "0" {
		t.Fatal("explicit window amendment could not turn DNS off")
	}
}

func TestPactDNSRefusesOtherEditsAndIdentityRotation(t *testing.T) {
	for _, reason := range []string{"stale revision", "captain app", "other profile", "settings", "identity", "invalid service"} {
		t.Run(reason, func(t *testing.T) {
			a, b, st := dnsPact(t)
			d, _, _ := a.load()
			next := clone(d.Configuration)
			revision := d.Configuration.Revision
			p := profile(next.Current, "alex")
			p.Protections = append(p.Protections, "safe_search")
			switch reason {
			case "stale revision":
				revision = "stale"
			case "captain app":
				p.ScreenMinutes = "0"
			case "other profile":
				profile(next.Current, "taylor").Name = "Robin"
			case "settings":
				next.Current.Settings.DNSDomain = "other.example.net"
			case "identity":
				p.ClientID = "rotated"
			case "invalid service":
				p.Services = []string{"bad service"}
			}
			before := string(st.data)
			applies := b.applies
			if r := a.API("configuration_dns_update", request(map[string]any{"revision": revision, "current": next.Current})); r["error"] == nil {
				t.Fatal("forbidden edit accepted", reason)
			}
			if string(st.data) != before || b.applies != applies {
				t.Fatal("refused DNS update changed the store/platform", reason)
			}
		})
	}
}

func TestPactDNSAllocatesOnlyWhenEnablingAndRetainsItsIdentity(t *testing.T) {
	a, b, _ := setup(t)
	c := importedDivergence()
	c.Current.Settings.DNSDomain = "dns.example.net"
	c.Pact.DNS = rules.ConfigurationDNS{Enabled: "0", Domain: "dns.example.net"}
	_ = c.Seal()
	b.current = c
	activate(t, a, b)
	okResult(t, updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.DNS = "1" }))
	d, _, _ := a.load()
	id := profile(d.Configuration.Current, "alex").ClientID
	if len(id) != 34 || d.Configuration.Pact.DNS.ClientID != id || profile(d.Configuration.Pact.Frozen, "alex").ClientID != id {
		t.Fatal("new DNS identity not atomically frozen", d)
	}
	okResult(t, updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.Protections = []string{"parental"} }))
	d, _, _ = a.load()
	if profile(d.Configuration.Current, "alex").ClientID != id {
		t.Fatal("DNS retry rotated identity")
	}
}

func TestPactDNSRequiresActivePactAndNoPendingConfiguration(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	if r := updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.DNS = "0" }); r["error"] != "active pact required" {
		t.Fatal(r)
	}
	a, b, st := dnsPact(t)
	okResult(t, a.API("configuration_apply", candidate(a, "Robin")))
	before := string(st.data)
	applies := b.applies
	if r := updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.Protections = append(p.Protections, "parental") }); r["error"] != "confirmation pending" || string(st.data) != before || b.applies != applies {
		t.Fatal("pending change interleaved with DNS update", r)
	}
}

func TestFailedPactDNSApplyRestoresThePreviousSnapshot(t *testing.T) {
	a, b, st := dnsPact(t)
	d, _, _ := a.load()
	before := d.Configuration.Pact.DNS
	b.fail = 1
	if r := updatePactDNS(t, a, func(p *rules.ConfigurationProfile) { p.Protections = append(p.Protections, "parental") }); r["error"] == nil {
		t.Fatal("failed apply reported as committed")
	}
	if !reflect.DeepEqual(b.current.Pact.DNS, before) {
		t.Fatal("failed apply lost the prior restriction snapshot")
	}
	recovered, err := New(b, b, st)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, _ := recovered.load()
	if !reflect.DeepEqual(restored.Configuration.Pact.DNS, before) || restored.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "10" {
		t.Fatal("recovery changed the old DNS/app snapshot")
	}
}
