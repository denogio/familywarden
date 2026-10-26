package identity

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/usign"
)

// Extra-time requests and grants (P8, protocol §3/§4, decisions A7/A15/D3).
//
// A child phone signs a request; a parent phone in scope (or the administrator)
// signs a decision bound to the exact request bytes; an approval becomes a grant,
// delivered only to the requesting phone in its router-signed policy, valid for two
// hours after approval. The ledger is durable, so a router reboot can neither
// revive an answered request. After retention pruning, the durable time floor
// prevents rollback from making a pruned signed upload fresh again.

const (
	requestsFile    = "requests.json"
	requestsVersion = 1
	maxLedger       = 256
	maxDeviceGrants = 32
	keepAnswered    = 24 * time.Hour
	maxUpload       = 16 << 10
	grantsFeature   = "grants-v1"
	targetTotal     = "total"
)

var (
	validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
	validHash      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	validGrantID   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	validPkg       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
)

// entry is one request and its answer. Expiry is bounded by both the authenticated
// creation time and router receipt, so delayed uploads cannot gain another hour.
type entry struct {
	ID           string `json:"id"`
	Hash         string `json:"sha256"` // of the exact signed request bytes
	Profile      string `json:"profile"`
	Device       string `json:"device"`
	DeviceKey    string `json:"device_key"`
	Expires      int64  `json:"expires"`
	GrantExpired bool   `json:"grant_expired,omitempty"`
	Target       string `json:"target"` // "total" or a package
	Label        string `json:"label,omitempty"`
	Note         string `json:"note,omitempty"`
	Minutes      int    `json:"minutes"`
	Received     int64  `json:"received"`
	State        string `json:"state"` // pending | expired | approved | denied
	DecidedBy    string `json:"decided_by,omitempty"`
	DecidedAt    int64  `json:"decided_at,omitempty"`
	Granted      int    `json:"granted,omitempty"`
	GrantID      string `json:"grant_id,omitempty"`
	NotAfter     int64  `json:"not_after,omitempty"`
}

type ledgerDoc struct {
	Version  int     `json:"version"`
	LastTime int64   `json:"last_time"`
	Requests []entry `json:"requests"`
}

func (e entry) expired(now int64) bool {
	return e.State == "pending" && now >= e.Expires
}

func (e entry) status(now int64) string {
	if e.expired(now) {
		return "expired"
	}
	return e.State
}

// end is when an entry stops mattering (request expiry or grant end).
func (e entry) end() int64 {
	if e.NotAfter > 0 {
		return e.NotAfter
	}
	return e.Expires
}

// ---- signed uploads on the phone port ----

// upload is "<usign signature>\n<exact JSON bytes>", the same shape as replies.
func splitUpload(body []byte) (sig string, raw []byte, ok bool) {
	i := bytes.IndexByte(body, '\n')
	if i < 0 || len(body) > maxUpload {
		return "", nil, false
	}
	return string(body[:i]), body[i+1:], true
}

// Strict object decode: exact names, no duplicates/nulls, required fields and
// actual EOF (not a syntax error in trailing bytes).
func decodeStrict(raw []byte, v any, required, optional string) bool {
	if !utf8.Valid(raw) {
		return false
	}
	fields := map[string]bool{}
	for _, name := range strings.Fields(required + " " + optional) {
		fields[name] = false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		seen, known := fields[name]
		if err != nil || !ok || !known || seen {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(value, []byte("null")) {
			return false
		}
		fields[name] = true
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return false
	}
	for _, name := range strings.Fields(required) {
		if !fields[name] {
			return false
		}
	}
	var rest json.RawMessage
	return d.Decode(&rest) == io.EOF && json.Unmarshal(raw, v) == nil
}

func cleanText(s string, max int) (string, bool) {
	if len([]rune(s)) > max {
		return "", false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return "", false
		}
	}
	return s, true
}

// App handles a signed upload ("request", "decision" or "usage"). Unauthenticated input
// gets a bare status; everything after authentication gets a signed reply.
func (s *Service) App(op string, body []byte) (int, []byte) {
	sig, raw, ok := splitUpload(body)
	if !ok || !validB64(sig, 100) {
		return StatusMalformed, nil
	}
	var head struct {
		Device string `json:"device"`
	}
	if json.Unmarshal(raw, &head) != nil || !validDevice.MatchString(head.Device) {
		return StatusMalformed, nil
	}
	s.mu.Lock()
	code, reply, ticket := s.uploadLocked(op, sig, raw, head.Device)
	var rk usign.SecretKey
	if code == StatusOK {
		rk = *s.key
	}
	s.mu.Unlock()
	if code != StatusOK {
		return code, nil
	}
	if ticket != nil {
		reply = append(reply, field{"home_internet", s.openTicket(*ticket)})
	}
	return s.sign(rk, reply)
}

// Verification, current scope and the durable decision share one lock, so a
// registry change cannot race between authentication and committing authority.
func (s *Service) uploadLocked(op, sig string, raw []byte, device string) (int, object, *ticketJob) {
	i := s.find(device)
	var d Device
	if i >= 0 {
		d = s.devices[i]
	}
	kind := map[string]string{"request": "request", "decision": "decision", "usage": "usage", "app": "app"}[op]
	if i < 0 || kind == "" {
		return StatusForbidden, nil, nil
	}
	key, err := usign.ParsePublicKey(d.PubKey)
	if err != nil || !key.Verify(sig, protocol.Message(kind, raw)) {
		return StatusForbidden, nil, nil
	}
	if _, err := s.routerKey(); err != nil {
		return StatusError, nil, nil
	}
	var reply object
	var ticket *ticketJob
	err = s.ad.WithContext(func(ctx platform.Context) error {
		switch op {
		case "request":
			reply = s.submit(d, raw, ctx)
		case "usage":
			reply = s.usage(d, raw, ctx)
		case "app":
			reply = s.appDecision(d, raw, ctx)
		default:
			reply, ticket = s.decide(d, raw, ctx)
		}
		return nil
	})
	if err != nil {
		return StatusError, nil, nil
	}
	h := sha256.Sum256(raw)
	return StatusOK, append(object{{"v", 1}, {"type", op + "_reply"}, {"device", d.ID},
		{"message_sha256", hex.EncodeToString(h[:])}, {"server_time", s.Now().Unix()}}, reply...), ticket
}

type ticketJob struct {
	profile string
	until   int64
}

// openTicket runs outside the lock: the adapter recompiles the firewall.
func (s *Service) openTicket(t ticketJob) bool {
	ok, err := s.ad.EnsureTicket(t.profile, t.until)
	if err != nil {
		log.Printf("grant ticket for %s: %v", t.profile, err)
	}
	return ok && err == nil
}

type requestMsg struct {
	Type      string `json:"type"`
	V         int    `json:"v"`
	RequestID string `json:"request_id"`
	Device    string `json:"device"`
	Profile   string `json:"profile"`
	Target    string `json:"target"`
	Minutes   int    `json:"minutes"`
	Label     string `json:"label,omitempty"`
	Note      string `json:"note,omitempty"`
	Created   int64  `json:"created"`
}

func refuse(code string) object { return object{{"error", code}} }

func (s *Service) submit(d Device, raw []byte, ctx platform.Context) object {
	var m requestMsg
	if !decodeStrict(raw, &m, "type v request_id device profile target minutes created", "label note") || m.Type != "request" || m.V != 1 || m.Device != d.ID || !validRequestID.MatchString(m.RequestID) {
		return refuse(string(protocol.ErrMalformed))
	}
	out := object{{"request_id", m.RequestID}}
	label, ok1 := cleanText(m.Label, 64)
	note, ok2 := cleanText(m.Note, 140)
	now := s.Now()
	created := time.Unix(m.Created, 0)
	switch {
	case !ok1 || !ok2:
		return append(out, refuse(string(protocol.ErrMalformed))...)
	case m.Profile != d.Profile:
		return append(out, refuse(string(protocol.ErrWrongProfile))...)
	case !validTarget(m.Target):
		return append(out, refuse(string(protocol.ErrUnknownPackage))...)
	case m.Minutes < 1 || m.Minutes > protocol.MaxMinutes:
		return append(out, refuse(string(protocol.ErrInvalidMinutes))...)
	case m.Created <= 0 || m.Created > maxEpoch || now.Sub(created) >= protocol.RequestTTL || created.Sub(now) > protocol.ClockSkew:
		return append(out, refuse(string(protocol.ErrRequestExpired))...)
	}
	if prof, ok := ctx.Profiles[d.Profile]; !ok || prof.AppsRev == "" {
		return append(out, refuse("app_control_off")...)
	}
	if ctx.PactCaptain != "" && d.Profile == ctx.PactCaptain {
		return append(out, refuse("pact_forbids")...)
	}
	hash := sha256.Sum256(raw)
	h := hex.EncodeToString(hash[:])
	ledger, err := s.loadLedger()
	if err != nil {
		return append(out, refuse("unavailable")...)
	}
	nowU := now.Unix()
	for _, e := range ledger {
		if e.ID == m.RequestID {
			if e.Hash == h { // a retry of the same upload
				return append(out, field{"state", e.status(nowU)})
			}
			return append(out, refuse("replay")...)
		}
	}
	for _, e := range ledger {
		if e.Profile == d.Profile && e.State == "pending" && !e.expired(nowU) {
			return append(out, refuse("request_open")...)
		}
	}
	if len(ledger) >= maxLedger {
		return append(out, refuse("unavailable")...)
	}
	next := append(append([]entry{}, ledger...), entry{ID: m.RequestID, Hash: h, Profile: d.Profile, Device: d.ID,
		DeviceKey: d.PubKey, Expires: min(nowU, m.Created) + int64(protocol.RequestTTL/time.Second),
		Target: m.Target, Label: label, Note: note, Minutes: m.Minutes, Received: nowU, State: "pending"})
	if err := s.saveLedger(next, nowU); err != nil {
		log.Printf("saving request: %v", err)
		return append(out, refuse("unavailable")...)
	}
	log.Printf("request %s from %s: %d min for %s", m.RequestID, d.Profile, m.Minutes, m.Target)
	return append(out, field{"state", "pending"})
}

type decisionMsg struct {
	Type      string `json:"type"`
	V         int    `json:"v"`
	RequestID string `json:"request_id"`
	Hash      string `json:"request_sha256"`
	Device    string `json:"device"`
	Approve   bool   `json:"approve"`
	Minutes   int    `json:"minutes"`
	Created   int64  `json:"created"`
}

// decide applies a parent's signed decision; phone issuers cannot collide with
// the local administrator sentinel, even if a device id happens to be "router".
func (s *Service) decide(d Device, raw []byte, ctx platform.Context) (object, *ticketJob) {
	var m decisionMsg
	if !decodeStrict(raw, &m, "type v request_id request_sha256 device approve minutes created", "") || m.Type != "decision" || m.V != 1 || m.Device != d.ID || !validRequestID.MatchString(m.RequestID) || !validHash.MatchString(m.Hash) {
		return refuse(string(protocol.ErrMalformed)), nil
	}
	created := time.Unix(m.Created, 0)
	if now := s.Now(); m.Created <= 0 || m.Created > maxEpoch || now.Sub(created) >= protocol.RequestTTL || created.Sub(now) > protocol.ClockSkew {
		return object{{"request_id", m.RequestID}, {"error", "decision_expired"}}, nil
	}
	return s.answer("phone:"+d.ID, m.RequestID, m.Hash, m.Approve, m.Minutes, func(e entry) bool {
		for _, p := range effectiveApproves(d, ctx) {
			if p == e.Profile {
				return true
			}
		}
		return false
	}, ctx)
}

// answer records the first decision on a pending request; an identical retry
// gets the same result, anything else is refused. hash "" skips the binding
// check (administrator via the local API, which sees the request itself).
func (s *Service) answer(by, id, hash string, approve bool, minutes int, allowed func(entry) bool, ctx platform.Context) (object, *ticketJob) {
	out := object{{"request_id", id}}
	ledger, err := s.loadLedger()
	if err != nil {
		return append(out, refuse("unavailable")...), nil
	}
	now := s.Now().Unix()
	idx := -1
	for i, e := range ledger {
		if e.ID == id {
			idx = i
		}
	}
	if idx < 0 || (hash != "" && ledger[idx].Hash != hash) {
		return append(out, refuse("unknown_request")...), nil
	}
	e := ledger[idx]
	if !allowed(e) || (ctx.PactCaptain != "" && e.Profile == ctx.PactCaptain) {
		return append(out, refuse(string(protocol.ErrNotParent))...), nil
	}
	if !s.requesterExists(e) {
		return append(out, refuse("not_enrolled")...), nil
	}
	if prof, ok := ctx.Profiles[e.Profile]; !ok || prof.AppsRev == "" {
		return append(out, refuse("app_control_off")...), nil
	}
	if approve && (minutes < 1 || minutes > e.Minutes) {
		return append(out, refuse(string(protocol.ErrInvalidMinutes))...), nil
	}
	if e.State == "expired" || e.expired(now) {
		return append(out, refuse(string(protocol.ErrRequestExpired))...), nil
	}
	if e.State != "pending" {
		same := e.DecidedBy == by && (e.State == "approved") == approve && (!approve || e.Granted == minutes)
		if same {
			var t *ticketJob
			if approve && !e.GrantExpired && now < e.NotAfter {
				t = &ticketJob{e.Profile, e.NotAfter}
			}
			return append(out, s.decisionFields(e)...), t
		}
		return append(out, field{"error", "already_answered"}, field{"answered_by", s.nameOf(e.DecidedBy)}), nil
	}
	e.DecidedBy, e.DecidedAt = by, now
	if approve {
		if len(s.grantsFor(e.Profile, e.Device, now)) >= maxDeviceGrants {
			return append(out, refuse("unavailable")...), nil
		}
		id, err := newID()
		if err != nil {
			return append(out, refuse("unavailable")...), nil
		}
		e.State, e.Granted, e.GrantID = "approved", minutes, id
		e.NotAfter = now + int64(protocol.GrantLifetime/time.Second)
	} else {
		e.State = "denied"
	}
	next := append([]entry{}, ledger...)
	next[idx] = e
	if err := s.saveLedger(next, now); err != nil {
		log.Printf("saving decision: %v", err)
		return append(out, refuse("unavailable")...), nil
	}
	log.Printf("request %s %s by %s (%d min)", id, e.State, by, e.Granted)
	var t *ticketJob
	if approve {
		t = &ticketJob{e.Profile, e.NotAfter}
	}
	return append(out, s.decisionFields(e)...), t
}

func (s *Service) decisionFields(e entry) object {
	o := object{{"state", e.State}}
	if e.State == "approved" {
		o = append(o, field{"granted", e.Granted}, field{"not_after", e.NotAfter})
	}
	return o
}

func (s *Service) nameOf(by string) string {
	if by == "router" {
		return "router"
	}
	by = strings.TrimPrefix(by, "phone:")
	if i := s.find(by); i >= 0 {
		return nameOr(s.devices[i].Name, by)
	}
	return by
}

func newID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

// ---- policy views (lock held) ----

// grantsFor returns the active grants of a profile, sorted by id.
func (s *Service) grantsFor(profile, device string, now int64) []entry {
	var out []entry
	for _, e := range s.ledger {
		if e.Profile == profile && e.Device == device && s.requesterExists(e) && e.State == "approved" &&
			!e.GrantExpired && now >= e.DecidedAt && now < e.NotAfter {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].GrantID < out[b].GrantID })
	return out
}

// appsView is the profile's app revision and requirements including its active
// grants: a grant changes what the phone must apply, so it changes the revision
// (and acknowledging it shows delivery).
// Core-approved new apps (newapps.go) change it too; they need no new feature.
func appsView(prof platform.Profile, grants []entry, approved []string) (string, []string) {
	if prof.AppsRev == "" || (len(grants) == 0 && len(approved) == 0) {
		return prof.AppsRev, prof.Requires
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", prof.AppsRev)
	for _, g := range grants {
		fmt.Fprintf(h, "grant|%s|%s|%d|%d|%d\n", g.GrantID, g.Target, g.Granted, g.DecidedAt, g.NotAfter)
	}
	for _, p := range approved {
		fmt.Fprintf(h, "approve|%s\n", p)
	}
	req := prof.Requires
	if len(grants) > 0 {
		req = append(append([]string{}, prof.Requires...), grantsFeature)
		sort.Strings(req)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], req
}

// withGrants adds active grants to the shell's apps object and replaces its revision.
func withGrants(apps json.RawMessage, rev string, grants []entry, approved []string) (json.RawMessage, error) {
	apps, err := withApprovals(apps, approved)
	if err != nil {
		return nil, err
	}
	if len(grants) == 0 && len(approved) == 0 {
		return apps, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(apps, &m); err != nil {
		return nil, err
	}
	m["rev"], _ = json.Marshal(rev)
	if len(grants) > 0 {
		list := []object{}
		for _, g := range grants {
			list = append(list, object{{"grant_id", g.GrantID}, {"device_id", g.Device}, {"target", g.Target}, {"minutes", g.Granted},
				{"not_before", g.DecidedAt}, {"not_after", g.NotAfter}})
		}
		m["grants"], _ = json.Marshal(list)
	}
	return json.Marshal(m)
}

// childRequest is the latest request of a profile for its phones' UI.
func (s *Service) childRequest(profile string, now int64) object {
	ledger := s.ledger
	var last *entry
	for i := range ledger {
		if ledger[i].Profile == profile && (last == nil || ledger[i].Received >= last.Received) {
			last = &ledger[i]
		}
	}
	if last == nil {
		return nil
	}
	o := object{{"request_id", last.ID}, {"target", last.Target}, {"minutes", last.Minutes},
		{"state", last.status(now)}, {"expires", last.Expires}}
	if last.DecidedBy != "" {
		o = append(o, field{"answered_by", s.nameOf(last.DecidedBy)}, field{"answered_at", last.DecidedAt})
	}
	if last.State == "approved" {
		o = append(o, field{"granted", last.Granted}, field{"not_after", last.NotAfter})
	}
	return o
}

// parentRequests lists pending requests a parent phone may answer.
func (s *Service) parentRequests(d Device, ctx platform.Context, now int64) []object {
	ledger := s.ledger
	scope := map[string]bool{}
	for _, p := range effectiveApproves(d, ctx) {
		scope[p] = true
	}
	out := []object{}
	for _, e := range ledger {
		if !scope[e.Profile] || e.State != "pending" || e.expired(now) || !s.requesterExists(e) || ctx.Profiles[e.Profile].AppsRev == "" {
			continue
		}
		o := object{{"request_id", e.ID}, {"request_sha256", e.Hash}, {"profile_id", e.Profile},
			{"profile_name", ctx.Profiles[e.Profile].Name}, {"target", e.Target}, {"minutes", e.Minutes},
			{"received", e.Received}, {"expires", e.Expires}}
		if e.Label != "" {
			o = append(o, field{"label", e.Label})
		}
		if e.Note != "" {
			o = append(o, field{"note", e.Note})
		}
		out = append(out, o)
	}
	return out
}

// ---- administrator (LuCI via the rpcd shim) ----

// Requests lists the ledger for the administrator (newest first).
func (s *Service) Requests() Reply {
	ctx, err := s.ad.Context()
	if err != nil {
		return fail("platform state unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ledger, err := s.loadLedger()
	if err != nil {
		return fail("requests unavailable")
	}
	now := s.Now().Unix()
	list := []Reply{}
	for i := len(ledger) - 1; i >= 0; i-- {
		e := ledger[i]
		r := Reply{"request_id": e.ID, "profile": e.Profile, "profile_name": ctx.Profiles[e.Profile].Name,
			"device": e.Device, "target": e.Target, "label": e.Label, "note": e.Note, "minutes": e.Minutes,
			"received": e.Received, "expires": e.Expires, "state": e.status(now)}
		if e.DecidedBy != "" {
			r["answered_by"], r["answered_at"] = s.nameOf(e.DecidedBy), e.DecidedAt
		}
		if e.State == "approved" {
			r["granted"], r["not_after"] = e.Granted, e.NotAfter
			r["delivered"] = s.delivered[e.GrantID] > 0
			if used, ok := s.grantUse(e, ctx); ok {
				r["used"] = used
			}
		}
		list = append(list, r)
	}
	return Reply{"requests": list}
}

// Answer is the administrator's decision from LuCI (issuer "router").
type Answer struct {
	RequestID string `json:"request_id"`
	Approve   bool   `json:"approve"`
	Minutes   int    `json:"minutes"`
}

// DecodeAnswer rejects accidental/ambiguous denials at the local API boundary.
func DecodeAnswer(raw []byte) (Answer, error) {
	var a Answer
	if !decodeStrict(raw, &a, "request_id approve", "minutes") {
		return a, fmt.Errorf("malformed decision")
	}
	return a, nil
}

func (s *Service) AnswerRequest(a Answer) Reply {
	if !validRequestID.MatchString(a.RequestID) {
		return fail("invalid request")
	}
	var ticket *ticketJob
	r := s.mutate(func(ctx platform.Context) Reply {
		out, t := s.answer("router", a.RequestID, "", a.Approve, a.Minutes, func(entry) bool { return true }, ctx)
		ticket = t
		r := Reply{}
		for _, f := range out {
			r[f.k] = f.v
		}
		if _, bad := r["error"]; !bad {
			r["status"] = "ok"
		}
		return r
	})
	if _, bad := r["error"]; !bad && ticket != nil {
		r["home_internet"] = s.openTicket(*ticket)
	}
	return r
}

// noteDelivery records that a phone acknowledged a revision containing grants.
func (s *Service) noteDelivery(ackRev, currentRev string, grants []entry, now int64) {
	if ackRev == "" || ackRev != currentRev {
		return
	}
	for _, g := range grants {
		if s.delivered[g.GrantID] == 0 {
			s.delivered[g.GrantID] = now
		}
	}
}
