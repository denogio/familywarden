package service

import (
	"testing"
)

func TestVoyageDisplayFactsUseTheFrozenPactAndStayScoped(t *testing.T) {
	f := newPortableFixture(t)
	captain := f.phone("fw-jordan", "jordan")
	child := f.phone("fw-alex", "alex")
	f.api("configuration_pact", map[string]any{"until": 0, "window_day": "sun", "window_start": "10:00", "window_stop": "11:00"})
	f.host.mu.Lock()
	f.host.current.Current.Settings.DelayHours = "99" // stale compatibility projection
	f.host.mu.Unlock()
	pact := objectField(t, f.sync(captain)["pact"])
	details := objectField(t, pact["details"])
	if details["waiting_seconds"] != float64(3600) {
		t.Fatal("waiting period was not frozen", pact)
	}
	window := objectField(t, details["amendment"])
	if window["days"] != "sun" || window["start"] != "10:00" || window["stop"] != "11:00" {
		t.Fatal(window)
	}
	if p := objectField(t, f.sync(child)["pact"]); p["active"] != false || p["details"] != nil {
		t.Fatal("captain metadata leaked to child", p)
	}
	f.restart()
	if d := objectField(t, objectField(t, f.sync(captain)["pact"])["details"]); d["waiting_seconds"] != float64(3600) {
		t.Fatal(d)
	}
}

func TestVoyageAmendmentDisplayPreservesTheExclusiveMidnightEnd(t *testing.T) {
	f := newPortableFixture(t)
	captain := f.phone("fw-jordan", "jordan")
	f.api("configuration_pact", map[string]any{"until": 0, "window_day": "sun", "window_start": "23:00", "window_stop": "24:00"})
	details := objectField(t, objectField(t, f.sync(captain)["pact"])["details"])
	if window := objectField(t, details["amendment"]); window["stop"] != "24:00" {
		t.Fatal(window)
	}
}

func TestFixedVoyageDoesNotAdvertiseAnAmendmentWindow(t *testing.T) {
	f := newPortableFixture(t)
	captain := f.phone("fw-jordan", "jordan")
	f.api("configuration_pact", map[string]any{"until": f.now().Unix() + 7200})
	p := objectField(t, f.sync(captain)["pact"])
	if d := objectField(t, p["details"]); d["amendment"] != nil || d["waiting_seconds"] != float64(3600) {
		t.Fatal(p)
	}
}
