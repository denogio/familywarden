// Package usign handles OpenWrt usign (Ed25519) keys and signatures, the format
// the router and the FamilyWarden phones already use:
//
//	public key: base64("Ed" | 8-byte key id | 32-byte key)
//	signature:  base64("Ed" | 8-byte key id | 64-byte signature over the raw message)
package usign

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

type PublicKey struct {
	KeyID [8]byte
	Key   ed25519.PublicKey
	text  string
}

var errFormat = errors.New("not a usign Ed25519 public key")

func ParsePublicKey(b64 string) (PublicKey, error) {
	b64 = strings.TrimSpace(b64)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != 42 || raw[0] != 'E' || raw[1] != 'd' {
		return PublicKey{}, errFormat
	}
	var k PublicKey
	copy(k.KeyID[:], raw[2:10])
	k.Key = ed25519.PublicKey(append([]byte(nil), raw[10:]...))
	k.text = b64
	return k, nil
}

// Fingerprint is the SHA-256 of the full base64 key, as shown in LuCI and the app.
// usign's own key id is caller-chosen and must never be used for identity.
func (k PublicKey) Fingerprint() string {
	sum := sha256.Sum256([]byte(k.text))
	return hex.EncodeToString(sum[:])
}

// Verify reports whether sig is a valid usign signature by k over message.
func (k PublicKey) Verify(sig string, message []byte) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sig))
	if err != nil || len(raw) != 74 || raw[0] != 'E' || raw[1] != 'd' {
		return false
	}
	if string(raw[2:10]) != string(k.KeyID[:]) {
		return false
	}
	return ed25519.Verify(k.Key, message, raw[10:])
}

// Sign creates a usign signature; used by the router key and by tests.
func Sign(priv ed25519.PrivateKey, keyID [8]byte, message []byte) string {
	raw := append([]byte("Ed"), keyID[:]...)
	return base64.StdEncoding.EncodeToString(append(raw, ed25519.Sign(priv, message)...))
}

// Format encodes a public key in usign's text form.
func Format(pub ed25519.PublicKey, keyID [8]byte) string {
	raw := append([]byte("Ed"), keyID[:]...)
	return base64.StdEncoding.EncodeToString(append(raw, pub...))
}
