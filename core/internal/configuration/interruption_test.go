package configuration

import (
	"testing"
	"time"
)

// Every durable write of every household mutation is interrupted in turn. The
// mutation must never report success, and after a restart the native projection
// must equal the durable document, which is either the state before or the
// uninterrupted result after the mutation: never a mixture.
func TestInterruptedMutationMatrix(t *testing.T) {
	type op struct {
		prepare func(*Authority) // runs uninterrupted first
		run     func(*Authority) map[string]any
		later   bool // advance the clock before run (release completion)
	}
	pact := func(a *Authority) { okResult(t, a.API("configuration_pact", []byte(`{"until":1800003600}`))) }
	requested := func(a *Authority) {
		pact(a)
		okResult(t, a.API("configuration_sail_request", []byte(`{}`)))
	}
	ops := map[string]op{
		"apply": {run: func(a *Authority) map[string]any { return a.API("configuration_apply", candidate(a, "Robin")) }},
		"pact":  {run: func(a *Authority) map[string]any { return a.API("configuration_pact", []byte(`{"until":1800003600}`)) }},
		"sail-request": {prepare: pact, run: func(a *Authority) map[string]any {
			return a.API("configuration_sail_request", []byte(`{}`))
		}},
		"sail-cancel": {prepare: requested, run: func(a *Authority) map[string]any {
			return a.API("configuration_sail_cancel", []byte(`{}`))
		}},
		"release-completion": {prepare: requested, later: true, run: func(a *Authority) map[string]any {
			return a.API("configuration_tick", nil)
		}},
		"confirm": {
			prepare: func(a *Authority) {
				r := a.API("configuration_apply", candidate(a, "Robin"))
				okResult(t, r)
				pendingConfirm = map[string]any{"revision": r["revision"], "token": r["token"]}
			},
			run: func(a *Authority) map[string]any { return a.API("configuration_confirm", request(pendingConfirm)) },
		},
	}
	start := time.Unix(1800000000, 0).UTC()
	later := start.Add(25 * time.Hour)
	fixture := func(t *testing.T, o op) (*Authority, *fakeBackend, *failWrite) {
		b := &fakeBackend{current: sample()}
		_ = b.current.Seal()
		st := &failWrite{}
		a, err := New(b, b, st)
		if err != nil {
			t.Fatal(err)
		}
		a.Now = func() time.Time { return start }
		t.Cleanup(func() { a.Close() })
		activate(t, a, b)
		if o.prepare != nil {
			o.prepare(a)
		}
		if o.later {
			a.Now = func() time.Time { return later }
		}
		return a, b, st
	}
	for name, o := range ops {
		t.Run(name, func(t *testing.T) {
			// Uninterrupted control: the only acceptable "after" state.
			control, _, cst := fixture(t, o)
			before := cst.writes
			okResult(t, o.run(control))
			after, _, _ := control.load()
			needed := cst.writes - before
			if needed == 0 {
				t.Fatal("mutation wrote nothing; the matrix would be vacuous")
			}
			t.Logf("%s: interrupting each of %d durable writes", name, needed)
			// Each case interrupts one durable write (k >= 1) or the native apply (k == 0).
			for k := 0; k <= needed; k++ {
				a, b, st := fixture(t, o)
				prior, _, _ := a.load()
				applies := b.applies
				if k == 0 {
					b.fail = 1
				} else {
					st.at = st.writes + k
				}
				r := o.run(a)
				if k == 0 && b.applies == applies {
					t.Logf("%s: no native apply to interrupt", name)
					continue
				}
				if k == 0 {
					t.Logf("%s: interrupting the native apply", name)
				}
				if r["error"] == nil {
					t.Fatalf("case %d/%d interrupted but the mutation reported success: %v", k, needed, r)
				}
				st.at, b.fail = 0, 0
				restarted, err := New(b, b, st)
				if err != nil {
					t.Fatalf("case %d/%d: restart could not reconcile: %v", k, needed, err)
				}
				restarted.Now = a.Now
				t.Cleanup(func() { restarted.Close() })
				d, found, err := restarted.load()
				if err != nil || !found || d.NeedsApply {
					t.Fatalf("case %d/%d: durable state unreconciled: %v %v", k, needed, d.NeedsApply, err)
				}
				if b.current.Revision != d.Configuration.Revision {
					t.Fatalf("case %d/%d: native projection differs from the durable document", k, needed)
				}
				// An unconfirmed edit legitimately rolls back to its confirmed predecessor.
				confirmed := prior.Configuration.Revision
				if prior.Pending != nil {
					confirmed = prior.Pending.Before.Revision
				}
				if rev := d.Configuration.Revision; rev != prior.Configuration.Revision && rev != after.Configuration.Revision && rev != confirmed {
					t.Fatalf("case %d/%d: mixed state after restart", k, needed)
				}
				if _, err := restarted.Context(); err != nil {
					t.Fatalf("case %d/%d: recovered authority refuses service: %v", k, needed, err)
				}
			}
		})
	}
}

var pendingConfirm map[string]any

// Bonus is a durable runtime ledger inside the journal: an interrupted grant is
// neither applied twice nor silently kept, and an explicit retry adds it once.
func TestInterruptedBonusIsNeitherLostSilentlyNorDoubled(t *testing.T) {
	a, b, mem := bonusSetup(t, "")
	activate(t, a, b.fakeBackend)
	st := &failWrite{memory: *mem}
	interrupted, err := New(b, b, st)
	if err != nil {
		t.Fatal(err)
	}
	interrupted.Now = a.Now
	t.Cleanup(func() { interrupted.Close() })
	st.at = st.writes + 1
	if r := giveBonus(interrupted, "", 15); r["error"] == nil {
		t.Fatal("interrupted bonus reported success", r)
	}
	st.at = 0
	restarted, err := New(b, b, st)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = a.Now
	t.Cleanup(func() { restarted.Close() })
	total := func() int { return restarted.API("configuration_bonus_read", nil)["totals"].(map[string]int)["alex"] }
	if total() != 0 {
		t.Fatal("interrupted bonus survived as success", total())
	}
	okResult(t, giveBonus(restarted, "", 15))
	if total() != 15 {
		t.Fatal("retry after interruption did not add exactly once", total())
	}
}
