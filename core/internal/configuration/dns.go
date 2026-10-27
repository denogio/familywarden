package configuration

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/agh"
	"github.com/denogio/familywarden/core/internal/rules"
)

// DNS is the desired AdGuard Home state: the DNS domain and every profile whose
// phones use strict DNS. During an active pact the captain's DNS can only be
// tightened: its snapshot keeps DNS on, keeps every protection/service and the
// domain, while live additions apply at once (removals wait for the pact's end).
func DNS(c rules.Configuration, now time.Time) (domain, captain string, profiles []agh.Profile) {
	domain = c.Current.Settings.DNSDomain
	var snap rules.ConfigurationDNS
	if Active(c.Pact, now) {
		captain, snap = c.Pact.Captain, c.Pact.DNS
		if snap.Domain != "" {
			domain = snap.Domain
		}
	}
	profiles = []agh.Profile{}
	seen := false
	for _, p := range c.Current.Profiles {
		bound := captain != "" && p.ID == captain
		seen = seen || bound
		if p.DNS != "1" && !(bound && snap.Enabled == "1") {
			continue
		}
		v := agh.Profile{ID: p.ID, Name: p.Name, ClientID: p.ClientID,
			Protections: append([]string{}, p.Protections...), Services: append([]string{}, p.Services...)}
		if bound {
			if v.ClientID == "" {
				v.ClientID = snap.ClientID
			}
			v.Protections = append(v.Protections, snap.Protections...)
			v.Services = append(v.Services, snap.Services...)
		}
		profiles = append(profiles, v)
	}
	// Even if the captain's profile was deleted meanwhile.
	if captain != "" && !seen && snap.Enabled == "1" {
		profiles = append(profiles, agh.Profile{ID: captain, Name: captain, ClientID: snap.ClientID,
			Protections: append([]string{}, snap.Protections...), Services: append([]string{}, snap.Services...)})
	}
	return domain, captain, profiles
}

var dnsLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
var dnsService = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// Match Android's validHostname contract before sending any policy to a phone.
func validDNSHostname(host string) bool {
	if len(host) < 3 || len(host) > 253 || !strings.Contains(host, ".") || !strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// New edits are strict, while untouched legacy fields retain their import
// semantics. In particular, an existing ClientID does not waive the domain
// requirement when switching DNS on.
func validateDNSEdit(old, next rules.Configuration) error {
	domain := next.Current.Settings.DNSDomain
	domainChanged := domain != old.Current.Settings.DNSDomain
	if domainChanged && domain != "" && !validDNSHostname("a."+domain) {
		return errors.New("invalid DNS domain")
	}
	endpoint := next.Current.Settings.ResolverURL
	if endpoint != old.Current.Settings.ResolverURL && endpoint != "" && !agh.ValidEndpoint(endpoint) {
		return errors.New("invalid AdGuard Home URL")
	}
	for _, p := range next.Current.Profiles {
		before := profile(old.Current, p.ID)
		edited := before == nil || p.DNS != before.DNS || p.ClientID != before.ClientID || !reflect.DeepEqual(p.Protections, before.Protections) || !reflect.DeepEqual(p.Services, before.Services)
		if !edited && !domainChanged {
			continue
		}
		if err := validateDNSProfile(p, domain); err != nil {
			return err
		}
	}
	return nil
}

func validateDNSProfile(p rules.ConfigurationProfile, domain string) error {
	if p.ClientID != "" && !agh.ValidClientID(p.ClientID) {
		return errors.New("invalid DNS ClientID")
	}
	for _, v := range p.Protections {
		switch v {
		case "filter_lists", "safe_browsing", "parental", "safe_search":
		default:
			return errors.New("invalid DNS protection")
		}
	}
	for _, v := range p.Services {
		if !dnsService.MatchString(v) {
			return errors.New("invalid DNS service")
		}
	}
	if p.DNS == "1" {
		if domain == "" {
			return errors.New("DNS domain required before enabling DNS")
		}
		if !validDNSHostname(p.ClientID + "." + domain) {
			return errors.New("invalid DNS hostname")
		}
	}
	return nil
}

func assignDNSClientIDs(next *rules.Configuration, domain string) error {
	used := map[string]bool{}
	for _, p := range next.Current.Profiles {
		if p.ClientID != "" {
			if used[p.ClientID] {
				return errors.New("duplicate DNS ClientID")
			}
			used[p.ClientID] = true
		}
	}
	for i := range next.Current.Profiles {
		p := &next.Current.Profiles[i]
		if p.DNS != "1" || p.ClientID != "" {
			continue
		}
		if domain == "" {
			return errors.New("DNS domain required before enabling DNS")
		}
		for {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			candidate := "fw" + hex.EncodeToString(id[:])
			if !used[candidate] {
				p.ClientID = candidate
				used[candidate] = true
				break
			}
		}
	}
	return nil
}

func dnsUnion(groups ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, v := range group {
			if v != "" && !seen[v] {
				out = append(out, v)
				seen[v] = true
			}
		}
	}
	return out
}

// DNS edits during a pact are explicit, irreversible transactions, separate
// from ordinary confirmed configuration edits and full window amendments.
// Current keeps the requested future policy; the pact accumulates every DNS
// restriction already in effect plus additions. Only a later explicit window
// amendment or pact end may loosen that enforced set.
func updateBoundDNS(c *rules.Configuration, raw []byte, now time.Time) error {
	if !Active(c.Pact, now) {
		return errors.New("active pact required")
	}
	var req editRequest
	if err := decode(raw, &req); err != nil {
		return err
	}
	if req.Revision != c.Revision {
		return errors.New("configuration conflict")
	}
	cap := c.Pact.Captain
	before := profile(c.Current, cap)
	next := clone(*c)
	next.Current = req.Current
	after := profile(next.Current, cap)
	if before == nil || after == nil {
		return errors.New("captain profile not found")
	}
	// No profile/device/app/schedule/settings change may ride along with DNS.
	left, right := clone(*c), clone(next)
	for _, doc := range []*rules.Configuration{&left, &right} {
		p := profile(doc.Current, cap)
		p.DNS, p.ClientID = "", ""
		p.Protections, p.Services = nil, nil
	}
	if !reflect.DeepEqual(left.Current, right.Current) {
		return errors.New("separate DNS updates from other configuration edits")
	}
	if after.ClientID != before.ClientID {
		return errors.New("DNS identity is bound by Ulysses pact")
	}
	snap := c.Pact.DNS
	domain := snap.Domain
	if domain == "" {
		domain = c.Current.Settings.DNSDomain
	}
	stableID := before.ClientID
	if stableID == "" {
		stableID = snap.ClientID
	}
	if stableID == "" && (before.DNS == "1" || snap.Enabled == "1") {
		return errors.New("bound DNS identity unavailable")
	}
	after.ClientID = stableID
	if err := assignDNSClientIDs(&next, domain); err != nil {
		return err
	}
	if err := validateDNSProfile(*after, domain); err != nil {
		return err
	}
	on := "0"
	if snap.Enabled == "1" || before.DNS == "1" || after.DNS == "1" {
		on = "1"
	}
	snap = rules.ConfigurationDNS{Enabled: on, ClientID: after.ClientID, Domain: domain,
		Protections: dnsUnion(snap.Protections, before.Protections, after.Protections),
		Services:    dnsUnion(snap.Services, before.Services, after.Services)}
	// Also validate the accumulated state, not just the future requested fields.
	if err := validateDNSProfile(rules.ConfigurationProfile{DNS: snap.Enabled, ClientID: snap.ClientID, Protections: snap.Protections, Services: snap.Services}, domain); err != nil {
		return err
	}
	next.Pact.DNS = snap
	frozen := profile(next.Pact.Frozen, cap)
	if frozen == nil {
		return errors.New("frozen captain profile not found")
	}
	frozen.DNS, frozen.ClientID = snap.Enabled, snap.ClientID
	frozen.Protections = append([]string{}, snap.Protections...)
	frozen.Services = append([]string{}, snap.Services...)
	if err := Validate(next); err != nil {
		return err
	}
	*c = next
	return nil
}
