package configuration

import (
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
	}
	return nil
}
