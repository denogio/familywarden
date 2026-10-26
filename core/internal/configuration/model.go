package configuration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/rules"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var appPackage = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

func integer(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
func decimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}
func safeText(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr:
		return v.IsNil() || safeText(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !safeText(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !safeText(v.Index(i)) {
				return false
			}
		}
	case reflect.String:
		return len(v.String()) <= 4096 && !strings.ContainsAny(v.String(), "\x00\r\n")
	}
	return true
}

func Active(p *rules.ConfigurationPact, now time.Time) bool {
	return p != nil && (p.ReleaseAt == "" || p.ReleaseAt == "0" || integer(p.ReleaseAt) > now.Unix()) &&
		(p.Type == "ongoing" || (p.Type == "until" && integer(p.Until) > now.Unix()))
}

func minute(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h, e1 := strconv.Atoi(s[:2])
	m, e2 := strconv.Atoi(s[3:])
	return h*60 + m, e1 == nil && e2 == nil && h >= 0 && h <= 24 && m >= 0 && m < 60 && (h < 24 || m == 0)
}

func InWindow(p *rules.ConfigurationPact, now time.Time) bool {
	if p == nil || p.Type != "ongoing" {
		return false
	}
	s, ok := minute(p.WindowStart)
	e, ok2 := minute(p.WindowStop)
	return ok && ok2 && strings.ToLower(now.Weekday().String()[:3]) == p.WindowDay && now.Hour()*60+now.Minute() >= s && now.Hour()*60+now.Minute() < e
}

func profile(data rules.ConfigurationData, id string) *rules.ConfigurationProfile {
	for i := range data.Profiles {
		if data.Profiles[i].ID == id {
			return &data.Profiles[i]
		}
	}
	return nil
}

func clone(c rules.Configuration) rules.Configuration {
	b, _ := json.Marshal(c)
	var out rules.Configuration
	_ = json.Unmarshal(b, &out)
	return out
}

func Validate(c rules.Configuration) error {
	if !safeText(reflect.ValueOf(c)) {
		return errors.New("invalid configuration text")
	}
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	if err := validateData(c.Current); err != nil {
		return err
	}
	if p := c.Pact; p != nil {
		if len(p.Frozen.Profiles) != 1 || p.Frozen.Profiles[0].ID != p.Captain {
			return errors.New("invalid frozen profile set")
		}
		if p.DefaultPolicy != "allow" && p.DefaultPolicy != "block" {
			return errors.New("invalid frozen default policy")
		}
		if p.Type != "ongoing" && p.Type != "until" {
			return errors.New("invalid pact type")
		}
		if !identifier.MatchString(p.Captain) || profile(p.Frozen, p.Captain) == nil || profile(c.Current, p.Captain) == nil {
			return errors.New("missing frozen captain")
		}
		s, ok := minute(p.WindowStart)
		e, ok2 := minute(p.WindowStop)
		if p.Type == "ongoing" && (!ok || !ok2 || s >= e || !strings.Contains(" sun mon tue wed thu fri sat ", " "+p.WindowDay+" ")) {
			return errors.New("invalid frozen amendment window")
		}
		if err := validateData(p.Frozen); err != nil {
			return err
		}
		if p.Type == "until" && (!decimal(p.Until) || integer(p.Until) <= 0) {
			return errors.New("invalid pact deadline")
		}
		if !decimal(p.DelaySeconds) {
			return errors.New("invalid frozen waiting period")
		}
		if p.ReleaseAt != "" && (!decimal(p.ReleaseAt) || integer(p.ReleaseAt) < 0) {
			return errors.New("invalid release deadline")
		}
	}
	return nil
}

func validateData(d rules.ConfigurationData) error {
	if d.Settings.Enabled != "" && d.Settings.Enabled != "0" && d.Settings.Enabled != "1" {
		return errors.New("invalid enabled setting")
	}
	if d.Settings.DefaultPolicy != "" && d.Settings.DefaultPolicy != "allow" && d.Settings.DefaultPolicy != "block" {
		return errors.New("invalid default policy")
	}
	if d.Settings.DelayHours != "" && (!decimal(d.Settings.DelayHours) || integer(d.Settings.DelayHours) > (1<<63-1)/3600) {
		return errors.New("invalid waiting period")
	}
	ids := map[string]bool{"settings": true}
	profiles := map[string]bool{}
	for _, p := range d.Profiles {
		if !identifier.MatchString(p.ID) || ids[p.ID] {
			return errors.New("invalid or duplicate profile id")
		}
		ids[p.ID] = true
		profiles[p.ID] = true
		for _, bit := range []string{p.Enabled, p.BlockRouter, p.DNS, p.Apps, p.SuspendNew} {
			if bit != "" && bit != "0" && bit != "1" {
				return errors.New("invalid profile flag")
			}
		}
		if p.Policy != "" && p.Policy != "allow" && p.Policy != "block" && p.Policy != "scheduled" {
			return errors.New("invalid network policy")
		}
		for _, mac := range p.Devices {
			b, err := net.ParseMAC(mac)
			if err != nil || len(b) != 6 {
				return errors.New("invalid device MAC")
			}
		}
		if p.DNS == "1" && p.ClientID == "" {
			return errors.New("sync DNS before importing an enabled profile without a ClientID")
		}
		for _, v := range append([]string{p.Name, p.ClientID}, append(p.Protections, p.Services...)...) {
			if strings.ContainsAny(v, "\x00\r\n") {
				return errors.New("invalid configuration text")
			}
		}
	}
	if d.Settings.Captain != "" && !profiles[d.Settings.Captain] {
		return errors.New("captain profile not found")
	}
	for _, w := range append(append([]rules.ConfigurationWindow{}, d.Schedules...), d.Downtime...) {
		if !identifier.MatchString(w.ID) || ids[w.ID] || !profiles[w.Profile] {
			return errors.New("invalid window identity/reference")
		}
		ids[w.ID] = true
		_, s := minute(w.Start)
		_, e := minute(w.Stop)
		if !s || !e {
			return errors.New("invalid window time")
		}
		for _, day := range strings.Fields(w.Days) {
			if !strings.Contains(" sun mon tue wed thu fri sat ", " "+day+" ") {
				return errors.New("invalid window day")
			}
		}
	}
	for _, app := range d.Apps {
		if !identifier.MatchString(app.ID) || ids[app.ID] || !profiles[app.Profile] {
			return errors.New("invalid app identity/reference")
		}
		ids[app.ID] = true
	}
	return nil
}

func frozen(data rules.ConfigurationData, captain string) rules.ConfigurationData {
	out := rules.ConfigurationData{}
	for _, p := range data.Profiles {
		if p.ID == captain {
			out.Profiles = append(out.Profiles, p)
		}
	}
	for _, w := range data.Schedules {
		if w.Profile == captain {
			out.Schedules = append(out.Schedules, w)
		}
	}
	for _, w := range data.Downtime {
		if w.Profile == captain {
			out.Downtime = append(out.Downtime, w)
		}
	}
	for _, app := range data.Apps {
		if app.Profile == captain {
			out.Apps = append(out.Apps, app)
		}
	}
	return out
}

func snapshotDNS(c rules.Configuration, captain string) rules.ConfigurationDNS {
	p := profile(c.Current, captain)
	return rules.ConfigurationDNS{Enabled: p.DNS, ClientID: p.ClientID, Domain: c.Current.Settings.DNSDomain, Protections: p.Protections, Services: p.Services}
}

func ValidateEdit(old rules.Configuration, next *rules.Configuration, now time.Time) error {
	// Strict editing is separate from legacy compatibility normalization: never
	// turn an invalid newly submitted limit into a silently repaired policy.
	if !reflect.DeepEqual(old.Current.Apps, next.Current.Apps) {
		for _, app := range next.Current.Apps {
			if !appPackage.MatchString(app.Package) || len(app.Package) > 255 || (app.Blocked != "" && app.Blocked != "0" && app.Blocked != "1") || (app.Minutes != "" && (!decimal(app.Minutes) || integer(app.Minutes) > 1440)) {
				return errors.New("invalid application rule")
			}
		}
	}
	for _, p := range next.Current.Profiles {
		before := profile(old.Current, p.ID)
		for i, v := range []string{p.ScreenMinutes, p.ScreenWeekday, p.ScreenWeekend} {
			prior := ""
			if before != nil {
				prior = []string{before.ScreenMinutes, before.ScreenWeekday, before.ScreenWeekend}[i]
			}
			if v != prior && v != "" && (!decimal(v) || integer(v) > 1440) {
				return errors.New("invalid screen-time limit")
			}
		}
	}

	if Active(old.Pact, now) {
		if !reflect.DeepEqual(old.Current.Settings, next.Current.Settings) {
			return errors.New("settings are bound by Ulysses pact")
		}
		if !InWindow(old.Pact, now) && !reflect.DeepEqual(frozen(old.Current, old.Pact.Captain), frozen(next.Current, old.Pact.Captain)) {
			return errors.New("captain configuration is bound by Ulysses pact")
		}
	}
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
		if p.DNS == "1" && p.ClientID == "" {
			if next.Current.Settings.DNSDomain == "" {
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
	}
	if Active(old.Pact, now) && InWindow(old.Pact, now) {
		next.Pact = clone(old).Pact
		next.Pact.Frozen = frozen(next.Current, old.Pact.Captain)
		next.Pact.Frozen.LegacyPhones = old.Pact.Frozen.LegacyPhones
		next.Pact.DNS = snapshotDNS(*next, old.Pact.Captain)
	}
	return Validate(*next)
}

func others(data rules.ConfigurationData, captain string) rules.ConfigurationData {
	out := rules.ConfigurationData{Settings: data.Settings, LegacyPhones: data.LegacyPhones}
	for _, p := range data.Profiles {
		if p.ID != captain {
			out.Profiles = append(out.Profiles, p)
		}
	}
	for _, w := range data.Schedules {
		if w.Profile != captain {
			out.Schedules = append(out.Schedules, w)
		}
	}
	for _, w := range data.Downtime {
		if w.Profile != captain {
			out.Downtime = append(out.Downtime, w)
		}
	}
	for _, v := range data.Apps {
		if v.Profile != captain {
			out.Apps = append(out.Apps, v)
		}
	}
	return out
}

func PactOperation(method string, raw []byte, c *rules.Configuration, now time.Time) error {
	switch method {
	case "configuration_amend":
		if !Active(c.Pact, now) || !InWindow(c.Pact, now) {
			return errors.New("pact amendment window is closed")
		}
		var req struct {
			Revision string                  `json:"revision"`
			Current  rules.ConfigurationData `json:"current"`
		}
		if err := decode(raw, &req); err != nil {
			return err
		}
		if req.Revision != c.Revision {
			return errors.New("configuration conflict")
		}
		if !reflect.DeepEqual(others(c.Current, c.Pact.Captain), others(req.Current, c.Pact.Captain)) {
			return errors.New("separate pact amendments from other configuration edits")
		}
		next := clone(*c)
		next.Current = req.Current
		if err := ValidateEdit(*c, &next, now); err != nil {
			return err
		}
		*c = next
		return nil
	case "configuration_sail_request":
		if !Active(c.Pact, now) {
			return errors.New("no active pact")
		}
		if integer(c.Pact.ReleaseAt) <= 0 {
			c.Pact.ReleaseAt = strconv.FormatInt(now.Unix()+integer(c.Pact.DelaySeconds), 10)
		}
		return nil
	case "configuration_sail_cancel":
		if c.Pact != nil {
			c.Pact.ReleaseAt = ""
		}
		return nil
	case "configuration_pact":
		var req struct {
			Until int64  `json:"until"`
			Day   string `json:"window_day"`
			Start string `json:"window_start"`
			Stop  string `json:"window_stop"`
		}
		if err := decode(raw, &req); err != nil {
			return err
		}
		if req.Until != 0 && req.Until <= now.Unix() {
			return errors.New("pact deadline must be in the future")
		}
		if req.Day == "" {
			req.Day = "sun"
		}
		if req.Start == "" {
			req.Start = "10:00"
		}
		if req.Stop == "" {
			req.Stop = "11:00"
		}
		s, ok := minute(req.Start)
		e, ok2 := minute(req.Stop)
		if !ok || !ok2 || s >= e || !strings.Contains(" sun mon tue wed thu fri sat ", " "+req.Day+" ") {
			return errors.New("invalid amendment window")
		}
		if Active(c.Pact, now) {
			if c.Pact.Type == "until" && req.Until > integer(c.Pact.Until) {
				c.Pact.Until = strconv.FormatInt(req.Until, 10)
				return nil
			}
			if !InWindow(c.Pact, now) {
				return errors.New("the pact can only be extended")
			}
			phones := c.Pact.Frozen.LegacyPhones
			c.Pact.Frozen = frozen(c.Current, c.Pact.Captain)
			c.Pact.Frozen.LegacyPhones = phones
			c.Pact.DNS = snapshotDNS(*c, c.Pact.Captain)
			c.Pact.WindowDay, c.Pact.WindowStart, c.Pact.WindowStop = req.Day, req.Start, req.Stop
			if req.Until > 0 {
				c.Pact.Type = "until"
				c.Pact.Until = strconv.FormatInt(req.Until, 10)
			}
			return nil
		}
		captain := c.Current.Settings.Captain
		p := profile(c.Current, captain)
		if p == nil || p.Name == "" {
			return errors.New("no Ulysses profile chosen in settings")
		}
		delay := int64(24)
		if v, e := strconv.ParseInt(c.Current.Settings.DelayHours, 10, 64); e == nil && v >= 0 && v <= (1<<63-1)/3600 {
			delay = v
		}
		kind := "ongoing"
		if req.Until > 0 {
			kind = "until"
		}
		c.Pact = &rules.ConfigurationPact{Captain: captain, Type: kind, Until: strconv.FormatInt(req.Until, 10), WindowDay: req.Day, WindowStart: req.Start, WindowStop: req.Stop, DelaySeconds: strconv.FormatInt(delay*3600, 10), DefaultPolicy: c.Current.Settings.DefaultPolicy, Frozen: frozen(c.Current, captain)}
		if c.Pact.DefaultPolicy == "" {
			c.Pact.DefaultPolicy = "allow"
		}
		c.Pact.DNS = snapshotDNS(*c, captain)
		return nil
	default:
		return fmt.Errorf("unknown configuration method %s", method)
	}
}

// Effective is the stateless projection consumed by the OpenWrt adapter. IDs in
// the durable configured/frozen records remain untouched; renderers can choose
// private IDs for the derived schedules to avoid cross-snapshot collisions.
func Effective(c rules.Configuration, now time.Time) rules.ConfigurationData {
	c = clone(c)
	out := c.Current
	if !Active(c.Pact, now) {
		return out
	}
	cap := c.Pact.Captain
	snap := c.Pact.Frozen
	if InWindow(c.Pact, now) {
		snap = frozen(c.Current, cap)
	}
	macs := map[string]bool{}
	for _, p := range snap.Profiles {
		for _, m := range p.Devices {
			macs[strings.ToLower(m)] = true
		}
	}
	profiles := []rules.ConfigurationProfile{}
	for _, p := range out.Profiles {
		if p.ID == cap {
			continue
		}
		devices := []string{}
		for _, m := range p.Devices {
			if !macs[strings.ToLower(m)] {
				devices = append(devices, m)
			}
		}
		p.Devices = devices
		profiles = append(profiles, p)
	}
	for _, p := range snap.Profiles {
		if live := profile(out, p.ID); live != nil {
			p.Name = live.Name
		}
		profiles = append(profiles, p)
	}
	out.Profiles = profiles
	windows := []rules.ConfigurationWindow{}
	for _, w := range out.Schedules {
		if w.Profile != cap {
			windows = append(windows, w)
		}
	}
	out.Schedules = append(windows, snap.Schedules...)
	windows = nil
	for _, w := range out.Downtime {
		if w.Profile != cap {
			windows = append(windows, w)
		}
	}
	out.Downtime = append(windows, snap.Downtime...)
	apps := []rules.ConfigurationApp{}
	for _, v := range out.Apps {
		if v.Profile != cap {
			apps = append(apps, v)
		}
	}
	out.Apps = append(apps, snap.Apps...)
	out.Settings.Enabled = "1"
	out.Settings.DefaultPolicy = c.Pact.DefaultPolicy
	return out
}
