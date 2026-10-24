package usign

import (
	"os"
	"strings"
	"testing"
)

// testdata/ was written by OpenWrt's own "usign -G" and "usign -S" (disposable key).
func TestSecretKeyFromRealUsignSignsLikeUsign(t *testing.T) {
	sec, _ := os.ReadFile("testdata/router.sec")
	pubFile, _ := os.ReadFile("testdata/router.pub")
	msg, _ := os.ReadFile("testdata/message.txt")
	sigFile, _ := os.ReadFile("testdata/message.sig")
	sk, err := ParseSecretKey(sec)
	if err != nil {
		t.Fatal(err)
	}
	pubText := strings.Split(strings.TrimSpace(string(pubFile)), "\n")[1]
	if sk.Public() != pubText {
		t.Fatalf("public key %s, usign wrote %s", sk.Public(), pubText)
	}
	want := strings.Split(strings.TrimSpace(string(sigFile)), "\n")[1]
	if got := sk.Sign(msg); got != want {
		t.Fatalf("signature differs from usign's (Ed25519 is deterministic): %s vs %s", got, want)
	}
	pub, _ := ParsePublicKey(pubText)
	if !pub.Verify(sk.Sign([]byte("other")), []byte("other")) {
		t.Fatal("own signature does not verify")
	}
}

func TestSecretKeyRejectsDamageAndPasswords(t *testing.T) {
	sec, _ := os.ReadFile("testdata/router.sec")
	lines := strings.Split(strings.TrimSpace(string(sec)), "\n")
	for name, file := range map[string]string{
		"empty":        "",
		"no comment":   lines[1],
		"public key":   lines[0] + "\nRWQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"flipped byte": lines[0] + "\n" + flip(lines[1], 80),
		"kdf rounds":   lines[0] + "\n" + flip(lines[1], 7),
	} {
		if _, err := ParseSecretKey([]byte(file)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func flip(b64 string, i int) string {
	b := []byte(b64)
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	return string(b)
}
