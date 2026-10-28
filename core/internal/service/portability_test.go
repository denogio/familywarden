package service

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/rules"
	"github.com/denogio/familywarden/core/internal/store"
	"github.com/denogio/familywarden/core/internal/usign"
)

// This is a recording/failing consumer of the real contracts, not a second
// firewall or product implementation. It has no OpenWrt imports or native paths.
// All policy, precedence, signatures and reconciliation run in service.New.
type recordingHost struct {
	mu                         sync.Mutex
	current                    rules.Configuration
	network                    map[string]platform.NetworkOverride
	key                        usign.SecretKey
	dns                        platform.DNSRuntime
	now                        func() time.Time
	owned                      bool
	legacyCalls                int
	failConfiguration, failNet int
	failFacts                  bool
	intents                    []platform.NetworkIntent
	restored                   []platform.NetworkIntent
}

var _ platform.Adapter = (*recordingHost)(nil)
var _ configuration.Backend = (*recordingHost)(nil)
var _ platform.DNSPlatform = (*recordingHost)(nil)

func copyJSON[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

func (h *recordingHost) legacy() error {
	h.legacyCalls++
	if h.owned {
		return errors.New("owned service called a legacy adapter method")
	}
	return nil
}
func (h *recordingHost) Context() (platform.Context, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.facts(), h.legacy()
}
func (h *recordingHost) Configuration() (rules.Configuration, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return copyJSON(h.current), h.legacy()
}
func (h *recordingHost) Policy(string, int) (platform.Policy, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return platform.Policy{}, h.legacy()
}
func (h *recordingHost) WithContext(fn func(platform.Context) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.legacy(); err != nil {
		return err
	}
	return fn(h.facts())
}
func (h *recordingHost) EnsureTicket(string, int64) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return false, h.legacy()
}
func (h *recordingHost) LegacyDevices() ([]platform.LegacyDevice, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return nil, h.legacy()
}
func (h *recordingHost) RemoveLegacyDevices() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.legacy()
}
func (h *recordingHost) RouterKey() (usign.SecretKey, error) { return h.key, nil }

// Facts report profile presence, native deadlines and the applied compatibility
// projection, never app configuration, schedule eligibility or DNS acknowledgements.
func (h *recordingHost) facts() platform.Context {
	c := platform.Context{RuntimeFormat: "runtime-facts-v1", BonusFormat: "bonus-state-v1", AuthorityFormat: configuration.OwnershipFormat,
		NetworkFormat: "network-actions-v1", NetworkIntentFormat: platform.NetworkIntentFormat,
		PactNetworkFormat: "pact-network-v1", TimeZone: "UTC", Profiles: map[string]platform.Profile{},
		MastCaptain: h.current.Current.Settings.Captain}
	if h.current.Pact != nil {
		c.PactCaptain = h.current.Pact.Captain
	}
	for _, p := range h.current.Current.Profiles {
		s := h.network[p.ID]
		mode := s.Mode
		if mode == "" || s.Expires > 0 && s.Expires <= h.now().Unix() {
			mode = "none"
		}
		c.Profiles[p.ID] = platform.Profile{Override: mode}
	}
	return c
}
func (h *recordingHost) Facts() (platform.Context, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failFacts {
		return platform.Context{}, errors.New("recording host facts unavailable")
	}
	return h.facts(), nil
}
func (h *recordingHost) FactsContext(fn func(platform.Context) error) error {
	return h.NetworkSnapshot(func(c platform.Context, _ map[string]platform.NetworkOverride) error { return fn(c) })
}
func (h *recordingHost) NetworkSnapshot(fn func(platform.Context, map[string]platform.NetworkOverride) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failFacts {
		return errors.New("recording host facts unavailable")
	}
	return fn(h.facts(), copyJSON(h.network))
}
func (h *recordingHost) ConfigurationTransaction(fn func(rules.Configuration, platform.Context, map[string]platform.NetworkOverride, func(rules.Configuration, platform.NetworkIntent) error) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failFacts {
		return errors.New("recording host facts unavailable")
	}
	return fn(copyJSON(h.current), h.facts(), copyJSON(h.network), func(c rules.Configuration, intent platform.NetworkIntent) error {
		h.intents = append(h.intents, copyJSON(intent))
		if h.failConfiguration > 0 {
			h.failConfiguration--
			return errors.New("recording host rejected configuration apply")
		}
		// The compatibility projection uses the same public core operation as
		// OpenWrt; the adapter does not reimplement pact-permit eligibility.
		for id, state := range h.network {
			next, err := configuration.PactNetworkOverride(c, id, state, h.now())
			if err != nil {
				return err
			}
			h.network[id] = next
		}
		h.current = copyJSON(c)
		return nil
	})
}
func (h *recordingHost) NetworkTransaction(id string, fn func(platform.Context, map[string]platform.NetworkOverride, func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failFacts {
		return errors.New("recording host facts unavailable")
	}
	return fn(h.facts(), copyJSON(h.network), func(next platform.NetworkOverride, intent, before platform.NetworkIntent) error {
		h.intents = append(h.intents, copyJSON(intent))
		if h.failNet > 0 {
			h.failNet--
			h.restored = append(h.restored, copyJSON(before))
			return errors.New("recording host rejected network apply")
		}
		h.network[id] = next
		return nil
	})
}
func (h *recordingHost) DNSTransaction(fn func(platform.DNSRuntime) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return fn(copyJSON(h.dns))
}
func (h *recordingHost) SetDNSCredentials(username, password string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dns.Auth = username + ":" + password
	return nil
}

// A real durable store and a controlled clock, shared by the production services.
// Native RAM and credentials remain outside it, so restart and reboot are distinct.
type portableFixture struct {
	t     *testing.T
	host  *recordingHost
	app   *Service
	dir   string
	mu    sync.Mutex
	clock time.Time
}

func portableKey(t *testing.T) usign.SecretKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return usign.SecretKey{KeyID: [8]byte{1}, Key: key}
}
func newPortableFixture(t *testing.T) *portableFixture {
	t.Helper()
	f := &portableFixture{t: t, dir: t.TempDir(), clock: time.Now().UTC().Truncate(time.Second)}
	f.host = &recordingHost{key: portableKey(t), now: f.now, network: map[string]platform.NetworkOverride{},
		current: rules.Configuration{Version: 1, Current: rules.ConfigurationData{
			Settings: rules.ConfigurationSettings{Enabled: "1", DefaultPolicy: "block", Captain: "jordan", DelayHours: "1"},
			Profiles: []rules.ConfigurationProfile{
				{ID: "alex", Name: "Alex", Enabled: "1", Policy: "scheduled", Apps: "1", ScreenMinutes: "40", Devices: []string{"aa:bb:cc:00:00:01"}},
				{ID: "robin", Name: "Robin", Enabled: "1", Policy: "allow"},
				{ID: "jordan", Name: "Jordan", Enabled: "1", Policy: "scheduled", BlockRouter: "1", Apps: "1", ScreenMinutes: "60", Devices: []string{"aa:bb:cc:00:00:02"}}},
			Schedules: []rules.ConfigurationWindow{
				{ID: "night", Profile: "alex", Days: "mon tue wed thu fri sat sun", Start: "22:00", Stop: "06:00"},
				{ID: "midnight", Profile: "alex", Days: "mon", Start: "23:00", Stop: "24:00"},
				{ID: "captain_night", Profile: "jordan", Days: "mon tue wed thu fri sat sun", Start: "22:00", Stop: "06:00"},
				{ID: "captain_midnight", Profile: "jordan", Days: "mon", Start: "23:00", Stop: "24:00"}},
			Apps: []rules.ConfigurationApp{{ID: "video", Profile: "alex", Package: "com.example.video", Minutes: "15"}},
		}}}
	if err := f.host.current.Seal(); err != nil {
		t.Fatal(err)
	}
	f.start()
	f.api("configuration_import", map[string]any{"revision": f.host.current.Revision})
	f.host.mu.Lock()
	f.host.owned, f.host.legacyCalls = true, 0
	f.host.mu.Unlock()
	t.Cleanup(func() {
		f.app.Close()
		f.host.mu.Lock()
		defer f.host.mu.Unlock()
		if f.host.legacyCalls != 0 {
			t.Errorf("owned service used legacy adapter methods %d times", f.host.legacyCalls)
		}
	})
	// A PATH with no tools proves the service cannot rely on native commands.
	t.Setenv("PATH", t.TempDir())
	return f
}
func (f *portableFixture) now() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.clock }
func (f *portableFixture) noon() {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.clock
	f.clock = time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, time.UTC)
}
func (f *portableFixture) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(d)
}
func (f *portableFixture) start() {
	st, err := store.Open(f.dir)
	if err != nil {
		f.t.Fatal(err)
	}
	f.app, err = New(f.host, f.host, st, "portable-test")
	if err != nil {
		f.t.Fatal(err)
	}
	f.app.Configuration.Now, f.app.Identity.Now = f.now, f.now
}
func (f *portableFixture) restart() { f.app.Close(); f.start() }
func (f *portableFixture) exchange(path string, body []byte) (int, []byte) {
	f.t.Helper()
	r := httptest.NewRecorder()
	f.app.Handler.ServeHTTP(r, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)))
	return r.Code, r.Body.Bytes()
}
func (f *portableFixture) call(path string, value any) map[string]any {
	f.t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		f.t.Fatal(err)
	}
	code, body := f.exchange(path, b)
	if code != http.StatusOK {
		f.t.Fatalf("%s: HTTP %d: %s", path, code, body)
	}
	var reply map[string]any
	if err := json.Unmarshal(body, &reply); err != nil {
		f.t.Fatal(path, err, string(body))
	}
	return reply
}
func (f *portableFixture) api(method string, value any) map[string]any {
	f.t.Helper()
	r := f.call("/api/"+method, value)
	if err, ok := r["error"]; ok && err != nil && err != "" {
		f.t.Fatal(method, r)
	}
	return r
}
func (f *portableFixture) configuration() rules.Configuration {
	f.t.Helper()
	r := f.api("configuration_export", nil)
	b, _ := json.Marshal(r["configuration"])
	var c rules.Configuration
	if err := json.Unmarshal(b, &c); err != nil || r["owner"] != "core" {
		f.t.Fatal(err, r)
	}
	return c
}
func (f *portableFixture) commit(c rules.Configuration) {
	f.t.Helper()
	r := f.api("configuration_apply", map[string]any{"revision": c.Revision, "current": c.Current})
	if r["status"] == "confirmation_required" {
		f.api("configuration_confirm", map[string]any{"revision": r["revision"], "token": r["token"]})
	}
}

type portablePhone struct {
	key usign.SecretKey
	id  string
}

func (f *portableFixture) phone(id, profile string, approves ...string) portablePhone {
	f.t.Helper()
	p := portablePhone{key: portableKey(f.t), id: id}
	f.sync(p)
	key, _ := usign.ParsePublicKey(p.key.Public())
	f.api("device_approve", map[string]any{"device": id, "profile": profile, "name": "Test phone", "fingerprint": key.Fingerprint(), "approves": strings.Join(approves, " ")})
	return p
}
func (f *portableFixture) signed(code int, body []byte) map[string]any {
	f.t.Helper()
	sig, raw, ok := strings.Cut(string(body), "\n")
	key, _ := usign.ParsePublicKey(f.host.key.Public())
	if code != http.StatusOK || !ok || !key.Verify(sig, []byte(raw)) {
		f.t.Fatalf("invalid signed reply: HTTP %d: %s", code, body)
	}
	var r map[string]any
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (p portablePhone) query() []byte {
	nonce, caps, applied := strings.Repeat("a", 64), "apps-v1,grants-v1,screen-time-v1", "-"
	q := url.Values{"device": {p.id}, "nonce": {nonce}, "key": {p.key.Public()}, "name": {"Test-phone"}, "applied": {applied}}
	q.Set("sig", p.key.Sign([]byte("familywarden-sync-v2\n"+p.id+"\n"+nonce+"\n")))
	q.Set("sig3", p.key.Sign([]byte("familywarden-sync-v3\n"+p.id+"\n"+nonce+"\n"+applied+"\n")))
	q.Set("sigcaps", p.key.Sign([]byte("familywarden-sync-caps-v1\n"+p.id+"\n"+nonce+"\n"+applied+"\n"+caps+"\n")))
	// The phone protocol intentionally decodes only base64 escapes, not CSV commas.
	return []byte(q.Encode() + "&caps=" + caps)
}
func (f *portableFixture) sync(p portablePhone) map[string]any {
	f.t.Helper()
	return f.signed(f.exchange("/phone", p.query()))
}
func (f *portableFixture) upload(p portablePhone, kind string, value any) map[string]any {
	f.t.Helper()
	b, _ := json.Marshal(value)
	body := []byte(p.key.Sign(protocol.Message(kind, b)) + "\n" + string(b))
	r := f.signed(f.exchange("/app/"+kind, body))
	hash := sha256.Sum256(b)
	if r["message_sha256"] != hex.EncodeToString(hash[:]) {
		f.t.Fatal("upload response not bound to its exact bytes", r)
	}
	return r
}

func objectField(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return m
}
func profileReport(f *portableFixture, id string) map[string]any {
	return objectField(f.t, objectField(f.t, f.api("configuration_report", nil)["profiles"])[id])
}

func TestPortableProductionPolicyActionsAndRestart(t *testing.T) {
	f := newPortableFixture(t)
	f.noon()
	if profileReport(f, "alex")["status"] != "blocked" {
		t.Fatal("overnight schedule was not default-deny at noon")
	}
	p := f.phone("fw-alex", "alex")
	f.host.current.Current.Profiles[0].Name = "Stale platform label"
	f.host.current.Current.Profiles[0].ScreenMinutes = "999"
	signed := f.sync(p)
	if objectField(t, signed["binding"])["profile_name"] != "Alex" || objectField(t, objectField(t, signed["apps"])["screen"])["mon"] != float64(40) {
		t.Fatal("owned policy depended on the compatibility projection", signed)
	}
	if f.host.intents[0].Profiles[0].Windows[1].Stop != "24:00" {
		t.Fatal("native adapter lost the core's midnight endpoint")
	}
	if signed["enrollment"] != "approved" || objectField(t, signed["apps"])["rev"] == "" {
		t.Fatal(signed)
	}
	if len(objectField(t, signed["rules"])["windows"].([]any)) != 0 {
		t.Fatal("ordinary child received captain hotspot rules")
	}
	captain := f.phone("fw-jordan", "jordan")
	captainPolicy := f.sync(captain)
	windows := objectField(t, captainPolicy["rules"])["windows"].([]any)
	if len(windows) != 2 || objectField(t, windows[0])["start"] != "22:00" || objectField(t, windows[0])["stop"] != "06:00" || objectField(t, windows[1])["stop"] != "00:00" {
		t.Fatal("owned phone schedule changed semantics", windows)
	}
	if strings.Contains(string(mustJSON(t, captainPolicy["rules"])), "24:00") {
		t.Fatal("phone got an unsupported midnight endpoint", signed)
	}
	before := f.configuration()
	f.api("configuration_bonus_add", map[string]any{"profile": "alex", "minutes": 15})
	if f.configuration().Revision != before.Revision || profileReport(f, "alex")["bonus_today"] != float64(15) {
		t.Fatal("runtime bonus changed configuration or was not reported")
	}
	apps := objectField(t, f.sync(p)["apps"])
	if objectField(t, apps["bonus"])["total"] != float64(15) {
		t.Fatal(apps)
	}
	f.api("configuration_block", map[string]any{"profile": "alex"})
	if profileReport(f, "alex")["status"] != "blocked" {
		t.Fatal("block ignored")
	}
	f.api("configuration_reset", map[string]any{"profile": "alex"})
	f.api("configuration_ticket", map[string]any{"profile": "alex", "minutes": 15})
	deadline := profileReport(f, "alex")["override_expires"].(float64)
	f.api("configuration_ticket", map[string]any{"profile": "alex", "minutes": 15})
	if profileReport(f, "alex")["override_expires"] != deadline+900 {
		t.Fatal("ticket was not additive")
	}
	f.restart()
	if profileReport(f, "alex")["override_expires"] != deadline+900 || profileReport(f, "alex")["bonus_today"] != float64(15) {
		t.Fatal("daemon restart lost runtime state")
	}
	f.host.mu.Lock()
	f.host.network = map[string]platform.NetworkOverride{} // native reboot clears RAM, not core documents
	f.host.mu.Unlock()
	f.restart()
	if profileReport(f, "alex")["override"] != "none" || profileReport(f, "alex")["bonus_today"] != float64(15) {
		t.Fatal("reboot confused native RAM and durable bonus")
	}
	c := f.configuration()
	c.Current.Profiles[0].Name = "Alex candidate"
	pending := f.api("configuration_apply", map[string]any{"revision": c.Revision, "current": c.Current})
	if pending["status"] != "confirmation_required" {
		t.Fatal(pending)
	}
	f.restart()
	if f.configuration().Current.Profiles[0].Name != "Alex" {
		t.Fatal("lost confirmation was committed")
	}
	last := f.host.intents[len(f.host.intents)-1]
	status := objectField(t, f.api("configuration_status", nil)["network_application"])
	if last.Format != platform.NetworkIntentFormat || last.ID == "" || status["intent_id"] != last.ID || status["revision"] != last.Revision {
		t.Fatal("apply acknowledgement not bound to the recorded intent", status, last)
	}
	applied := len(f.host.intents)
	f.advance(10 * time.Hour)
	if profileReport(f, "alex")["status"] != "allowed" {
		t.Fatal("overnight window did not open at 22:00")
	}
	f.advance(8 * time.Hour)
	if profileReport(f, "alex")["status"] != "blocked" || profileReport(f, "alex")["bonus_today"] != float64(0) {
		t.Fatal("overnight window did not close or yesterday's bonus remained")
	}
	if len(f.host.intents) != applied {
		t.Fatal("schedule/bonus boundaries needed an extra native apply")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPortableSignedGrantPactAndProtectedRecovery(t *testing.T) {
	f := newPortableFixture(t)
	f.noon()
	kid, parent, captain := f.phone("fw-alex", "alex"), f.phone("fw-robin", "robin", "alex"), f.phone("fw-jordan", "jordan")
	r := f.upload(kid, "request", map[string]any{"type": "request", "v": 1, "request_id": "req-portable-0001", "device": kid.id, "profile": "alex", "target": "com.example.video", "minutes": 30, "label": "Video", "created": f.now().Unix()})
	if r["state"] != "pending" {
		t.Fatal(r)
	}
	// Administrator transport and signed parent transport share the same production ledger.
	requests := f.sync(parent)["requests"].([]any)
	if len(requests) != 1 {
		t.Fatal(requests)
	}
	unauthorized := f.upload(captain, "decision", map[string]any{"type": "decision", "v": 1, "request_id": "req-portable-0001", "request_sha256": objectField(t, requests[0])["request_sha256"], "device": captain.id, "approve": true, "minutes": 20, "created": f.now().Unix()})
	if unauthorized["error"] == nil {
		t.Fatal("phone without parent scope approved another profile", unauthorized)
	}
	r = f.upload(parent, "decision", map[string]any{"type": "decision", "v": 1, "request_id": "req-portable-0001", "request_sha256": objectField(t, requests[0])["request_sha256"], "device": parent.id, "approve": true, "minutes": 20, "created": f.now().Unix()})
	if r["state"] != "approved" || r["home_internet"] != true {
		t.Fatal(r)
	}
	grants := objectField(t, f.sync(kid)["apps"])["grants"].([]any)
	if len(grants) != 1 {
		t.Fatal("signed grant not delivered")
	}
	grantID := objectField(t, grants[0])["grant_id"].(string)
	r = f.upload(kid, "usage", map[string]any{"type": "usage", "v": 1, "device": kid.id, "profile": "alex", "day": f.now().Format(time.DateOnly), "seq": 1,
		"total": 900, "apps": map[string]int{"com.example.video": 600}, "grants_used": map[string]int{grantID: 600}})
	if r["state"] != "stored" {
		t.Fatal(r)
	}
	f.restart()
	if objectField(t, f.api("requests", nil)["requests"].([]any)[0])["used"] != float64(600) {
		t.Fatal("spent grant usage lost at the service/storage boundary")
	}
	f.api("configuration_block", map[string]any{"profile": "alex"})
	f.sync(kid) // grant repair must not replace a manual block
	if profileReport(f, "alex")["override"] != "block" {
		t.Fatal("grant repair opened a manual block")
	}
	f.api("configuration_ticket", map[string]any{"profile": "jordan", "minutes": 15})
	f.api("configuration_pact", map[string]any{"until": 0, "window_day": "sun", "window_start": "10:00", "window_stop": "11:00"})
	intent := f.host.intents[len(f.host.intents)-1]
	recovery := false
	for _, rule := range intent.Router {
		recovery = recovery || rule.Profile == "jordan" && rule.Action == "drop" && rule.Recovery
	}
	if intent.Captain == nil || intent.Captain.ID != "jordan" || !recovery || f.host.network["alex"].Mode != "block" {
		t.Fatal("pact intent lost lockdown/recovery or changed another profile", intent)
	}
	f.restart()
	if objectField(t, f.sync(captain)["pact"])["active"] != true {
		t.Fatal("captain did not receive the pact after restart")
	}
	if f.call("/api/device_update", map[string]any{"device": captain.id, "profile": "alex"})["error"] == nil {
		t.Fatal("administrator moved a frozen captain phone to another profile")
	}
	frozen := f.configuration()
	frozen.Current.Profiles[2].ScreenMinutes = "999"
	if f.call("/api/configuration_amend", map[string]any{"revision": frozen.Revision, "current": frozen.Current})["error"] == nil {
		t.Fatal("pact amendment succeeded outside its window")
	}
	if objectField(t, objectField(t, f.sync(captain)["apps"])["screen"])["mon"] != float64(60) {
		t.Fatal("refused amendment changed the signed frozen policy")
	}
	for _, method := range []string{"configuration_allow", "configuration_ticket", "configuration_bonus_add"} {
		if f.call("/api/"+method, map[string]any{"profile": "jordan", "minutes": 15})["error"] == nil {
			t.Fatal("pact escape accepted", method)
		}
	}
	if f.call("/api/configuration_sail_request", map[string]any{})["error"] == nil {
		t.Fatal("ordinary API exposed protected recovery")
	}
	if f.call("/release/request", map[string]any{"answer": "wrong"})["error"] == nil {
		t.Fatal("wrong recovery phrase accepted")
	}
	r = f.call("/release/request", map[string]any{"answer": "press on"})
	if r["release_at"] != float64(f.now().Unix()+3600) {
		t.Fatal(r)
	}
	f.advance(time.Hour)
	f.api("configuration_tick", nil)
	if f.configuration().Pact != nil || objectField(t, f.sync(captain)["pact"])["active"] != false {
		t.Fatal("protected release did not finish")
	}
	if profileReport(f, "jordan")["override"] == "allow" {
		t.Fatal("release resurrected a pre-pact permit")
	}
	f.advance(time.Hour + time.Second)
	if objectField(t, f.sync(kid)["apps"])["grants"] != nil {
		t.Fatal("expired grant remained in signed policy")
	}
}

func TestPortableFailedConfigurationApplyRestoresCommittedDocument(t *testing.T) {
	f := newPortableFixture(t)
	before := f.configuration()
	candidate := copyJSON(before)
	candidate.Current.Profiles[0].Name = "Alex not applied"
	f.host.failConfiguration = 1
	if f.call("/api/configuration_apply", map[string]any{"revision": before.Revision, "current": candidate.Current})["error"] == nil {
		t.Fatal("failed configuration apply reported success")
	}
	if f.host.current.Revision != before.Revision || f.call("/api/configuration_export", nil)["error"] == nil {
		t.Fatal("failed candidate became an acknowledged configuration")
	}
	f.restart()
	if f.configuration().Revision != before.Revision {
		t.Fatal("restart did not reconcile the committed document")
	}
}

func TestPortableFailureRefusesPolicyUntilRestart(t *testing.T) {
	f := newPortableFixture(t)
	p := f.phone("fw-alex", "alex")
	f.host.failNet = 1
	if f.call("/api/configuration_block", map[string]any{"profile": "alex"})["error"] == nil {
		t.Fatal("apply failure reported success")
	}
	if f.call("/api/configuration_export", nil)["error"] == nil {
		t.Fatal("uncertain enforcement was not blocked")
	}
	if len(f.host.restored) != 1 || f.host.network["alex"].Mode != "" {
		t.Fatal("failed apply did not retain previous RAM state")
	}
	// Phone failures are unsigned, rather than a successful legacy policy fallback.
	code, body := f.exchange("/phone", p.query())
	if code != http.StatusInternalServerError || len(body) != 0 {
		t.Fatal("failed service did not refuse the authenticated policy request", code, string(body))
	}
	f.restart()
	if f.configuration().Pact != nil || profileReport(f, "alex")["override"] != "none" {
		t.Fatal("restart did not reconcile prior state")
	}
	f.host.failFacts = true
	if f.call("/api/configuration_report", nil)["error"] == nil {
		t.Fatal("missing facts became successful empty state")
	}
	if f.call("/api/configuration_block", map[string]any{"profile": "alex"})["error"] == nil {
		t.Fatal("mutation succeeded without facts")
	}
}

func TestPortableDNSAcknowledgesOnlyVerifiedState(t *testing.T) {
	var mu sync.Mutex
	var clients []map[string]any
	failReadback := false
	agh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/control/access/list":
			json.NewEncoder(w).Encode(map[string]any{"allowed_clients": []string{}, "disallowed_clients": []string{}, "blocked_hosts": []string{}})
		case "/control/clients":
			if clients == nil || failReadback {
				json.NewEncoder(w).Encode(map[string]any{"clients": []any{}})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"clients": clients})
			}
		case "/control/clients/add", "/control/clients/update":
			var v map[string]any
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				t.Error(err)
				return
			}
			if data, ok := v["data"].(map[string]any); ok {
				v = data
			}
			clients = []map[string]any{v}
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected AGH request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer agh.Close()
	f := newPortableFixture(t)
	phone := f.phone("fw-alex", "alex")
	c := f.configuration()
	c.Current.Settings.DNSDomain, c.Current.Settings.ResolverURL = "dns.example.test", agh.URL
	c.Current.Profiles[0].DNS, c.Current.Profiles[0].ClientID = "1", "fw-alex"
	f.commit(c)
	f.api("configuration_dns_auth", map[string]any{"username": "fixture", "password": "fixture-secret"})
	if f.sync(phone)["dns"] != nil {
		t.Fatal("desired DNS advertised before verification")
	}
	f.api("configuration_dns_sync", nil)
	ack := objectField(t, f.sync(phone)["dns"])
	if ack["hostname"] != "fw-alex.dns.example.test" {
		t.Fatal(ack)
	}
	mu.Lock()
	failReadback = true
	mu.Unlock()
	failed := f.call("/api/configuration_dns_sync", nil)
	if failed["error"] == nil || failed["error"] == "" {
		t.Fatal("unverified remote state acknowledged", failed)
	}
	f.restart()
	if objectField(t, f.sync(phone)["dns"])["hostname"] != ack["hostname"] {
		t.Fatal("verified DNS acknowledgement lost on restart")
	}
	data, err := os.ReadFile(filepath.Join(f.dir, "dns.json"))
	if err != nil || bytes.Contains(data, []byte("fixture-secret")) {
		t.Fatal("core persisted native credentials", err)
	}
}
