// Package agh reconciles FamilyWarden's AdGuard Home clients. It is router
// agnostic: callers supply the desired DNS profiles, the AGH endpoint and the
// ClientIDs FamilyWarden itself added to allowed_clients, and store the result.
//
// Ownership boundary: only clients named "familywarden-<ClientID>" and the
// owned allowed_clients entries are ever changed. /control/access/set replaces
// the whole list, so it is re-read just before writing and verified afterwards;
// an empty allowed list (everyone allowed) is never touched. A profile's DNS is
// acknowledged only after AGH accepted its client.
package agh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const Prefix = "familywarden-"

// ValidEndpoint accepts a base HTTP(S) URL, never embedded credentials, query
// parameters or fragments. Credentials are supplied separately and privately.
func ValidEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// Profile is one profile whose phones use strict DNS. ClientID may be empty
// (legacy routers): a free one is derived from Name and reported in Allocated.
type Profile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ClientID    string   `json:"client_id"`
	Protections []string `json:"protections"`
	Services    []string `json:"services"`
}

type Input struct {
	URL      string    `json:"url"`
	Auth     string    `json:"auth"` // base64("user:password"); AGH sends no auth challenge
	Domain   string    `json:"domain"`
	Owned    []string  `json:"owned"`
	Acks     []string  `json:"acks"`  // last acknowledged lines; a profile that fails keeps its own
	Taken    []string  `json:"taken"` // ClientIDs stored for any profile
	Profiles []Profile `json:"profiles"`
}

// Result: when Complete, Ack replaces the acknowledgement file (and Owned the
// owned list if WriteOwned). Otherwise both stay as they were. Error is the
// last problem, "" when everything is in sync.
type Result struct {
	Error      string       `json:"error"`
	Complete   bool         `json:"complete"`
	Ack        []string     `json:"ack"`
	WriteOwned bool         `json:"write_owned"`
	Owned      []string     `json:"owned"`
	Allocated  []Allocation `json:"allocated"`
}

// Allocation is a ClientID newly derived for a profile without one.
type Allocation struct {
	Profile  string `json:"profile"`
	ClientID string `json:"client_id"`
}

type client struct {
	Name                     string          `json:"name"`
	IDs                      []string        `json:"ids"`
	UseGlobalSettings        *bool           `json:"use_global_settings"`
	FilteringEnabled         *bool           `json:"filtering_enabled"`
	SafeBrowsingEnabled      *bool           `json:"safebrowsing_enabled"`
	ParentalEnabled          *bool           `json:"parental_enabled"`
	SafeSearch               map[string]bool `json:"safe_search"`
	UseGlobalBlockedServices *bool           `json:"use_global_blocked_services"`
	BlockedServices          []string        `json:"blocked_services"`
	Upstreams                []string        `json:"upstreams"`
	Tags                     []string        `json:"tags"`
	IgnoreQuerylog           bool            `json:"ignore_querylog"`
	IgnoreStatistics         bool            `json:"ignore_statistics"`
}

type access struct {
	Allowed    []string `json:"allowed_clients"`
	Disallowed []string `json:"disallowed_clients"`
	Blocked    []string `json:"blocked_hosts"`
}

// Missing access-list fields are not an empty list. Refuse incomplete replies
// rather than erase foreign entries when posting the replacement document.
func (a *access) UnmarshalJSON(data []byte) error {
	var v struct {
		Allowed    *[]string `json:"allowed_clients"`
		Disallowed *[]string `json:"disallowed_clients"`
		Blocked    *[]string `json:"blocked_hosts"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Allowed == nil || v.Disallowed == nil || v.Blocked == nil {
		return fmt.Errorf("incomplete AdGuard Home access list")
	}
	*a = access{*v.Allowed, *v.Disallowed, *v.Blocked}
	return nil
}

type api struct {
	url, auth string
	http      *http.Client
}

func (a api) do(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, a.url+"/control/"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+a.auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	const maxReply = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply+1))
	if err != nil {
		return err
	}
	if len(data) > maxReply {
		return fmt.Errorf("oversized AdGuard Home reply")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

var slugRun = regexp.MustCompile(`[^a-z0-9]+`)
var validID = regexp.MustCompile(`^[a-z0-9-]+$`)

// Slug turns a display name into a DNS-safe ClientID base (at most 40 chars).
func Slug(name string) string {
	// ASCII-only lowercasing plus these letters, exactly as the legacy shell did.
	s := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, name)
	s = strings.NewReplacer("æ", "ae", "Æ", "ae", "ø", "oe", "Ø", "oe", "å", "aa", "Å", "aa", "ä", "ae", "Ä", "ae",
		"ö", "oe", "Ö", "oe", "ü", "ue", "Ü", "ue", "é", "e", "è", "e").Replace(s)
	s = strings.Trim(slugRun.ReplaceAllString(s, "-"), "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.TrimRight(s, "-")
}

func ValidClientID(id string) bool {
	return id != "" && len(id) <= 63 && validID.MatchString(id) && id[0] != '-' && id[len(id)-1] != '-'
}

func unique(v []string) []string {
	out := []string{}
	for _, x := range v {
		if x != "" && !contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func contains(v []string, x string) bool {
	for _, y := range v {
		if y == x {
			return true
		}
	}
	return false
}

var serviceID = regexp.MustCompile(`^[a-z0-9_]+$`)

func fields(p Profile, id string) map[string]any {
	ss := contains(p.Protections, "safe_search")
	safe := map[string]bool{}
	for _, k := range []string{"enabled", "bing", "duckduckgo", "ecosia", "google", "pixabay", "yandex", "youtube"} {
		safe[k] = ss
	}
	blocked := []string{}
	for _, s := range p.Services {
		if serviceID.MatchString(s) {
			blocked = append(blocked, s)
		}
	}
	return map[string]any{
		"name": Prefix + id, "ids": []string{id}, "use_global_settings": false,
		"filtering_enabled": contains(p.Protections, "filter_lists"), "safebrowsing_enabled": contains(p.Protections, "safe_browsing"),
		"parental_enabled": contains(p.Protections, "parental"), "safe_search": safe,
		"use_global_blocked_services": false, "blocked_services": blocked,
		"upstreams": []string{}, "tags": []string{}, "ignore_querylog": false, "ignore_statistics": false,
	}
}

// AckLine is "<profile> <clientid> <domain> <protections> <services>".
func AckLine(p Profile, id, domain string) string {
	prot := []string{}
	for _, s := range []string{"filter_lists", "safe_browsing", "parental", "safe_search"} {
		if contains(p.Protections, s) {
			prot = append(prot, s)
		}
	}
	join := func(v []string) string {
		if len(v) == 0 {
			return "-"
		}
		return strings.Join(v, ",")
	}
	return fmt.Sprintf("%s %s %s %s %s", p.ID, id, domain, join(prot), join(p.Services))
}

func sorted(v []string) []string {
	out := append([]string{}, v...)
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func previousAck(lines []string, profile string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, profile+" ") {
			return line
		}
	}
	return ""
}

func clientMatches(c client, p Profile) bool {
	if c.Name != Prefix+p.ClientID || len(c.IDs) != 1 || c.IDs[0] != p.ClientID {
		return false
	}
	for _, flag := range []struct {
		got  *bool
		want bool
	}{
		{c.UseGlobalSettings, false}, {c.FilteringEnabled, contains(p.Protections, "filter_lists")},
		{c.SafeBrowsingEnabled, contains(p.Protections, "safe_browsing")}, {c.ParentalEnabled, contains(p.Protections, "parental")},
		{c.UseGlobalBlockedServices, false},
	} {
		if flag.got == nil || *flag.got != flag.want {
			return false
		}
	}
	wanted := fields(p, p.ClientID)
	for key, value := range wanted["safe_search"].(map[string]bool) {
		if c.SafeSearch[key] != value {
			return false
		}
	}
	return equal(sorted(c.BlockedServices), sorted(wanted["blocked_services"].([]string))) &&
		len(c.Upstreams) == 0 && len(c.Tags) == 0 && !c.IgnoreQuerylog && !c.IgnoreStatistics
}

// Sync brings AdGuard Home in line with in.Profiles.
func Sync(in Input, hc *http.Client) Result {
	res := Result{Allocated: []Allocation{}, Ack: []string{}}
	a := api{strings.TrimRight(in.URL, "/"), in.Auth, hc}
	var clients struct {
		Clients []client `json:"clients"`
	}
	var acc access
	if a.do("GET", "clients", nil, &clients) != nil || clients.Clients == nil || a.do("GET", "access/list", nil, &acc) != nil {
		res.Error = "AdGuard Home unreachable or credentials rejected"
		return res
	}
	taken := append([]string{}, in.Taken...)
	for _, c := range clients.Clients {
		taken = append(taken, c.IDs...)
	}
	taken = append(append(taken, acc.Allowed...), acc.Disallowed...)

	var keep, ids []string
	var applied []Profile
	for _, p := range in.Profiles {
		p.Protections, p.Services = unique(p.Protections), unique(p.Services)
		id := p.ClientID
		if id == "" {
			base := Slug(p.Name)
			if base == "" {
				base = "profile"
			}
			id = base
			for n := 2; contains(taken, id); n++ {
				id = fmt.Sprintf("%s-%d", base, n)
			}
			taken = append(taken, id)
			res.Allocated = append(res.Allocated, Allocation{p.ID, id})
		}
		if !ValidClientID(id) {
			res.Error = "invalid ClientID for " + p.ID
			continue
		}
		keep = append(keep, id)
		name := Prefix + id
		keepOld := func() {
			if line := previousAck(in.Acks, p.ID); line != "" {
				res.Ack = append(res.Ack, line)
			}
		}
		exists, foreign := false, false
		for _, c := range clients.Clients {
			if c.Name == name {
				exists = true
			} else if contains(c.IDs, id) {
				foreign = true
			}
		}
		if foreign {
			res.Error = "ClientID " + id + " is used by another AdGuard Home client"
			keepOld()
			continue
		}
		if exists {
			if a.do("POST", "clients/update", map[string]any{"name": name, "data": fields(p, id)}, nil) != nil {
				res.Error = "updating " + name + " failed"
				keepOld()
				continue
			}
		} else if a.do("POST", "clients/add", fields(p, id), nil) != nil {
			res.Error = "adding " + name + " failed"
			keepOld()
			continue
		}
		ids = append(ids, id)
		p.ClientID = id
		applied = append(applied, p)
		res.Ack = append(res.Ack, AckLine(p, id, in.Domain))
	}

	// A successful POST is not proof that the requested filtering settings
	// were installed. Read once after all writes, before acknowledging or
	// adding their IDs to a restrictive access list.
	if len(applied) > 0 {
		var after struct {
			Clients []client `json:"clients"`
		}
		if a.do("GET", "clients", nil, &after) != nil || after.Clients == nil {
			res.Error = "AdGuard Home clients unreadable after update"
			return res
		}
		bad := map[string]string{}
		badIDs := map[string]bool{}
		for _, p := range applied {
			matches := 0
			for _, c := range after.Clients {
				if clientMatches(c, p) {
					matches++
				}
			}
			if matches != 1 {
				res.Error = "verifying " + Prefix + p.ClientID + " failed"
				bad[p.ID] = previousAck(in.Acks, p.ID)
				badIDs[p.ClientID] = true
			}
		}
		if len(bad) > 0 {
			acks := []string{}
			for _, line := range res.Ack {
				if old, failed := bad[strings.SplitN(line, " ", 2)[0]]; failed {
					if old != "" {
						acks = append(acks, old)
					}
				} else {
					acks = append(acks, line)
				}
			}
			res.Ack = acks
			verified := []string{}
			for _, id := range ids {
				if !badIDs[id] {
					verified = append(verified, id)
				}
			}
			ids = verified
		}
	}

	// Our clients that no longer belong to a DNS profile (even a failed update keeps its client).
	for _, c := range clients.Clients {
		if strings.HasPrefix(c.Name, Prefix) && !contains(keep, strings.TrimPrefix(c.Name, Prefix)) {
			if a.do("POST", "clients/delete", map[string]string{"name": c.Name}, nil) != nil {
				res.Error = "deleting " + c.Name + " failed"
			}
		}
	}

	// allowed_clients: add our missing ids, drop ids we own that are no longer used.
	if len(acc.Allowed) > 0 {
		if a.do("GET", "access/list", nil, &acc) != nil {
			res.Error = "AdGuard Home access list unreadable"
			return res
		}
		// An administrator may have opened the list since the first read.
		// Do not turn that freshly empty list back into a restrictive one.
		if len(acc.Allowed) == 0 {
			res.Complete = true
			return res
		}
		changed := false
		next, owned := []string{}, []string{}
		for _, x := range acc.Allowed {
			if contains(in.Owned, x) && !contains(keep, x) {
				changed = true
				continue
			}
			next = append(next, x)
			if contains(in.Owned, x) {
				owned = append(owned, x)
			}
		}
		for _, x := range ids {
			if !contains(acc.Allowed, x) {
				next, owned, changed = append(next, x), append(owned, x), true
			}
		}
		if changed {
			want := access{next, nonNil(acc.Disallowed), nonNil(acc.Blocked)}
			if a.do("POST", "access/set", want, nil) != nil {
				res.Error = "updating AdGuard Home allowed clients failed"
				return res
			}
			var after access
			if a.do("GET", "access/list", nil, &after) != nil {
				res.Error = "access list not readable after update"
				return res
			}
			if !equal(sorted(after.Allowed), sorted(next)) || !equal(after.Disallowed, acc.Disallowed) || !equal(after.Blocked, acc.Blocked) {
				res.Error = "AdGuard Home access list changed unexpectedly; check it manually"
				return res
			}
		}
		res.WriteOwned, res.Owned = true, owned
	}
	res.Complete = true
	return res
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

type Service struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Services lists AdGuard Home's blockable services for the service picker.
func Services(url, auth string, hc *http.Client) ([]Service, error) {
	var all struct {
		Services []Service `json:"blocked_services"`
	}
	if err := (api{strings.TrimRight(url, "/"), auth, hc}).do("GET", "blocked_services/all", nil, &all); err != nil {
		return nil, err
	}
	if all.Services == nil {
		return nil, fmt.Errorf("incomplete AdGuard Home services catalogue")
	}
	return all.Services, nil
}
