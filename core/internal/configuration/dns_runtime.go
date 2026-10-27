package configuration

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/denogio/familywarden/core/internal/agh"
	"github.com/denogio/familywarden/core/internal/platform"
)

const dnsDocument = "dns.json"

type dnsState struct {
	Version   int      `json:"version"`
	Owned     []string `json:"owned"`
	Acks      []string `json:"acks"`
	Attempted string   `json:"attempted"`
	Time      int64    `json:"time"`
	Error     string   `json:"error"`
}

func parseDNSAck(line string) (id string, row map[string]any, err error) {
	f := strings.Fields(line)
	if len(f) != 5 || !identifier.MatchString(f[0]) || !agh.ValidClientID(f[1]) || !validDNSHostname(f[1]+"."+f[2]) {
		return "", nil, errors.New("invalid DNS acknowledgement")
	}
	list := func(s string) []string {
		if s == "-" {
			return []string{}
		}
		return strings.Split(s, ",")
	}
	protections, services := list(f[3]), list(f[4])
	for _, p := range protections {
		switch p {
		case "filter_lists", "safe_browsing", "parental", "safe_search":
		default:
			return "", nil, errors.New("invalid DNS acknowledgement protection")
		}
	}
	for _, s := range services {
		if !dnsService.MatchString(s) {
			return "", nil, errors.New("invalid DNS acknowledgement service")
		}
	}
	return f[0], map[string]any{"hostname": f[1] + "." + f[2], "protections": protections, "services": services}, nil
}

// DNS runtime is independent of editable configuration: confirmation rollback
// cannot undo verified remote effects or ownership. Import once, under the
// adapter's legacy-writer lock, before publishing any owned acknowledgement.
func (a *Authority) loadDNS(native platform.DNSRuntime) (dnsState, error) {
	var s dnsState
	found, err := a.store.ReadDocument(dnsDocument, &s)
	if err != nil {
		return s, err
	}
	if !found {
		s = dnsState{Version: 1, Owned: native.LegacyOwned, Acks: native.LegacyAcks}
	}
	if s.Version != 1 || s.Time < 0 {
		return s, errors.New("unsupported DNS runtime state")
	}
	seen := map[string]bool{}
	for _, line := range s.Acks {
		id, _, err := parseDNSAck(line)
		if err != nil {
			return s, err
		}
		if seen[id] {
			return s, errors.New("duplicate DNS acknowledgement")
		}
		seen[id] = true
	}
	for _, id := range s.Owned {
		if !agh.ValidClientID(id) {
			return s, errors.New("invalid owned DNS ClientID")
		}
	}
	if !found {
		err = a.store.WriteDocument(dnsDocument, s)
	}
	return s, err
}

func (a *Authority) dnsPlatform() (platform.DNSPlatform, error) {
	p, ok := a.Adapter.(platform.DNSPlatform)
	if !ok {
		return nil, errors.New("DNS platform unavailable")
	}
	return p, nil
}

// Called with the authority mutex held. The platform callback holds only its
// native DNS lock: no configuration/network lock or recursive core socket call.
// ponytail: serial reconciliation under the authority mutex; split execution
// from publication only if real AGH latency requires concurrent policy service.
func (a *Authority) dnsAPI(method string, raw []byte, d document) (map[string]any, error) {
	p, err := a.dnsPlatform()
	if err != nil {
		return nil, err
	}
	if method == "configuration_dns_auth" {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		if len(req.Username) > 4096 || len(req.Password) > 4096 || req.Username == "" || req.Password == "" || strings.ContainsAny(req.Username, ":\x00\r\n") || strings.ContainsAny(req.Password, "\x00\r\n") {
			return nil, errors.New("invalid credentials")
		}
		if err := p.SetDNSCredentials(req.Username, req.Password); err != nil {
			return nil, errors.New("could not store DNS credentials")
		}
		return map[string]any{"status": "ok"}, nil
	}
	var reply map[string]any
	err = p.DNSTransaction(func(native platform.DNSRuntime) error {
		s, err := a.loadDNS(native)
		if err != nil {
			return err
		}
		domain, _, profiles := DNS(d.Configuration, a.Now())
		input := agh.Input{URL: d.Configuration.Current.Settings.ResolverURL, Domain: domain, Profiles: profiles, Auth: native.Auth, Owned: s.Owned, Acks: s.Acks}
		for _, profile := range d.Configuration.Current.Profiles {
			if profile.ClientID != "" {
				input.Taken = append(input.Taken, profile.ClientID)
			}
		}
		// No credentials are persisted in this fingerprint or returned to callers.
		desired, _ := json.Marshal(struct {
			URL, Domain, CredentialHash string
			Profiles                    []agh.Profile
		}{input.URL, domain, fmt.Sprintf("%x", sha256.Sum256([]byte(native.Auth))), profiles})
		fingerprint := fmt.Sprintf("%x", sha256.Sum256(desired))
		pending := s.Attempted != fingerprint
		if method == "configuration_dns_services" {
			if !agh.ValidEndpoint(input.URL) || native.Auth == "" {
				return errors.New("AdGuard Home not configured")
			}
			services, err := agh.Services(input.URL, native.Auth, agh.NewHTTPClient())
			if err != nil {
				return errors.New("AdGuard Home unreachable")
			}
			reply = map[string]any{"services": services}
			return nil
		}
		now := a.Now().Unix()
		sync := method == "configuration_dns_sync" || (method == "configuration_dns_tick" && (pending || (s.Error != "" && now-s.Time >= 300)))
		if sync {
			next := s
			next.Attempted, next.Time, next.Error = fingerprint, now, ""
			switch {
			case input.URL == "" || domain == "":
				if len(profiles) > 0 {
					next.Error = "DNS domain or AdGuard Home URL not set"
				} else {
					next.Acks = []string{}
				}
			case !agh.ValidEndpoint(input.URL):
				next.Error = "invalid AdGuard Home URL"
			case native.Auth == "":
				next.Error = "AdGuard Home credentials not set"
			default:
				for _, profile := range profiles {
					if !agh.ValidClientID(profile.ClientID) {
						return errors.New("core-owned profile has no ClientID")
					}
				}
				result := agh.Sync(input, agh.NewHTTPClient())
				if len(result.Allocated) > 0 {
					return errors.New("unexpected owned DNS identity allocation")
				}
				next.Error = result.Error
				if result.Complete {
					next.Acks = result.Ack
					if result.WriteOwned {
						next.Owned = result.Owned
					}
				} else if next.Error == "" {
					next.Error = "DNS reconciliation incomplete"
				}
			}
			for _, line := range next.Acks {
				if _, _, err := parseDNSAck(line); err != nil {
					return err
				}
			}
			if err := a.store.WriteDocument(dnsDocument, next); err != nil {
				return err
			}
			s = next
			pending = false
		}
		rows := map[string]any{}
		for _, line := range s.Acks {
			id, _, err := parseDNSAck(line)
			if err != nil {
				return err
			}
			f := strings.Fields(line)
			rows[id] = map[string]any{"client_id": f[1], "hostname": f[1] + "." + f[2], "protections": f[3], "services": f[4]}
		}
		reply = map[string]any{"format": "dns-state-v1", "credentials": native.Auth != "", "time": s.Time, "error": s.Error, "pending": pending, "profiles": rows}
		return nil
	})
	return reply, err
}

func (a *Authority) verifiedDNS(profile string) (json.RawMessage, error) {
	p, err := a.dnsPlatform()
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = p.DNSTransaction(func(native platform.DNSRuntime) error {
		state, err := a.loadDNS(native)
		if err != nil {
			return err
		}
		for _, line := range state.Acks {
			id, row, err := parseDNSAck(line)
			if err != nil {
				return err
			}
			if id == profile {
				raw, err = json.Marshal(row)
				return err
			}
		}
		return nil
	})
	return raw, err
}
