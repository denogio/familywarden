package identity

import (
	"testing"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/store"
)

type mutationGuard struct {
	platform.Adapter
	t      *testing.T
	active bool
	calls  int
}

func (g *mutationGuard) Context() (platform.Context, error) {
	g.t.Error("authority mutation used a released context snapshot")
	return g.Adapter.Context()
}

func (g *mutationGuard) WithContext(fn func(platform.Context) error) error {
	return g.Adapter.WithContext(func(ctx platform.Context) error {
		g.calls++
		g.active = true
		defer func() { g.active = false }()
		return fn(ctx)
	})
}

func (g *mutationGuard) EnsureTicket(profile string, until int64) (bool, error) {
	if g.active {
		g.t.Error("ticket/apply ran inside the state gate (lock-order inversion)")
	}
	return g.Adapter.EnsureTicket(profile, until)
}

type guardedDocuments struct {
	Storage
	guard *mutationGuard
}

func (s guardedDocuments) WriteDocument(name string, v any) error {
	if !s.guard.active {
		s.guard.t.Error("document saved outside the mutation gate:", name)
	}
	return s.Storage.WriteDocument(name, v)
}

type guardedUsage struct {
	UsageStore
	guard *mutationGuard
}

func (s guardedUsage) ApplyUsage(profile string, r store.Report) (bool, error) {
	if !s.guard.active {
		s.guard.t.Error("usage saved outside the mutation gate")
	}
	return s.UsageStore.ApplyUsage(profile, r)
}

func TestAllRuntimeAuthorityMutationsUseThePlatformGate(t *testing.T) {
	for _, op := range []string{"approve", "update", "delete", "removal", "answer", "new-app", "signed-request", "signed-decision", "signed-usage", "signed-app"} {
		t.Run(op, func(t *testing.T) {
			f := usageFamily(t)
			var action func() Reply
			switch op {
			case "approve":
				p := newPhone(t)
				f.s.Phone(p.query("fw-new", false, "", ""))
				action = func() Reply {
					return f.s.Approve(Change{Device: "fw-new", Profile: "kid", Fingerprint: fingerprint(p.pub)})
				}
			case "update":
				action = func() Reply { return f.s.Update(Change{Device: "fw-parent", Name: "Robin phone"}) }
			case "delete":
				action = func() Reply { return f.s.Delete("fw-parent") }
			case "removal":
				action = func() Reply { return f.s.AllowRemoval("fw-parent") }
			case "answer", "signed-decision":
				raw, _ := f.request(t, "req-00000001", 20)
				if op == "answer" {
					action = func() Reply { return f.s.AnswerRequest(Answer{RequestID: "req-00000001", Approve: true, Minutes: 10}) }
				} else {
					action = func() Reply {
						return Reply(send(t, f.parent, f.s, "decision", decisionJSON("req-00000001", sha(raw), "fw-parent", true, 10, t0)))
					}
				}
			case "new-app", "signed-app":
				if m := send(t, f.kid, f.s, "usage", usageWithNewApps(1, `{"com.example.newgame":"New Game"}`)); m["state"] != "stored" {
					t.Fatal(m)
				}
				if op == "new-app" {
					action = func() Reply {
						return f.s.AppDecision(AppDecisionInput{Profile: "kid", Package: "com.example.newgame", Action: "approve"})
					}
				} else {
					action = func() Reply {
						return Reply(send(t, f.parent, f.s, "app", appDecisionJSON("fw-parent", "kid", "com.example.newgame", "approve", t0)))
					}
				}
			case "signed-request":
				action = func() Reply {
					return Reply(send(t, f.kid, f.s, "request", requestJSON("req-00000001", "fw-kid", "kid", "com.example.video", 10, t0)))
				}
			case "signed-usage":
				action = func() Reply {
					return Reply(send(t, f.kid, f.s, "usage", usageJSON("fw-kid", "kid", usageDay, 1, 60, `{}`, "")))
				}
			}
			g := &mutationGuard{Adapter: f.s.ad, t: t}
			f.s.ad = g
			f.s.st = guardedDocuments{Storage: f.s.st, guard: g}
			f.s.Usage = guardedUsage{UsageStore: f.s.Usage, guard: g}
			if r := action(); r["error"] != nil {
				t.Fatal(r)
			}
			if g.calls != 1 || g.active {
				t.Fatal("missing or leaked transaction", g.calls, g.active)
			}
		})
	}
}
