package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/denogio/familywarden/core/internal/configuration"
	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/protocol"
)

func (s *Service) appRule(d Device, raw []byte, status bool) object {
	var m struct {
		Type     string `json:"type"`
		V        int    `json:"v"`
		Device   string `json:"device"`
		Profile  string `json:"profile"`
		Package  string `json:"package"`
		Mode     string `json:"mode"`
		Minutes  int    `json:"minutes"`
		Revision string `json:"apps_rev"`
		Nonce    string `json:"nonce"`
		Created  int64  `json:"created"`
		Expires  int64  `json:"expires"`
	}
	if !decodeStrict(raw, &m, "type v device profile package mode minutes apps_rev nonce created expires", "") || m.Type != "rule" || m.V != 1 || m.Device != d.ID ||
		!validNonce.MatchString(m.Nonce) || m.Created <= 0 || m.Created > maxEpoch-600 || m.Expires != m.Created+600 ||
		m.Created > s.Now().Add(protocol.ClockSkew).Unix() || len(m.Revision) > 128 {
		return refuse("malformed")
	}
	authority, ok := s.ad.(interface {
		AppRule(configuration.RuleChange, bool, func(platform.Context) bool) map[string]any
	})
	if !ok {
		return refuse("configuration_required")
	}
	owner := sha256.Sum256([]byte(d.ID + "\n" + d.PubKey))
	digest := sha256.Sum256(raw)
	r := authority.AppRule(configuration.RuleChange{Profile: m.Profile, Package: m.Package, Mode: m.Mode, Minutes: m.Minutes, Revision: m.Revision, Expires: m.Expires, Nonce: m.Nonce, Owner: hex.EncodeToString(owner[:]), Digest: hex.EncodeToString(digest[:])}, status,
		func(ctx platform.Context) bool { return slices.Contains(effectiveApproves(d, ctx), m.Profile) })
	out := object{}
	for k, v := range r {
		out = append(out, field{k, v})
	}
	return out
}
