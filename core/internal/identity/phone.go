package identity

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/denogio/familywarden/core/internal/usign"
)

// Status codes of the phone endpoint (unchanged from the shell CGI).
const (
	StatusOK        = 200
	StatusMalformed = 400
	StatusForbidden = 403
	StatusError     = 500
)

var nameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// param mirrors the old CGI: first "name=" pair, only the %-escapes base64 needs
// decoded. A raw "+" stays "+" (url.ParseQuery would turn it into a space).
func param(query, name string) string {
	for _, kv := range strings.Split(query, "&") {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return strings.NewReplacer("%2B", "+", "%2b", "+", "%2F", "/", "%2f", "/", "%3D", "=", "%3d", "=").Replace(v)
		}
	}
	return ""
}

func verify(key, sig string, msg string) bool {
	k, err := usign.ParsePublicKey(key)
	return err == nil && k.Verify(sig, []byte(msg))
}

// Phone answers one signed sync request (v2/v3, plus authenticated capabilities).
// The reply is the router signature line, a newline, then the exact signed payload.
func (s *Service) Phone(query string) (status int, body []byte) {
	dev, nonce, key, sig := param(query, "device"), param(query, "nonce"), param(query, "key"), param(query, "sig")
	sig3, applied, caps, sigcaps := param(query, "sig3"), param(query, "applied"), param(query, "caps"), param(query, "sigcaps")
	name := nameChars.ReplaceAllString(param(query, "name"), "")
	if len(name) > 40 {
		name = name[:40]
	}
	if !validDevice.MatchString(dev) || !validNonce.MatchString(nonce) || !validB64(key, 56) || !validB64(sig, 100) {
		return StatusMalformed, nil
	}
	version := 2
	if sig3 != "" {
		if !validB64(sig3, 100) || (applied != "-" && !validRev.MatchString(applied)) {
			return StatusMalformed, nil
		}
		if !verify(key, sig3, "familywarden-sync-v3\n"+dev+"\n"+nonce+"\n"+applied+"\n") {
			return StatusForbidden, nil
		}
		version = 3
	}
	if caps != "" || sigcaps != "" {
		if version != 3 || !canonicalCaps(caps) || !validB64(sigcaps, 100) {
			return StatusMalformed, nil
		}
		if !verify(key, sigcaps, "familywarden-sync-caps-v1\n"+dev+"\n"+nonce+"\n"+applied+"\n"+caps+"\n") {
			return StatusForbidden, nil
		}
	}
	if !verify(key, sig, "familywarden-sync-v2\n"+dev+"\n"+nonce+"\n") {
		return StatusForbidden, nil
	}

	var tickets []ticketJob
	// Registered before Unlock: restore lost RAM tickets after releasing the lock.
	defer func() {
		if status == StatusOK {
			for _, t := range tickets {
				s.openTicket(t)
			}
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	rk, err := s.routerKey()
	if err != nil {
		log.Printf("router key: %v", err)
		return StatusError, nil
	}
	now := s.Now().Unix()
	base := object{{"v", version}, {"server_time", now}, {"nonce", nonce}, {"device", dev}}
	i := s.find(dev)
	if i < 0 || s.devices[i].PubKey != key {
		s.addPending(dev, key, name, now)
		return s.sign(rk, append(base, field{"enrollment", "pending"}))
	}
	d := s.devices[i]
	if _, err := s.loadLedger(); err != nil {
		log.Printf("request ledger: %v", err)
		return StatusError, nil
	}
	delete(s.pending, dev)
	pol, err := s.ad.Policy(d.Profile, version)
	if err != nil {
		log.Printf("policy for %s: %v", d.Profile, err)
		return StatusError, nil
	}
	ctx := pol.Context
	prof, exists := ctx.Profiles[d.Profile]
	grants := s.grantsFor(d.Profile, d.ID, now)
	if d.Profile == ctx.PactCaptain || len(pol.Apps) == 0 || string(pol.Apps) == "null" {
		grants = nil // no grants beyond a pact, and none without app control
	}
	for _, g := range grants {
		tickets = append(tickets, ticketJob{g.Profile, g.NotAfter})
	}
	rev, requires := appsView(prof, grants)
	seen := seenPhone{at: now, caps: caps}
	if version == 3 && applied != "-" {
		seen.applied = applied
	}
	if !covers(caps, version, requires) {
		seen.applied, seen.err = "", "unsupported_capabilities"
	}
	s.seen[dev] = seen
	if !exists {
		// Its profile is gone: no binding, no policy, as before.
		return s.sign(rk, append(base, field{"enrollment", "pending"}))
	}
	out := append(base, field{"enrollment", "approved"})
	if seen.err == "" {
		s.noteDelivery(seen.applied, rev, grants, now)
	}
	if version == 3 || len(requires) > 0 {
		out = append(out, field{"requires", nonNil(requires)})
		if seen.err != "" {
			// Signed, request-bound refusal: no binding and no weaker policy.
			return s.sign(rk, append(out, field{"error", "unsupported_capabilities"}))
		}
	}
	if validTZ.MatchString(ctx.TimeZone) {
		out = append(out, field{"timezone", ctx.TimeZone})
	}
	binding := object{{"device_id", dev}, {"profile_id", d.Profile}, {"profile_name", prof.Name},
		{"captain", pol.Captain != "" && pol.Captain == d.Profile},
		{"administrator", d.Administrator},
		{"can_self_remove", d.Administrator || s.removals[dev] > now}}
	if version == 3 {
		scope := []object{}
		for _, p := range effectiveApproves(d, ctx) {
			scope = append(scope, object{{"profile_id", p}, {"profile_name", ctx.Profiles[p].Name}})
		}
		binding = append(binding, field{"approves", scope})
	}
	out = append(out, field{"binding", binding})
	if version == 3 {
		parents := []object{}
		for _, p := range s.devices {
			if len(parents) == maxParents {
				break
			}
			for _, a := range effectiveApproves(p, ctx) {
				if a == d.Profile {
					parents = append(parents, object{{"device_id", p.ID}, {"name", nameOr(p.Name, p.ID)}, {"pubkey", p.PubKey}})
				}
			}
		}
		out = append(out, field{"parents", parents})
		if r := s.childRequest(d.Profile, now); r != nil && prof.AppsRev != "" {
			out = append(out, field{"request", r})
		}
		if len(effectiveApproves(d, ctx)) > 0 {
			out = append(out, field{"requests", s.parentRequests(d, ctx, now)})
			if u := s.parentUsage(d, ctx); u != nil {
				out = append(out, field{"usage", u})
			}
		}
	}
	out = append(out, field{"captain", pol.Captain}, field{"pact", raw(pol.Pact)}, field{"rules", raw(pol.Rules)})
	if len(pol.DNS) > 0 && string(pol.DNS) != "null" {
		out = append(out, field{"dns", raw(pol.DNS)})
	}
	if version == 3 && len(pol.Apps) > 0 && string(pol.Apps) != "null" {
		apps, err := withGrants(pol.Apps, rev, grants)
		if err != nil {
			log.Printf("apps of %s: %v", d.Profile, err)
			return StatusError, nil
		}
		out = append(out, field{"apps", raw(apps)})
	}
	return s.sign(rk, out)
}

func (s *Service) sign(rk usign.SecretKey, o object) (int, []byte) {
	payload, err := json.Marshal(o)
	if err != nil || len(payload) > 64<<10 {
		log.Printf("reply encoding failed or exceeded 64 KiB: %v", err)
		return StatusError, nil
	}
	return StatusOK, append([]byte(rk.Sign(payload)+"\n"), payload...)
}

// addPending remembers an unapproved phone in RAM, at most maxPending, oldest dropped.
func (s *Service) addPending(dev, key, name string, now int64) {
	p, ok := s.pending[dev]
	if !ok {
		for len(s.pending) >= maxPending {
			oldest := ""
			for id, q := range s.pending {
				if oldest == "" || q.last < s.pending[oldest].last {
					oldest = id
				}
			}
			delete(s.pending, oldest)
		}
		p.first = now
	}
	p.key, p.name, p.last = key, name, now
	s.pending[dev] = p
}

func raw(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("null")
	}
	return m
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func nameOr(n, id string) string {
	if n == "" {
		return id
	}
	return n
}
