package identity

import (
	"slices"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
	"github.com/denogio/familywarden/core/internal/store"
)

// history is a bounded informational read, not a policy or administrator API.
// Authentication and current registry/context are locked by uploadLocked.
func (s *Service) history(d Device, raw []byte, ctx platform.Context) object {
	var m struct {
		Type    string `json:"type"`
		V       int    `json:"v"`
		Device  string `json:"device"`
		Profile string `json:"profile"`
		Day     string `json:"day"`
		Nonce   string `json:"nonce"`
		Created int64  `json:"created"`
	}
	if !decodeStrict(raw, &m, "type v device profile day nonce created", "") ||
		m.Type != "history" || m.V != 1 || m.Device != d.ID || !validNonce.MatchString(m.Nonce) || !validProfile.MatchString(m.Profile) {
		return refuse("malformed")
	}
	now := s.Now().In(routerZone(ctx))
	if m.Created <= 0 || m.Created > maxEpoch || now.Sub(time.Unix(m.Created, 0)) >= protocol.ClockSkew || time.Unix(m.Created, 0).Sub(now) > protocol.ClockSkew {
		return refuse("request_expired")
	}
	if m.Profile != d.Profile && !slices.Contains(effectiveApproves(d, ctx), m.Profile) {
		return refuse("not_parent")
	}
	if p, ok := ctx.Profiles[m.Profile]; !ok || p.AppsRev == "" {
		return refuse("app_control_off")
	}
	day, err := time.ParseInLocation(time.DateOnly, m.Day, now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err != nil || day.Format(time.DateOnly) != m.Day || day.After(today) || day.Before(today.AddDate(0, 0, -(store.Retention-1))) {
		return refuse("day_out_of_range")
	}
	if s.Usage == nil {
		return refuse("unavailable")
	}
	total, devices, apps, _, err := s.dayUsage(m.Profile, m.Day, 20)
	if err != nil {
		return refuse("unavailable")
	}
	if devices == nil {
		devices = []Reply{}
	}
	return object{{"profile_id", m.Profile}, {"day", m.Day}, {"reported", len(devices) > 0},
		{"total", total}, {"devices", devices}, {"apps", nonNilApps(apps)}}
}
