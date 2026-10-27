package configuration

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

const networkFormat = "network-actions-v1"

var errNetworkBound = errors.New("bound by Ulysses pact")

func validOverride(s platform.NetworkOverride) bool {
	return (s.Mode == "none" || s.Mode == "allow" || s.Mode == "block") && s.Expires >= 0
}

// PactNetworkOverride revokes every pre-pact allow for the bound captain,
// including permanent and timed permits. Blocks and other profiles are retained.
// Apply this during every configuration reconciliation, not just pact creation,
// so imported/restarted pacts cannot resurrect an earlier permission.
func PactNetworkOverride(c rules.Configuration, id string, current platform.NetworkOverride, now time.Time) (platform.NetworkOverride, error) {
	if !Active(c.Pact, now) || c.Pact.Captain != id {
		return current, nil
	}
	if !validOverride(current) {
		return current, errors.New("invalid network state")
	}
	if current.Mode == "allow" {
		return platform.NetworkOverride{Mode: "none"}, nil
	}
	return current, nil
}

// decideNetwork shares manual actions and approved-grant repair precedence.
// Grant repair never replaces an explicit block, shortens an allow or opens a pact.
// A parent's manual ticket may replace a block and extends an active timed allow.
func decideNetwork(current platform.NetworkOverride, action string, now, value int64, bound bool) (platform.NetworkOverride, bool, error) {
	if !validOverride(current) {
		return current, false, errors.New("invalid network state")
	}
	if current.Expires > 0 && current.Expires <= now {
		current = platform.NetworkOverride{Mode: "none"}
	}
	if action == "grant" {
		if bound || value <= now || current.Mode == "block" {
			return current, false, nil
		}
		if current.Mode == "allow" && (current.Expires == 0 || current.Expires >= value) {
			return current, true, nil
		}
		return platform.NetworkOverride{Mode: "allow", Expires: value}, true, nil
	}
	if bound && (action == "allow" || action == "ticket") {
		return current, false, errNetworkBound
	}
	switch action {
	case "block", "allow":
		return platform.NetworkOverride{Mode: action}, true, nil
	case "reset":
		return platform.NetworkOverride{Mode: "none"}, true, nil
	case "ticket":
		if value < 1 || value > math.MaxInt32 {
			return current, false, errors.New("minutes must be a positive integer")
		}
		base := now
		if current.Mode == "allow" && current.Expires > now {
			base = current.Expires
		}
		if base > math.MaxInt64-value*60 {
			return current, false, errors.New("ticket deadline overflow")
		}
		return platform.NetworkOverride{Mode: "allow", Expires: base + value*60}, true, nil
	}
	return current, false, errors.New("unknown network action")
}

// Called with the authority mutex held. The backend owns RAM storage/locks and
// native application, not product eligibility. No durable configuration change.
func (a *Authority) applyNetwork(d document, profileID, action string, value int64) (platform.NetworkOverride, bool, error) {
	if d.Pending != nil {
		return platform.NetworkOverride{}, false, errors.New("confirmation pending")
	}
	var next platform.NetworkOverride
	opened := false
	err := a.backend.NetworkTransaction(profileID, func(facts platform.Context, overrides map[string]platform.NetworkOverride, apply func(platform.NetworkOverride, platform.NetworkIntent, platform.NetworkIntent) error) error {
		if facts.NetworkFormat != networkFormat {
			return errors.New("incompatible network adapter: upgrade core and backend together")
		}
		now := a.at(facts)
		p := profile(Effective(d.Configuration, now), profileID)
		if p == nil || p.Name == "" {
			return errors.New("profile not found")
		}
		bound := Active(d.Configuration.Pact, now) && d.Configuration.Pact.Captain == profileID
		current := normalizeOverride(overrides[profileID])
		var err error
		next, opened, err = decideNetwork(current, action, now.Unix(), value, bound)
		if err != nil || !opened {
			return err
		}
		if action == "grant" && next == current {
			return nil // Existing permanent/longer permission, as in the legacy bridge.
		}
		post := make(map[string]platform.NetworkOverride, len(overrides)+1)
		for id, row := range overrides {
			post[id] = row
		}
		post[profileID] = next
		intent, err := BuildNetworkIntent(d.Configuration, now, post)
		if err != nil {
			return err
		}
		restore, err := BuildNetworkIntent(d.Configuration, now, overrides)
		if err != nil {
			return err
		}
		if err := apply(next, intent, restore); err != nil {
			a.blocked = err // Uncertain enforcement must not yield a successful policy.
			return err
		}
		a.acknowledgeNetwork(intent)
		return nil
	})
	return next, opened && err == nil, err
}

func (a *Authority) changeNetwork(d document, method string, raw []byte) (map[string]any, error) {
	var req struct {
		Profile string          `json:"profile"`
		Minutes json.RawMessage `json:"minutes"`
	}
	if err := decode(raw, &req); err != nil {
		return nil, err
	}
	if !identifier.MatchString(req.Profile) {
		return nil, errors.New("invalid profile")
	}
	action := strings.TrimPrefix(method, "configuration_")
	minutes := int64(30)
	if action == "ticket" && req.Minutes != nil {
		s := string(req.Minutes)
		if len(s) > 0 && s[0] == '"' {
			if err := json.Unmarshal(req.Minutes, &s); err != nil {
				return nil, err
			}
		}
		var err error
		minutes, err = strconv.ParseInt(s, 10, 64)
		if !decimal(s) || err != nil || minutes < 1 || minutes > math.MaxInt32 {
			return nil, errors.New("minutes must be a positive integer")
		}
	}
	next, _, err := a.applyNetwork(d, req.Profile, action, minutes)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"status": "ok"}
	if action == "ticket" {
		// Preserve the absolute, offset-bearing timestamp returned by the old API.
		result["profile"] = req.Profile
		result["expires"] = time.Unix(next.Expires, 0).UTC().Format("2006-01-02T15:04:05-07:00")
	}
	return result, nil
}

// EnsureTicket overrides the embedded legacy bridge only after explicit import.
// Identity invokes it after releasing its own mutation/context locks.
func (a *Authority) EnsureTicket(profileID string, until int64) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refresh(); err != nil {
		return false, err
	}
	d, found, err := a.load()
	if err != nil {
		return false, err
	}
	if !found {
		return a.Adapter.EnsureTicket(profileID, until)
	}
	if !identifier.MatchString(profileID) {
		return false, errors.New("invalid profile")
	}
	_, opened, err := a.applyNetwork(d, profileID, "grant", until)
	return opened, err
}
