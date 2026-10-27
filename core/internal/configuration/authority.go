// Package configuration owns the household configuration and its recovery journal.
// Activation is explicit; an absent document leaves the legacy owner in place.
package configuration

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

const documentName = "configuration.json"

type Storage interface {
	ReadDocument(string, any) (bool, error)
	WriteDocument(string, any) error
}

// Backend owns platform locking, import and projection, not product decisions.
// apply runs synchronously inside the same transaction as the callback, with
// ticket-before-state lock order and no callback into the core socket.
type Backend interface {
	ConfigurationTransaction(func(rules.Configuration, platform.Context, func(rules.Configuration) error) error) error
	// NetworkTransaction serializes a volatile override snapshot through apply.
	// A failed apply attempts to restore previous state/enforcement and reports
	// failure (including restoration errors), never success.
	NetworkTransaction(string, func(platform.Context, platform.NetworkOverride, func(platform.NetworkOverride) error) error) error
	// NetworkSnapshot serializes a read of the platform's volatile network state
	// and facts with the platform's writers. The callback receives one consistent
	// snapshot and must not call the adapter again.
	NetworkSnapshot(func(platform.Context, map[string]platform.NetworkOverride) error) error
}

type pendingChange struct {
	Before rules.Configuration `json:"before"`
	Token  string              `json:"token"`
	Until  int64               `json:"until"`
}

type document struct {
	Version       int                 `json:"version"`
	Configuration rules.Configuration `json:"configuration"`
	Pending       *pendingChange      `json:"pending,omitempty"`
	NeedsApply    bool                `json:"needs_apply,omitempty"`
	Bonus         *bonusState         `json:"bonus,omitempty"`
}

type Authority struct {
	platform.Adapter
	backend Backend
	store   Storage
	mu      sync.Mutex
	Now     func() time.Time
	timer   *time.Timer
	blocked error // uncertain persistence/apply requires restart reconciliation, never legacy fallback
}

func New(ad platform.Adapter, backend Backend, st Storage) (*Authority, error) {
	a := &Authority{Adapter: ad, backend: backend, store: st, Now: time.Now}
	a.mu.Lock()
	defer a.mu.Unlock()
	d, found, err := a.load()
	if err != nil || !found {
		return a, err
	}
	// A lost browser/session is never confirmation. Recover the durable prior
	// state before serving policies after a restart, even before its deadline.
	if d.Pending != nil {
		d.Configuration = d.Pending.Before
		d.Pending = nil
	}
	if err := a.backend.ConfigurationTransaction(func(_ rules.Configuration, facts platform.Context, apply func(rules.Configuration) error) error {
		if facts.BonusFormat != bonusFormat {
			return errors.New("incompatible bonus adapter: upgrade core and backend together")
		}
		if d.Bonus == nil {
			var err error
			d.Bonus, err = importBonus(facts)
			if err != nil {
				return err
			}
		}
		if err := apply(d.Configuration); err != nil {
			return err
		}
		d.NeedsApply = false
		return a.save(d)
	}); err != nil {
		return nil, err
	}
	return a, nil
}

// A rollback target is independently sealed, not covered by the candidate's
// revision. Validate both before any recovery can write to the platform/store.
func validateSealed(c rules.Configuration) error {
	if err := Validate(c); err != nil {
		return err
	}
	rev := c.Revision
	if err := c.Seal(); err != nil || rev != c.Revision {
		return errors.New("invalid household revision")
	}
	return nil
}

func (a *Authority) load() (document, bool, error) {
	var d document
	var raw json.RawMessage
	found, err := a.store.ReadDocument(documentName, &raw)
	if err != nil || !found {
		return d, found, err
	}
	if err := decode(raw, &d); err != nil {
		return d, true, err
	}
	if d.Version != 1 {
		return d, true, errors.New("unsupported household document")
	}
	if err := validateSealed(d.Configuration); err != nil {
		return d, true, err
	}
	if d.Bonus != nil {
		if err := d.Bonus.validate(); err != nil {
			return d, true, err
		}
	}
	if d.Pending != nil {
		if err := validateSealed(d.Pending.Before); err != nil {
			return d, true, err
		}
		if len(d.Pending.Token) != 64 || d.Pending.Until <= 0 {
			return d, true, errors.New("invalid recovery journal")
		}
	}
	return d, true, nil
}

func (a *Authority) save(d document) error {
	if err := d.Configuration.Seal(); err != nil {
		return err
	}
	if err := a.store.WriteDocument(documentName, d); err != nil {
		return err
	}
	var readback document
	found, err := a.store.ReadDocument(documentName, &readback)
	want, _ := json.Marshal(d)
	got, _ := json.Marshal(readback)
	if err != nil || !found || !bytes.Equal(want, got) {
		return errors.New("configuration read-back failed")
	}
	return nil
}

func (a *Authority) refresh() error {
	if a.blocked != nil {
		return a.blocked
	}
	d, found, err := a.load()
	if err != nil || !found {
		return err
	}
	if d.NeedsApply {
		return errors.New("configuration recovery required")
	}
	if d.Bonus == nil {
		return errors.New("bonus recovery required")
	}
	changed := false
	if d.Pending != nil && a.Now().Unix() >= d.Pending.Until {
		d.Configuration = d.Pending.Before
		d.Pending = nil
		changed = true
	}
	if p := d.Configuration.Pact; p != nil && !Active(p, a.Now()) {
		d.Configuration.Pact = nil
		changed = true
	}
	if !changed {
		return nil
	}
	return a.backend.ConfigurationTransaction(func(_ rules.Configuration, _ platform.Context, apply func(rules.Configuration) error) error {
		d.NeedsApply = true
		if err := a.save(d); err != nil {
			a.blocked = err
			return err
		}
		if err := apply(d.Configuration); err != nil {
			a.blocked = err
			return err
		}
		d.NeedsApply = false
		if err := a.save(d); err != nil {
			a.blocked = err
			return err
		}
		return nil
	})
}

func (a *Authority) Context() (platform.Context, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refresh(); err != nil {
		return platform.Context{}, err
	}
	_, found, err := a.load()
	if err != nil {
		return platform.Context{}, err
	}
	if !found {
		c, err := a.Adapter.Context()
		if err != nil {
			return platform.Context{}, err
		}
		return c, nil
	}
	c, err := a.Adapter.Facts()
	if err != nil {
		return platform.Context{}, err
	}
	return a.appContext(c)
}

// Called with the authority mutex held; rebuilds the snapshot's profiles from
// the authoritative document. The platform supplies facts only (handshakes,
// timezone, captains, profile presence, volatile overrides/DNS).
func (a *Authority) appContext(c platform.Context) (platform.Context, error) {
	d, found, err := a.load()
	if err != nil {
		return platform.Context{}, err
	}
	if !found {
		return c, nil
	}
	if d.Bonus == nil {
		return platform.Context{}, errors.New("bonus recovery required")
	}
	now := a.at(c)
	return AppContext(d.Configuration, c, now, d.Bonus.lines(now.Format(time.DateOnly)))
}

func (a *Authority) Policy(profile string, version int) (platform.Policy, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refresh(); err != nil {
		return platform.Policy{}, err
	}
	d, found, err := a.load()
	if err != nil {
		return platform.Policy{}, err
	}
	if !found {
		p, err := a.Adapter.Policy(profile, version)
		if err != nil {
			return p, err
		}
		return normalizePhoneWindows(p)
	}
	facts, err := a.Adapter.Facts()
	if err != nil {
		return platform.Policy{}, err
	}
	if d.Bonus == nil {
		return platform.Policy{}, errors.New("bonus recovery required")
	}
	now := a.at(facts)
	c, err := AppContext(d.Configuration, facts, now, d.Bonus.lines(now.Format(time.DateOnly)))
	if err != nil {
		return platform.Policy{}, err
	}
	p := platform.Policy{Context: c}
	if prof, ok := facts.Profiles[profile]; ok {
		p.DNS = prof.DNS
		if b, err := json.Marshal(map[string]string{"override": prof.Override}); err == nil {
			p.Rules = b
		}
	}
	if facts.MastCaptain == profile {
		p.Captain = profile
	}
	if prof, ok := c.Profiles[profile]; ok && version == 3 && prof.AppInput != nil {
		apps, _, err := prof.AppInput.Compile()
		if err != nil {
			return platform.Policy{}, err
		}
		if apps != nil {
			p.Apps, err = json.Marshal(apps)
			if err != nil {
				return platform.Policy{}, err
			}
		}
	}
	return PhonePolicy(d.Configuration, p, profile, now)
}

func (a *Authority) WithContext(fn func(platform.Context) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refresh(); err != nil {
		return err
	}
	_, found, err := a.load()
	if err != nil {
		return err
	}
	if !found {
		return a.Adapter.WithContext(fn)
	}
	return a.Adapter.FactsContext(func(c platform.Context) error {
		compiled, err := a.appContext(c)
		if err != nil {
			return err
		}
		return fn(compiled)
	})
}

func (a *Authority) Configuration() (rules.Configuration, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refresh(); err != nil {
		return rules.Configuration{}, err
	}
	d, found, err := a.load()
	if err != nil {
		return rules.Configuration{}, err
	}
	if found {
		return d.Configuration, nil
	}
	return a.Adapter.Configuration()
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("configuration nesting too deep")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return errors.New("duplicate configuration field")
				}
				keys[name] = true
				if err := uniqueJSON(d, depth+1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := uniqueJSON(d, depth+1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid configuration JSON")
		}
		_, err = d.Token()
		return err
	}
	return nil
}

func decode(raw []byte, v any) error {
	probe := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueJSON(probe, 0); err != nil {
		return err
	}
	if _, err := probe.Token(); err != io.EOF {
		return errors.New("trailing configuration data")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	return nil
}

func (a *Authority) at(facts platform.Context) time.Time {
	loc, err := time.LoadLocation(facts.TimeZone)
	if err != nil {
		loc = time.UTC
	}
	return a.Now().In(loc)
}

func (a *Authority) API(method string, raw []byte) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	fail := func(err error) map[string]any { return map[string]any{"error": err.Error()} }
	if err := a.refresh(); err != nil {
		return fail(err)
	}
	d, found, err := a.load()
	if err != nil {
		return fail(err)
	}
	switch method {
	case "configuration_status":
		owner := "legacy"
		if found {
			owner = "core"
		}
		return map[string]any{"owner": owner}
	case "configuration_block", "configuration_allow", "configuration_reset", "configuration_ticket":
		if !found {
			return fail(errors.New("configuration import required"))
		}
		r, err := a.changeNetwork(d, method, raw)
		if err != nil {
			reply := fail(err)
			if errors.Is(err, errNetworkBound) && d.Configuration.Pact != nil {
				reply["pact_until"] = integer(d.Configuration.Pact.Until)
			}
			return reply
		}
		return r
	case "configuration_bonus_add":
		if !found || d.Bonus == nil {
			return fail(errors.New("configuration import required"))
		}
		r, err := a.addBonus(d, raw)
		if err != nil {
			return fail(err)
		}
		return r
	case "configuration_bonus_read":
		if !found || d.Bonus == nil {
			return fail(errors.New("configuration import required"))
		}
		facts, err := a.Adapter.Facts()
		if err != nil {
			return fail(err)
		}
		if facts.BonusFormat != bonusFormat {
			return fail(errors.New("incompatible bonus adapter"))
		}
		day := a.at(facts).Format(time.DateOnly)
		totals := map[string]int{}
		for _, e := range d.Bonus.Entries {
			if e.Day == day && e.Target == "total" {
				totals[e.Profile] = e.Minutes
			}
		}
		return map[string]any{"format": bonusFormat, "day": day, "totals": totals}
	case "configuration_report":
		if !found || d.Bonus == nil {
			return fail(errors.New("configuration import required"))
		}
		var req struct {
			Profile string `json:"profile"`
		}
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		if req.Profile != "" && !identifier.MatchString(req.Profile) {
			return fail(errors.New("invalid profile"))
		}
		var single *string
		if req.Profile != "" {
			single = &req.Profile
		}
		report, err := a.networkReport(d, single)
		if err != nil {
			return fail(err)
		}
		return report
	case "configuration_dns":
		if !found {
			return fail(errors.New("configuration import required"))
		}
		domain, captain, profiles := DNS(d.Configuration, a.Now())
		return map[string]any{"format": "dns-config-v1", "captain": captain, "input": map[string]any{"domain": domain, "profiles": profiles, "url": d.Configuration.Current.Settings.ResolverURL}}
	case "configuration_export":
		if !found {
			c, err := a.Adapter.Configuration()
			if err != nil {
				return fail(errors.New("configuration unavailable"))
			}
			if err := c.Seal(); err != nil {
				return fail(errors.New("configuration unavailable"))
			}
			return map[string]any{"owner": "legacy", "configuration": c}
		}
		return map[string]any{"owner": "core", "configuration": d.Configuration, "pending": d.Pending != nil}
	case "configuration_import":
		if found {
			return map[string]any{"status": "ok", "revision": d.Configuration.Revision, "already_imported": true}
		}
		var req struct {
			Revision string `json:"revision"`
		}
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		var imported rules.Configuration
		err := a.backend.ConfigurationTransaction(func(c rules.Configuration, facts platform.Context, apply func(rules.Configuration) error) error {
			if err := Validate(c); err != nil {
				return err
			}
			if err := c.Seal(); err != nil {
				return err
			}
			if c.Revision != req.Revision {
				return errors.New("configuration conflict")
			}
			// The original complete snapshot is durable and read back before any
			// legacy file becomes a projection. Restart can finish a lost import.
			bonus, err := importBonus(facts)
			if err != nil {
				return err
			}
			d := document{Version: 1, Configuration: c, NeedsApply: true, Bonus: bonus}
			if err := a.save(d); err != nil {
				return err
			}
			if err := apply(c); err != nil {
				return err
			}
			d.NeedsApply = false
			if err := a.save(d); err != nil {
				return err
			}
			imported = c
			return nil
		})
		if err != nil {
			return fail(err)
		}
		return map[string]any{"status": "ok", "revision": imported.Revision}
	case "configuration_apply":
		if !found {
			return fail(errors.New("configuration import required"))
		}
		if d.Pending != nil {
			return fail(errors.New("confirmation pending"))
		}
		var req editRequest
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		if req.Revision != d.Configuration.Revision {
			return fail(errors.New("configuration conflict"))
		}
		if Active(d.Configuration.Pact, a.Now()) && !reflect.DeepEqual(frozen(d.Configuration.Current, d.Configuration.Pact.Captain), frozen(req.Current, d.Configuration.Pact.Captain)) {
			return fail(errors.New("use configuration_amend for protected pact edits"))
		}
		next := d.Configuration
		next.Current = req.Current
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return fail(err)
		}
		var pending *pendingChange
		attempted := false
		err := a.backend.ConfigurationTransaction(func(_ rules.Configuration, facts platform.Context, apply func(rules.Configuration) error) error {
			if err := ValidateEdit(d.Configuration, &next, a.at(facts)); err != nil {
				return err
			}
			if err := next.Seal(); err != nil {
				return err
			}
			if next.Revision == d.Configuration.Revision {
				return nil
			}
			pending = &pendingChange{Before: d.Configuration, Token: hex.EncodeToString(token[:]), Until: a.Now().Add(2 * time.Minute).Unix()}
			candidate := document{Version: 1, Configuration: next, Pending: pending, NeedsApply: true, Bonus: d.Bonus}
			attempted = true
			if err := a.save(candidate); err != nil {
				return err
			}
			if err := apply(next); err != nil {
				if restore := apply(d.Configuration); restore != nil {
					return fmt.Errorf("apply failed; recovery required: %w", restore)
				}
				if restore := a.save(d); restore != nil {
					return restore
				}
				return err
			}
			candidate.NeedsApply = false
			return a.save(candidate)
		})
		if err != nil {
			if attempted {
				a.blocked = err
			}
			return fail(err)
		}
		if pending == nil {
			return map[string]any{"status": "ok", "revision": next.Revision}
		}
		if a.timer != nil {
			a.timer.Stop()
		}
		a.timer = time.AfterFunc(2*time.Minute, func() { a.mu.Lock(); defer a.mu.Unlock(); _ = a.refresh() })
		return map[string]any{"status": "confirmation_required", "revision": next.Revision, "token": pending.Token, "deadline": pending.Until}
	case "configuration_confirm":
		if !found || d.Pending == nil {
			return fail(errors.New("no pending confirmation"))
		}
		var req struct {
			Revision string `json:"revision"`
			Token    string `json:"token"`
		}
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		if req.Revision != d.Configuration.Revision || req.Token != d.Pending.Token {
			return fail(errors.New("configuration conflict"))
		}
		d.Pending = nil
		if err := a.save(d); err != nil {
			return fail(err)
		}
		if a.timer != nil {
			a.timer.Stop()
		}
		return map[string]any{"status": "ok", "revision": d.Configuration.Revision}
	case "configuration_tick":
		return map[string]any{"status": "ok"}
	case "configuration_reapply":
		if !found {
			return fail(errors.New("configuration import required"))
		}
		err := a.backend.ConfigurationTransaction(func(_ rules.Configuration, _ platform.Context, apply func(rules.Configuration) error) error {
			return apply(d.Configuration)
		})
		if err != nil {
			a.blocked = err
			return fail(err)
		}
		return map[string]any{"status": "ok"}
	default:
		if !found {
			return fail(errors.New("configuration import required"))
		}
		if d.Pending != nil {
			return fail(errors.New("confirmation pending"))
		}
		next := d.Configuration
		attempted := false
		err := a.backend.ConfigurationTransaction(func(_ rules.Configuration, facts platform.Context, apply func(rules.Configuration) error) error {
			if err := PactOperation(method, raw, &next, a.at(facts)); err != nil {
				return err
			}
			if err := Validate(next); err != nil {
				return err
			}
			if err := next.Seal(); err != nil {
				return err
			}
			attempted = true
			// Core mutex plus platform transaction prevents any signed observer
			// seeing a pact that later rolls back after a failed apply.
			if err := a.save(document{Version: 1, Configuration: next, NeedsApply: true, Bonus: d.Bonus}); err != nil {
				return err
			}
			if err := apply(next); err != nil {
				if restore := apply(d.Configuration); restore != nil {
					return restore
				}
				if restore := a.save(d); restore != nil {
					return restore
				}
				return err
			}
			return a.save(document{Version: 1, Configuration: next, Bonus: d.Bonus})
		})
		if err != nil {
			if attempted {
				a.blocked = err
			}
			return fail(err)
		}
		result := map[string]any{"status": "ok", "revision": next.Revision}
		if next.Pact != nil {
			result["captain"] = next.Pact.Captain
			result["type"] = next.Pact.Type
			result["pact_until"] = integer(next.Pact.Until)
			result["release_at"] = integer(next.Pact.ReleaseAt)
		}
		return result
	}
}
