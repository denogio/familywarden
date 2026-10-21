// Package protocol verifies the signed app-control messages exchanged between
// phones and the router (plans/app-control-protocol.md). A message is signed over
// a type-specific prefix line plus the exact JSON bytes sent, so a signature for
// one message type can never be replayed as another, and no canonical JSON form
// is needed across Kotlin and Go.
package protocol

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/denogio/familywarden/core/internal/usign"
)

const (
	MaxMinutes    = 240
	RequestTTL    = time.Hour     // D7: an unanswered request expires after 1 hour
	GrantLifetime = 2 * time.Hour // D3: a grant is valid 2 hours after approval
	ClockSkew     = 5 * time.Minute
)

// Error is a protocol error code from plans/app-control-protocol.md §7.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrMalformed      Error = "malformed"
	ErrBadSignature   Error = "bad_signature"
	ErrWrongProfile   Error = "wrong_profile"
	ErrNotParent      Error = "not_parent"
	ErrRequestExpired Error = "request_expired"
	ErrGrantExpired   Error = "grant_expired"
	ErrInvalidMinutes Error = "invalid_minutes"
	ErrUnknownPackage Error = "unknown_package"
)

// Message is what gets signed: "familywarden-<kind>-v1\n" + raw JSON.
func Message(kind string, raw []byte) []byte {
	return append([]byte("familywarden-"+kind+"-v1\n"), raw...)
}

type Request struct {
	Type      string `json:"type"`
	V         int    `json:"v"`
	RequestID string `json:"request_id"`
	Device    string `json:"device"`
	Profile   string `json:"profile"`
	Package   string `json:"package"`
	Minutes   int    `json:"minutes"`
	Created   int64  `json:"created"`
}

type Grant struct {
	Type      string  `json:"type"`
	V         int     `json:"v"`
	GrantID   string  `json:"grant_id"`
	RequestID *string `json:"request_id"`
	Issuer    string  `json:"issuer"`
	Profile   string  `json:"profile"`
	Package   string  `json:"package"`
	Minutes   int     `json:"minutes"`
	NotBefore int64   `json:"not_before"`
	NotAfter  int64   `json:"not_after"`
}

// verified checks the signature over the exact bytes before anything parses them.
func verified(kind string, raw []byte, sig string, key usign.PublicKey, into any) error {
	if !key.Verify(sig, Message(kind, raw)) {
		return ErrBadSignature
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(into); err != nil {
		return ErrMalformed
	}
	return nil
}

// VerifyRequest checks a child's request signed by that phone's approved key.
// now is the router's clock.
func VerifyRequest(raw []byte, sig string, deviceKey usign.PublicKey, device, profile string, now time.Time) (Request, error) {
	var r Request
	if err := verified("request", raw, sig, deviceKey, &r); err != nil {
		return r, err
	}
	switch {
	case r.Type != "request" || r.V != 1 || r.RequestID == "" || r.Device != device:
		return r, ErrMalformed
	case r.Profile != profile:
		return r, ErrWrongProfile
	case r.Package == "":
		return r, ErrUnknownPackage
	case r.Minutes < 1 || r.Minutes > MaxMinutes:
		return r, ErrInvalidMinutes
	}
	created := time.Unix(r.Created, 0)
	if now.Sub(created) >= RequestTTL || created.Sub(now) > ClockSkew {
		return r, ErrRequestExpired
	}
	return r, nil
}

// VerifyGrant checks a grant on the child's side (or on the router before opening
// a network ticket). issuers holds the router key under "router" and the keys of
// parents allowed for this profile under their device ids. now must be the
// router-anchored clock, never the phone's wall clock alone.
func VerifyGrant(raw []byte, sig string, issuers map[string]usign.PublicKey, profile string, now time.Time) (Grant, error) {
	var g Grant
	// The issuer field selects the key, so read it before verifying, then verify
	// the same bytes with that key before trusting any other field.
	var head struct {
		Issuer string `json:"issuer"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return g, ErrMalformed
	}
	key, ok := issuers[head.Issuer]
	if !ok {
		return g, ErrNotParent
	}
	if err := verified("grant", raw, sig, key, &g); err != nil {
		return g, err
	}
	switch {
	case g.Type != "grant" || g.V != 1 || g.GrantID == "" || g.Issuer != head.Issuer:
		return g, ErrMalformed
	case g.Profile != profile:
		return g, ErrWrongProfile
	case g.Package == "":
		return g, ErrUnknownPackage
	case g.Minutes < 1 || g.Minutes > MaxMinutes:
		return g, ErrInvalidMinutes
	}
	from, to := time.Unix(g.NotBefore, 0), time.Unix(g.NotAfter, 0)
	if !to.After(from) || to.Sub(from) > GrantLifetime {
		return g, ErrMalformed
	}
	if now.Add(ClockSkew).Before(from) || !now.Before(to) {
		return g, ErrGrantExpired
	}
	return g, nil
}
