package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/usign"
)

// fakeAdapter is the second consumer of platform.Adapter (A18): no OpenWrt here.
type fakeAdapter struct {
	ctx        platform.Context
	ctxErr     error
	legacy     []platform.LegacyDevice
	removed    bool
	removeErr  error
	key        usign.SecretKey
	policyErr  error
	lastPolicy string
	ticketMu   sync.Mutex
	tickets    []string
	ticketOK   bool
}

func (f *fakeAdapter) Context() (platform.Context, error) { return f.ctx, f.ctxErr }
func (f *fakeAdapter) Policy(profile string, version int) (platform.Policy, error) {
	f.lastPolicy = profile
	captain := ""
	if profile == "adult" {
		captain = "adult"
	}
	return platform.Policy{Context: f.ctx, Captain: captain,
		Pact:  json.RawMessage(`{"active":false,"type":"","until":0,"release_at":0}`),
		Rules: json.RawMessage(`{"enabled":"0","policy":"allow","override":"none","windows":[]}`),
		Apps:  json.RawMessage(`{"rev":"0123456789abcdef"}`)}, f.policyErr
}
func (f *fakeAdapter) RouterKey() (usign.SecretKey, error)             { return f.key, nil }
func (f *fakeAdapter) LegacyDevices() ([]platform.LegacyDevice, error) { return f.legacy, nil }
func (f *fakeAdapter) EnsureTicket(profile string, until int64) (bool, error) {
	f.ticketMu.Lock()
	defer f.ticketMu.Unlock()
	f.tickets = append(f.tickets, fmt.Sprintf("%s@%d", profile, until))
	return f.ticketOK, nil
}
func (f *fakeAdapter) RemoveLegacyDevices() error { f.removed = true; return f.removeErr }

type memStore struct {
	docs    map[string][]byte
	failing bool
}

func (m *memStore) ReadDocument(name string, v any) (bool, error) {
	b, ok := m.docs[name]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, v)
}
func (m *memStore) WriteDocument(name string, v any) error {
	if m.failing {
		return errors.New("disk full")
	}
	b, err := json.Marshal(v)
	m.docs[name] = b
	return err
}

type phone struct {
	pub  string
	priv ed25519.PrivateKey
	id   [8]byte
}

func newPhone(t *testing.T) phone {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	p := phone{priv: priv, id: [8]byte{7}}
	p.pub = usign.Format(pub, p.id)
	return p
}

func (p phone) sign(msg string) string { return usign.Sign(p.priv, p.id, []byte(msg)) }

const nonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func esc(s string) string { return url.QueryEscape(s) }

// query builds a phone request; caps "" = a v3 phone without capability signatures.
func (p phone) query(dev string, v3 bool, applied, caps string) string {
	q := "device=" + dev + "&nonce=" + nonce + "&key=" + esc(p.pub) + "&sig=" + esc(p.sign("familywarden-sync-v2\n"+dev+"\n"+nonce+"\n")) + "&name=Pixel"
	if v3 {
		q += "&sig3=" + esc(p.sign("familywarden-sync-v3\n"+dev+"\n"+nonce+"\n"+applied+"\n")) + "&applied=" + applied
		if caps != "" {
			q += "&caps=" + caps + "&sigcaps=" + esc(p.sign("familywarden-sync-caps-v1\n"+dev+"\n"+nonce+"\n"+applied+"\n"+caps+"\n"))
		}
	}
	return q
}

func routerKey(t *testing.T) usign.SecretKey {
	b, err := os.ReadFile("../usign/testdata/router.sec")
	if err != nil {
		t.Fatal(err)
	}
	k, err := usign.ParseSecretKey(b)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func setup(t *testing.T) (*Service, *fakeAdapter, *memStore) {
	ad := &fakeAdapter{key: routerKey(t), ctx: platform.Context{TimeZone: "Europe/Copenhagen", Profiles: map[string]platform.Profile{
		"kid":    {Name: "Kid", AppsRev: "0123456789abcdef", Requires: []string{"apps-v1", "screen-time-v1"}},
		"parent": {Name: "Parent"},
		"adult":  {Name: "Adult"},
	}}}
	st := &memStore{docs: map[string][]byte{}}
	s, err := New(ad, st)
	if err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Unix(1800000000, 0) }
	return s, ad, st
}

// payload verifies the router signature over the exact bytes and decodes them.
func reply(t *testing.T, s *Service, q string) map[string]any {
	t.Helper()
	code, body := s.Phone(q)
	if code != StatusOK {
		t.Fatalf("status %d", code)
	}
	sig, rest, ok := strings.Cut(string(body), "\n")
	pub, _ := usign.ParsePublicKey(routerKey(t).Public())
	if !ok || !pub.Verify(sig, []byte(rest)) {
		t.Fatal("reply not signed by the router key over its exact bytes")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(rest), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func enroll(t *testing.T, s *Service, p phone, dev, profile string, approves ...string) {
	t.Helper()
	s.Phone(p.query(dev, false, "", ""))
	a := strings.Join(approves, " ")
	if r := s.Approve(Change{Device: dev, Profile: profile, Fingerprint: fingerprint(p.pub), Approves: &a}); r["status"] != "ok" {
		t.Fatalf("approve %s: %v", dev, r)
	}
}

func TestUnknownPhoneIsPendingAndGetsNoPolicy(t *testing.T) {
	s, _, _ := setup(t)
	m := reply(t, s, newPhone(t).query("fw-new", true, "-", "apps-v1,screen-time-v1"))
	if m["enrollment"] != "pending" || m["binding"] != nil || m["apps"] != nil || m["nonce"] != nonce {
		t.Fatalf("pending reply: %v", m)
	}
	if got := s.Devices()["pending"].([]Reply); len(got) != 1 || got[0]["name"] != "Pixel" {
		t.Fatalf("pending list: %v", got)
	}
}

func TestSignaturesAreCheckedAndDomainSeparated(t *testing.T) {
	s, _, _ := setup(t)
	p := newPhone(t)
	enroll(t, s, p, "fw-a", "kid")
	other := newPhone(t)
	cases := map[string]struct {
		q    string
		want int
	}{
		"malformed nonce":     {strings.Replace(p.query("fw-a", false, "", ""), nonce, "xyz", 1), StatusMalformed},
		"v2 sig by other key": {strings.Replace(p.query("fw-a", false, "", ""), "sig="+esc(p.sign("familywarden-sync-v2\nfw-a\n"+nonce+"\n")), "sig="+esc(other.sign("familywarden-sync-v2\nfw-a\n"+nonce+"\n")), 1), StatusForbidden},
		"v3 sig over v2 msg":  {p.query("fw-a", false, "", "") + "&sig3=" + esc(p.sign("familywarden-sync-v2\nfw-a\n"+nonce+"\n")) + "&applied=-", StatusForbidden},
		"bad applied":         {p.query("fw-a", true, "-", "") + "x", StatusMalformed},
		"caps without v3":     {p.query("fw-a", false, "", "") + "&caps=apps-v1&sigcaps=" + esc(p.sign("x")), StatusMalformed},
		"unsorted caps":       {p.query("fw-a", true, "-", "screen-time-v1,apps-v1"), StatusMalformed},
		"duplicate caps":      {p.query("fw-a", true, "-", "apps-v1,apps-v1"), StatusMalformed},
		"unsigned caps":       {p.query("fw-a", true, "-", "") + "&caps=apps-v1&sigcaps=" + esc(p.sign("familywarden-sync-caps-v1\nfw-a\n"+nonce+"\n-\nother\n")), StatusForbidden},
	}
	for name, c := range cases {
		if code, _ := s.Phone(c.q); code != c.want {
			t.Errorf("%s: status %d, want %d", name, code, c.want)
		}
	}
}

func TestRawPlusIsNotASpace(t *testing.T) {
	if param("key=a+b%2Bc%3d&key=x", "key") != "a+b+c=" {
		t.Fatal("param must decode only the base64 escapes and keep '+'")
	}
}

func TestCapabilityRefusalIsSignedBoundAndClearsAck(t *testing.T) {
	s, _, _ := setup(t)
	p := newPhone(t)
	enroll(t, s, p, "fw-kid", "kid")
	m := reply(t, s, p.query("fw-kid", true, "0123456789abcdef", ""))
	if m["error"] != "unsupported_capabilities" || m["binding"] != nil || m["apps"] != nil || m["nonce"] != nonce {
		t.Fatalf("refusal: %v", m)
	}
	d := s.Devices()["devices"].([]Reply)[0]
	if d["apps_applied"] != "" || d["apps_error"] != "unsupported_capabilities" {
		t.Fatalf("incompatible phone acknowledged: %v", d)
	}
	// v2 cannot strip negotiation to obtain a weaker binding for an app-controlled profile.
	if m := reply(t, s, p.query("fw-kid", false, "", "")); m["binding"] != nil {
		t.Fatal("v2 binding for an app-controlled profile")
	}
	m = reply(t, s, p.query("fw-kid", true, "0123456789abcdef", "apps-v1,screen-time-v1"))
	if m["enrollment"] != "approved" || m["apps"] == nil || m["timezone"] != "Europe/Copenhagen" {
		t.Fatalf("compatible reply: %v", m)
	}
	if d := s.Devices()["devices"].([]Reply)[0]; d["apps_applied"] != "0123456789abcdef" || d["apps_error"] != "" {
		t.Fatalf("ack not recorded: %v", d)
	}
}

func TestApprovalNeedsTheComparedFingerprint(t *testing.T) {
	s, _, _ := setup(t)
	p := newPhone(t)
	s.Phone(p.query("fw-a", false, "", ""))
	if r := s.Approve(Change{Device: "fw-a", Profile: "kid", Fingerprint: "0000"}); r["error"] != "pending device changed; refresh and compare fingerprint" {
		t.Fatal(r)
	}
	if r := s.Approve(Change{Device: "fw-b", Profile: "kid"}); r["error"] != "no pending device" {
		t.Fatal(r)
	}
	if r := s.Approve(Change{Device: "fw-a", Profile: "nope", Fingerprint: fingerprint(p.pub)}); r["error"] != "profile not found" {
		t.Fatal(r)
	}
	// Same id, different key: stays pending and never gets the approved binding.
	enroll(t, s, p, "fw-a", "parent")
	if m := reply(t, s, newPhone(t).query("fw-a", false, "", "")); m["enrollment"] != "pending" {
		t.Fatalf("foreign key: %v", m)
	}
}

func TestPendingListIsBounded(t *testing.T) {
	s, _, _ := setup(t)
	p := newPhone(t)
	for i := 0; i < 15; i++ {
		s.Phone(p.query("fw-p"+string(rune('a'+i)), false, "", ""))
	}
	if n := len(s.Devices()["pending"].([]Reply)); n != maxPending {
		t.Fatalf("pending %d", n)
	}
}

func TestPactFreezesTheCaptainsPhones(t *testing.T) {
	s, ad, _ := setup(t)
	p, q := newPhone(t), newPhone(t)
	enroll(t, s, p, "fw-adult", "adult")
	enroll(t, s, q, "fw-parent", "parent", "adult", "kid")
	ad.ctx.PactCaptain = "adult"
	for name, r := range map[string]Reply{
		"move":    s.Update(Change{Device: "fw-adult", Profile: "parent"}),
		"delete":  s.Delete("fw-adult"),
		"removal": s.AllowRemoval("fw-adult"),
	} {
		if r["error"] != "bound by Ulysses pact" {
			t.Errorf("%s: %v", name, r)
		}
	}
	// Joining the captain only strengthens the pact.
	r := newPhone(t)
	enroll(t, s, r, "fw-join", "adult")
	// No parent keys for the captain, and its parent scope is suspended, not deleted.
	m := reply(t, s, p.query("fw-adult", true, "-", "apps-v1,screen-time-v1"))
	if parents := m["parents"].([]any); len(parents) != 0 {
		t.Fatalf("captain trusts parents: %v", parents)
	}
	d := find(s, "fw-parent")
	if d["approves"] != "adult kid" || d["approves_effective"] != "kid" {
		t.Fatalf("scope during pact: %v", d)
	}
	none := "kid"
	if r := s.Update(Change{Device: "fw-parent", Approves: &none}); r["status"] != "ok" {
		t.Fatalf("reducing a scope: %v", r)
	}
	again := "adult kid"
	if r := s.Update(Change{Device: "fw-parent", Approves: &again}); r["error"] != "bound by Ulysses pact" {
		t.Fatalf("new power over the captain: %v", r)
	}
	// Pact state unknown: refuse changes (fail closed).
	ad.ctxErr = errors.New("shell down")
	if r := s.Delete("fw-parent"); r["error"] == nil {
		t.Fatal("mutation without pact state")
	}
}

func find(s *Service, id string) Reply {
	for _, d := range s.Devices()["devices"].([]Reply) {
		if d["id"] == id {
			return d
		}
	}
	return nil
}

func TestParentScopeAndKeys(t *testing.T) {
	s, _, _ := setup(t)
	parent, kid := newPhone(t), newPhone(t)
	if r := s.Update(Change{Device: "fw-x"}); r["error"] != "invalid profile" {
		t.Fatalf("update of unknown: %v", r)
	}
	enroll(t, s, parent, "fw-parent", "parent", "kid", "kid")
	enroll(t, s, kid, "fw-kid", "kid")
	own := "parent"
	if r := s.Update(Change{Device: "fw-parent", Approves: &own}); r["error"] != "a phone cannot approve for its own profile" {
		t.Fatal(r)
	}
	m := reply(t, s, kid.query("fw-kid", true, "-", "apps-v1,screen-time-v1"))
	parents := m["parents"].([]any)
	if len(parents) != 1 || parents[0].(map[string]any)["pubkey"] != parent.pub {
		t.Fatalf("parents: %v", parents)
	}
	m = reply(t, s, parent.query("fw-parent", true, "-", "apps-v1,screen-time-v1"))
	scope := m["binding"].(map[string]any)["approves"].([]any)
	if len(scope) != 1 || scope[0].(map[string]any)["profile_id"] != "kid" || m["binding"].(map[string]any)["administrator"] != false {
		t.Fatalf("scope: %v", m["binding"])
	}
	if m := reply(t, s, parent.query("fw-parent", false, "", "")); m["binding"].(map[string]any)["approves"] != nil || m["parents"] != nil {
		t.Fatal("v2 reply carries v3 role fields")
	}
	// Unrelated update keeps the scope.
	if r := s.Update(Change{Device: "fw-parent", Name: "Robin"}); r["status"] != "ok" || find(s, "fw-parent")["approves"] != "kid" {
		t.Fatalf("scope lost: %v", find(s, "fw-parent"))
	}
}

func TestRemovalWindow(t *testing.T) {
	s, _, _ := setup(t)
	p := newPhone(t)
	enroll(t, s, p, "fw-a", "parent")
	if m := reply(t, s, p.query("fw-a", false, "", "")); m["binding"].(map[string]any)["can_self_remove"] != false {
		t.Fatal("managed phone may remove itself")
	}
	s.AllowRemoval("fw-a")
	if m := reply(t, s, p.query("fw-a", false, "", "")); m["binding"].(map[string]any)["can_self_remove"] != true {
		t.Fatal("removal not allowed within the hour")
	}
	s.Now = func() time.Time { return time.Unix(1800000000+3601, 0) }
	if m := reply(t, s, p.query("fw-a", false, "", "")); m["binding"].(map[string]any)["can_self_remove"] != false {
		t.Fatal("removal window did not lapse")
	}
}

func TestImportIsVerifiedOnceAndFailsClosed(t *testing.T) {
	p := newPhone(t)
	ad := &fakeAdapter{key: routerKey(t), legacy: []platform.LegacyDevice{
		{ID: "fw-old", Name: "Old", Profile: "kid", PubKey: p.pub, Administrator: "1", Approves: "kid parent"},
		{ID: "bad id", Profile: "kid", PubKey: p.pub},
		{ID: "fw-badkey", Profile: "kid", PubKey: "x"},
	}}
	if _, err := New(ad, &memStore{docs: map[string][]byte{}, failing: true}); err == nil || ad.removed {
		t.Fatal("failed import must not start and must not remove the legacy copy")
	}
	st := &memStore{docs: map[string][]byte{}}
	s, err := New(ad, st)
	if err != nil || !ad.removed {
		t.Fatalf("import: %v removed=%v", err, ad.removed)
	}
	if len(s.devices) != 1 || !s.devices[0].Administrator || strings.Join(s.devices[0].Approves, " ") != "parent" {
		t.Fatalf("imported %+v", s.devices)
	}
	ad.removed = false
	ad.legacy = append(ad.legacy, platform.LegacyDevice{ID: "fw-late", Profile: "kid", PubKey: p.pub})
	s, err = New(ad, st)
	if err != nil || len(s.devices) != 1 || ad.removed {
		t.Fatal("import ran twice")
	}
	// A removal failure after a verified import is harmless and not fatal.
	ad2 := &fakeAdapter{key: routerKey(t), removeErr: errors.New("uci busy")}
	if _, err := New(ad2, &memStore{docs: map[string][]byte{}}); err != nil {
		t.Fatal(err)
	}
	// An unknown schema is refused rather than misread.
	st.docs[devicesFile] = []byte(`{"version":2,"devices":[]}`)
	if _, err := New(ad, st); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestSaveFailureKeepsTheRegistry(t *testing.T) {
	s, _, st := setup(t)
	p := newPhone(t)
	s.Phone(p.query("fw-a", false, "", ""))
	st.failing = true
	if r := s.Approve(Change{Device: "fw-a", Profile: "kid", Fingerprint: fingerprint(p.pub)}); r["error"] != "saving device failed" {
		t.Fatal(r)
	}
	if len(s.devices) != 0 || len(s.Devices()["pending"].([]Reply)) != 1 {
		t.Fatal("failed save changed the registry or dropped the pending phone")
	}
}

func TestPolicyErrorIsNotAnUnsignedReply(t *testing.T) {
	s, ad, _ := setup(t)
	p := newPhone(t)
	enroll(t, s, p, "fw-a", "parent")
	ad.policyErr = errors.New("shell down")
	if code, body := s.Phone(p.query("fw-a", true, "-", "apps-v1,screen-time-v1")); code != StatusError || body != nil {
		t.Fatalf("got %d %q", code, body)
	}
}
