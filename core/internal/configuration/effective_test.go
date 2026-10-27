package configuration

import (
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/rules"
)

func importedDivergence() rules.Configuration {
	c := pacted("ongoing")
	c.Pact.ReleaseAt = ""
	c.Current.Profiles[0].Apps = "1"
	c.Current.Profiles[0].ScreenMinutes = "999"
	c.Pact.Frozen.Profiles[0].Apps = "1"
	c.Pact.Frozen.Profiles[0].ScreenMinutes = "10"
	_ = c.Seal()
	return c
}

func TestImportedPactNeverChangesMerelyAtWindowBoundaries(t *testing.T) {
	c := importedDivergence()
	for _, at := range []time.Time{
		time.Date(2027, 1, 3, 9, 59, 0, 0, time.UTC),
		time.Date(2027, 1, 3, 10, 30, 0, 0, time.UTC),
		time.Date(2027, 1, 3, 11, 1, 0, 0, time.UTC),
	} {
		effective := Effective(c, at)
		if profile(effective, "alex").ScreenMinutes != "10" || effective.Schedules[0].Stop != "20:00" {
			t.Fatal("window boundary promoted ignored configured rules", at, effective)
		}
	}
}

func TestOnlyExplicitAmendmentCanPromoteImportedConfiguredRules(t *testing.T) {
	a, b, _ := setup(t)
	a.Now = func() time.Time { return time.Date(2027, 1, 3, 10, 30, 0, 0, time.UTC) }
	b.current = importedDivergence()
	activate(t, a, b)
	r := a.API("configuration_apply", candidate(a, "Robin"))
	okResult(t, r)
	okResult(t, a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": r["token"]})))
	d, _, _ := a.load()
	if d.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "10" || d.Configuration.Pact.Frozen.Schedules[0].Stop != "20:00" {
		t.Fatal("unrelated edit refreshed the captain's frozen snapshot")
	}
	// Explicit confirmation can deliberately activate previously ignored
	// configured values, even when the current document itself is unchanged.
	okResult(t, a.API("configuration_amend", request(map[string]any{"revision": d.Configuration.Revision, "current": d.Configuration.Current})))
	d, _, _ = a.load()
	if d.Pending != nil || d.Configuration.Pact.Frozen.Profiles[0].ScreenMinutes != "999" {
		t.Fatal("explicit amendment did not commit the new snapshot", d)
	}
	after := Effective(d.Configuration, time.Date(2027, 1, 3, 11, 1, 0, 0, time.UTC))
	if profile(after, "alex").ScreenMinutes != "999" || after.Schedules[0].Stop != "23:00" {
		t.Fatal("amendment reverted when its window closed", after)
	}
}
