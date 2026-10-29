package configuration

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strconv"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// RuleChange is a single standing-rule delta, never a configuration document.
// The identity service holds its registry lock through authorization and commit.
type RuleChange struct {
	Owner    string
	Nonce    string
	Digest   string
	Profile  string
	Package  string
	Mode     string
	Minutes  int
	Revision string
	Expires  int64
}

// AppRule runs under identity -> authority -> platform locks. authorize must not
// call the adapter. Status reports desired-state equality, not a delivery receipt.
func (a *Authority) AppRule(req RuleChange, status bool, authorize func(platform.Context) bool) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	fail := func(code string) map[string]any { return map[string]any{"error": code} }
	if !ruleHash.MatchString(req.Owner) || !ruleHash.MatchString(req.Nonce) || !ruleHash.MatchString(req.Digest) || req.Expires <= 0 || req.Expires > 253402300799 || !identifier.MatchString(req.Profile) || !rules.ValidAppPackage(req.Package) || req.Revision == "" ||
		!slices.Contains([]string{"allowed", "limit", "always", "blocked"}, req.Mode) || req.Minutes < 0 || req.Minutes > 1440 || (req.Mode != "limit" && req.Minutes != 0) {
		return fail("malformed")
	}
	if a.blocked != nil {
		return fail("outcome_unknown")
	}
	// Ordinary policy sync owns lifecycle maintenance. A refusal or status
	// query must not roll back confirmation or prune state as a side effect.
	d, found, err := a.load()
	if err != nil {
		return fail("outcome_unknown")
	}
	if !found {
		return fail("configuration_required")
	}
	if d.NeedsApply || d.Bonus == nil {
		return fail("outcome_unknown")
	}
	result := fail("unavailable")
	attempted := false
	err = a.backend.ConfigurationTransaction(func(_ rules.Configuration, facts platform.Context, overrides map[string]platform.NetworkOverride, apply func(rules.Configuration, platform.NetworkIntent) error) error {
		ctx, err := configuredAppContext(d.Configuration, facts, a.Now(), d.Bonus.lines(a.at(facts).Format("2006-01-02")))
		if err != nil {
			return err
		}
		if !authorize(ctx) {
			result = fail("not_parent")
			return nil
		}
		prof, ok := ctx.Profiles[req.Profile]
		if !ok || prof.AppsRev == "" {
			result = fail("app_control_off")
			return nil
		}
		if d.Pending != nil {
			result = fail("confirmation_pending")
			return nil
		}
		if ctx.PactCaptain == req.Profile {
			result = fail("not_parent")
			return nil
		}
		next := clone(d.Configuration)
		p := profile(next.Current, req.Profile)
		p.AlwaysAllowed = slices.DeleteFunc(p.AlwaysAllowed, func(v string) bool { return v == req.Package })
		kept := next.Current.Apps[:0]
		id := ""
		for _, app := range next.Current.Apps {
			if app.Profile == req.Profile && app.Package == req.Package {
				if id == "" {
					id = app.ID
				}
				continue
			}
			kept = append(kept, app)
		}
		next.Current.Apps = kept
		if req.Mode == "always" {
			p.AlwaysAllowed = append(p.AlwaysAllowed, req.Package)
		} else {
			if id == "" {
				var token [16]byte
				if _, err := rand.Read(token[:]); err != nil {
					return err
				}
				id = "app" + hex.EncodeToString(token[:])
			}
			app := rules.ConfigurationApp{ID: id, Profile: req.Profile, Package: req.Package, Blocked: "0"}
			if req.Mode == "blocked" {
				app.Blocked = "1"
			}
			if req.Mode == "limit" {
				app.Minutes = strconv.Itoa(req.Minutes)
			}
			next.Current.Apps = append(next.Current.Apps, app)
		}
		// Compare compiled rules rather than representation IDs or legacy spellings.
		input := profileAppConfig(next.Current, *p, d.Bonus.lines(a.at(facts).Format("2006-01-02"))[req.Profile])
		desired, _, err := input.Compile()
		if err != nil || desired == nil {
			return errors.New("rule compilation failed")
		}
		current, _, err := prof.AppInput.Compile()
		if err != nil || current == nil {
			return errors.New("rule compilation failed")
		}
		matches := ruleEquals(current, req)
		reply := func(state string, rev string) map[string]any {
			return map[string]any{"state": state, "profile_id": req.Profile, "package": req.Package, "apps_rev": rev}
		}
		// An already committed nonce may only be observed, never recommitted,
		// even if another parent restored the original content-hash revision.
		for _, entry := range d.RuleCommits {
			if entry.Owner == req.Owner && entry.Nonce == req.Nonce {
				if entry.Digest != req.Digest {
					result = fail("replay")
					return nil
				}
				if matches {
					result = reply("matched", prof.AppsRev)
				} else {
					result = reply("conflict", prof.AppsRev)
				}
				return nil
			}
		}
		if matches {
			result = reply("matched", prof.AppsRev)
			return nil
		}
		if prof.AppsRev != req.Revision {
			result = reply("conflict", prof.AppsRev)
			return nil
		}
		now := max(a.Now().Unix(), d.RuleTime)
		if now >= req.Expires {
			result = reply("expired", prof.AppsRev)
			return nil
		}
		if status {
			result = reply("retryable", prof.AppsRev)
			return nil
		}
		commits := make([]ruleCommit, 0, maxRuleCommits)
		for _, entry := range d.RuleCommits {
			if entry.Expires > now {
				commits = append(commits, entry)
			}
		}
		if len(commits) >= maxRuleCommits {
			result = fail("operation_capacity")
			return nil
		}
		commits = append(commits, ruleCommit{Owner: req.Owner, Nonce: req.Nonce, Digest: req.Digest, Expires: req.Expires})
		if err := ValidateEdit(d.Configuration, &next, a.at(facts)); err != nil {
			result = fail("invalid_rule")
			return nil
		}
		if err := next.Seal(); err != nil {
			return err
		}
		intent, err := a.intentFor(next, facts, overrides)
		if err != nil {
			return err
		}
		// Durable desired state precedes projection. Any uncertain write/apply is
		// recoverable at startup and must never be labelled a pre-commit refusal.
		candidate := document{Version: 1, Configuration: next, Bonus: d.Bonus, NeedsApply: true, RuleCommits: commits, RuleTime: now}
		attempted = true
		if err := a.save(candidate); err != nil {
			return err
		}
		if err := a.applyConfiguration(apply, next, intent); err != nil {
			return err
		}
		candidate.NeedsApply = false
		if err := a.save(candidate); err != nil {
			return err
		}
		result = reply("saved", desired.Rev)
		return nil
	})
	if err != nil {
		if attempted {
			a.blocked = err
			return fail("outcome_unknown")
		}
		return fail("unavailable")
	}
	return result
}

func ruleEquals(p *rules.AppPolicy, r RuleChange) bool {
	if slices.Contains(p.Blocked, r.Package) {
		return r.Mode == "blocked"
	}
	if slices.Contains(p.AlwaysAllowed, r.Package) {
		return r.Mode == "always"
	}
	for _, l := range p.Limits {
		if l.Package == r.Package {
			return r.Mode == "limit" && l.Minutes == r.Minutes
		}
	}
	return slices.Contains(p.Approved, r.Package) && r.Mode == "allowed"
}

const maxRuleCommits = 64

var ruleHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Kept atomically with committed configuration, including recovery and root edits.
// RuleTime prevents clock rollback from reviving entries pruned after expiration.
type ruleCommit struct {
	Owner   string `json:"owner"`
	Nonce   string `json:"nonce"`
	Digest  string `json:"digest"`
	Expires int64  `json:"expires"`
}

func validateRuleCommits(entries []ruleCommit, floor int64) error {
	if len(entries) > maxRuleCommits || floor < 0 || floor > 253402300799 {
		return errors.New("invalid rule replay journal")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		key := entry.Owner + entry.Nonce
		if !ruleHash.MatchString(entry.Owner) || !ruleHash.MatchString(entry.Nonce) || !ruleHash.MatchString(entry.Digest) || entry.Expires <= 0 || entry.Expires > 253402300799 || seen[key] {
			return errors.New("invalid rule replay journal")
		}
		seen[key] = true
	}
	return nil
}
