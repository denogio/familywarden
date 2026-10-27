package configuration

import (
	"errors"
	"strings"

	"github.com/denogio/familywarden/core/internal/rules"
)

// Recovery is deliberately separate from the administrator API. The native
// release entry point authenticates the root password and serializes/throttles
// attempts; the core owns the phrase, pact status and cooling-off decisions.
func (a *Authority) Recovery(action string, raw []byte) map[string]any {
	fail := func(err error) map[string]any { return map[string]any{"error": err.Error()} }
	switch action {
	case "status":
		return a.API("configuration_sail_status", nil)
	case "cancel":
		var req struct{}
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		return a.releaseMutation("configuration_sail_cancel")
	case "request":
		var req struct {
			Answer string `json:"answer"`
		}
		if err := decode(raw, &req); err != nil {
			return fail(err)
		}
		answer := strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			if (r >= 'a' && r <= 'z') || r == ' ' {
				return r
			}
			return -1
		}, req.Answer)
		if strings.Trim(answer, " ") != "press on" {
			return fail(errors.New("The sirens sing on. Still tied to the mast."))
		}
		return a.releaseMutation("configuration_sail_request")
	default:
		return fail(errors.New("unknown protected release action"))
	}
}

func (a *Authority) releaseMutation(method string) map[string]any {
	reply := a.API(method, []byte(`{}`))
	if reply["error"] != nil {
		return reply
	}
	return a.API("configuration_sail_status", nil)
}

func (a *Authority) releaseStatus(c rules.Configuration) map[string]any {
	now := a.Now()
	out := map[string]any{"pact": Active(c.Pact, now), "captain": "", "type": "", "pact_until": int64(0), "window": " -", "delay": int64(0), "release_at": int64(0), "now": now.Unix()}
	if p := c.Pact; p != nil {
		out["captain"], out["type"] = p.Captain, p.Type
		out["pact_until"], out["window"] = integer(p.Until), p.WindowDay+" "+p.WindowStart+"-"+p.WindowStop
		out["delay"], out["release_at"] = integer(p.DelaySeconds), integer(p.ReleaseAt)
	}
	return out
}
