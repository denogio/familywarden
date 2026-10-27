package configuration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// Shell-shaped platform reply for the same state (what _json_pact_rules prints).
func shellPolicy(captain, override string) platform.Policy {
	rules := `{"enabled":"0","policy":"allow","override":"none","windows":[]}`
	if captain != "" {
		rules = `{"enabled":"1","policy":"scheduled","override":"` + override + `","windows":[]}`
	}
	return platform.Policy{Captain: captain, Pact: json.RawMessage(`{"active":false,"type":"","until":0,"release_at":0}`), Rules: json.RawMessage(rules)}
}

func decoded(t *testing.T, p platform.Policy) (map[string]any, map[string]any) {
	t.Helper()
	var pact, r map[string]any
	if json.Unmarshal(p.Pact, &pact) != nil || json.Unmarshal(p.Rules, &r) != nil {
		t.Fatal(string(p.Pact), string(p.Rules))
	}
	return pact, r
}

func pacted(typ string) rules.Configuration {
	c := sample()
	snap := frozen(c.Current, "alex")
	c.Current.Schedules[0].Stop = "23:00" // looser live edit, ignored while frozen
	c.Pact = &rules.ConfigurationPact{Captain: "alex", Type: typ, Until: "1800003600", WindowDay: "sun",
		WindowStart: "10:00", WindowStop: "11:00", DelaySeconds: "3600", DefaultPolicy: "block", ReleaseAt: "1800001800", Frozen: snap}
	return c
}

func TestPhonePolicyComesFromTheDocument(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC() // a Friday, outside the Sunday window
	p, err := PhonePolicy(sample(), shellPolicy("alex", "block"), "alex", now)
	if err != nil {
		t.Fatal(err)
	}
	pact, r := decoded(t, p)
	if pact["active"] != false || r["override"] != "block" || r["policy"] != "scheduled" || len(r["windows"].([]any)) != 1 {
		t.Fatal(string(p.Pact), string(p.Rules))
	}

	// Another profile is never the captain: empty rules, inactive pact.
	p, err = PhonePolicy(sample(), shellPolicy("", ""), "taylor", now)
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Rules) != `{"enabled":"0","policy":"allow","override":"none","windows":[]}` || string(p.Pact) != `{"active":false,"type":"","until":0,"release_at":0}` {
		t.Fatal(string(p.Pact), string(p.Rules))
	}

	// An active pact sends the frozen windows and its deadlines, not live edits.
	p, err = PhonePolicy(pacted("until"), shellPolicy("alex", "none"), "alex", now)
	if err != nil {
		t.Fatal(err)
	}
	pact, r = decoded(t, p)
	w := r["windows"].([]any)[0].(map[string]any)
	if pact["active"] != true || pact["type"] != "until" || pact["until"] != float64(1800003600) || pact["release_at"] != float64(1800001800) || w["stop"] != "20:00" {
		t.Fatal(string(p.Pact), string(p.Rules))
	}

	// Opening the window alone must not activate ignored configured edits.
	// Only an explicit amendment may replace the frozen captain.
	sunday := time.Date(2027, 1, 3, 10, 30, 0, 0, time.UTC)
	p, err = PhonePolicy(pacted("ongoing"), shellPolicy("alex", "none"), "alex", sunday)
	if err != nil {
		t.Fatal(err)
	}
	if _, r = decoded(t, p); r["windows"].([]any)[0].(map[string]any)["stop"] != "20:00" {
		t.Fatal(string(p.Rules))
	}
}

func TestPhonePolicyRefusesDivergentPlatformState(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	if _, err := PhonePolicy(sample(), shellPolicy("", ""), "alex", now); err == nil {
		t.Fatal("captain mismatch accepted")
	}
	broken := shellPolicy("alex", "")
	broken.Rules = json.RawMessage(`{}`)
	if _, err := PhonePolicy(sample(), broken, "alex", now); err == nil {
		t.Fatal("missing override accepted")
	}
}
