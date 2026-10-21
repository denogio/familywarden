package usign

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestRoundTripAndTampering(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	id := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	text := Format(pub, id)
	k, err := ParsePublicKey(text)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("familywarden-request-v1\n{}\n")
	sig := Sign(priv, id, msg)
	if !k.Verify(sig, msg) {
		t.Fatal("valid signature rejected")
	}
	if k.Verify(sig, []byte("familywarden-grant-v1\n{}\n")) {
		t.Fatal("signature accepted for another message")
	}
	if k.Verify(Sign(priv, [8]byte{9}, msg), msg) {
		t.Fatal("signature with a different key id accepted")
	}
	_, other, _ := ed25519.GenerateKey(nil)
	if k.Verify(Sign(other, id, msg), msg) {
		t.Fatal("signature by another key with a copied key id accepted")
	}
	sum := sha256.Sum256([]byte(text))
	if k.Fingerprint() != hex.EncodeToString(sum[:]) {
		t.Fatal("fingerprint must be SHA-256 of the full base64 key, as the app shows it")
	}
}

func TestRejectsMalformedKeys(t *testing.T) {
	for _, s := range []string{"", "not base64!", "RWQ=", Format(make([]byte, 31), [8]byte{})} {
		if _, err := ParsePublicKey(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
