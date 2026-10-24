// Package protocol defines app-control signing domains, bounds and error codes.
// The active request/decision handler is in identity/requests.go. Messages sign
// a type-specific prefix line plus the exact JSON bytes sent, so a signature for
// one message type can never be replayed as another, and no canonical JSON form
// is needed across Kotlin and Go.
package protocol

import "time"

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
