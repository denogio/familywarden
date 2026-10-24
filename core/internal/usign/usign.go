// Package usign handles OpenWrt usign (Ed25519) keys and signatures, the format
// the router and the FamilyWarden phones already use:
//
//	public key: base64("Ed" | 8-byte key id | 32-byte key)
//	signature:  base64("Ed" | 8-byte key id | 64-byte signature over the raw message)
package usign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
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

// SecretKey is an unencrypted usign secret key, as written by "usign -G".
type SecretKey struct {
	KeyID [8]byte
	Key   ed25519.PrivateKey
}

// ParseSecretKey reads usign's secret key file ("untrusted comment" line plus
// base64): "Ed" "BK" | kdf rounds (4) | salt (16) | checksum (8) | key id (8) | key (64).
// Password-protected keys (kdf rounds > 0) and damaged keys are refused.
func ParseSecretKey(file []byte) (SecretKey, error) {
	var sk SecretKey
	lines := strings.Split(strings.TrimSpace(string(file)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "untrusted comment:") {
		return sk, errors.New("not a usign secret key file")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(raw) != 104 || string(raw[:4]) != "EdBK" {
		return sk, errors.New("not a usign Ed25519 secret key")
	}
	if !bytes.Equal(raw[4:8], []byte{0, 0, 0, 0}) {
		return sk, errors.New("password-protected usign keys are not supported")
	}
	key := raw[40:104]
	sum := sha512.Sum512(key)
	if !bytes.Equal(sum[:8], raw[24:32]) {
		return sk, errors.New("usign secret key checksum mismatch")
	}
	copy(sk.KeyID[:], raw[32:40])
	sk.Key = ed25519.PrivateKey(append([]byte(nil), key...))
	// The public half must match the private seed, or signatures would never verify.
	if !bytes.Equal(ed25519.NewKeyFromSeed(sk.Key.Seed()), sk.Key) {
		return SecretKey{}, errors.New("inconsistent usign secret key")
	}
	return sk, nil
}

// Sign signs message the way "usign -S" does; returns the base64 signature line.
func (sk SecretKey) Sign(message []byte) string { return Sign(sk.Key, sk.KeyID, message) }

// Public returns the matching public key in usign text form.
func (sk SecretKey) Public() string {
	return Format(sk.Key.Public().(ed25519.PublicKey), sk.KeyID)
}

// Format encodes a public key in usign's text form.
func Format(pub ed25519.PublicKey, keyID [8]byte) string {
	raw := append([]byte("Ed"), keyID[:]...)
	return base64.StdEncoding.EncodeToString(append(raw, pub...))
}
