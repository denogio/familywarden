package configuration

import (
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
