package protocol

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/usign"
)

type signer struct {
	priv ed25519.PrivateKey
	id   [8]byte
	pub  usign.PublicKey
}

func newSigner(t *testing.T, id byte) signer {
	pub, priv, _ := ed25519.GenerateKey(nil)
	s := signer{priv: priv, id: [8]byte{id}}
	k, err := usign.ParsePublicKey(usign.Format(pub, s.id))
	if err != nil {
		t.Fatal(err)
	}
	s.pub = k
	return s
}

func (s signer) sign(kind, raw string) string {
	return usign.Sign(s.priv, s.id, Message(kind, []byte(raw)))
}

var now = time.Unix(1792540000, 0)

func request(minutes int, created int64) string {
	return fmt.Sprintf(`{"type":"request","v":1,"request_id":"r1","device":"phone-a","profile":"alex","package":"com.roblox.client","minutes":%d,"created":%d}`, minutes, created)
}

func TestRequest(t *testing.T) {
	child, other := newSigner(t, 1), newSigner(t, 2)
	ok := request(30, now.Unix())
	if _, err := VerifyRequest([]byte(ok), child.sign("request", ok), child.pub, "phone-a", "alex", now); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
	cases := []struct {
		name, raw, sig, device, profile string
		want                            error
	}{
		{"other key", ok, other.sign("request", ok), "phone-a", "alex", ErrBadSignature},
		{"grant signature reused as request", ok, child.sign("grant", ok), "phone-a", "alex", ErrBadSignature},
		{"other device", ok, child.sign("request", ok), "phone-b", "alex", ErrMalformed},
		{"other profile", ok, child.sign("request", ok), "phone-a", "sam", ErrWrongProfile},
		{"zero minutes", request(0, now.Unix()), child.sign("request", request(0, now.Unix())), "phone-a", "alex", ErrInvalidMinutes},
		{"too many minutes", request(241, now.Unix()), child.sign("request", request(241, now.Unix())), "phone-a", "alex", ErrInvalidMinutes},
		{"older than an hour", request(30, now.Add(-time.Hour).Unix()), child.sign("request", request(30, now.Add(-time.Hour).Unix())), "phone-a", "alex", ErrRequestExpired},
		{"from the future", request(30, now.Add(10*time.Minute).Unix()), child.sign("request", request(30, now.Add(10*time.Minute).Unix())), "phone-a", "alex", ErrRequestExpired},
	}
	for _, c := range cases {
		if _, err := VerifyRequest([]byte(c.raw), c.sig, child.pub, c.device, c.profile, now); err != c.want {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	// Signed bytes are verified before parsing: altering a single byte breaks it.
	tampered := request(31, now.Unix())
	if _, err := VerifyRequest([]byte(tampered), child.sign("request", ok), child.pub, "phone-a", "alex", now); err != ErrBadSignature {
		t.Errorf("tampered minutes: got %v", err)
	}
}

func grant(issuer, profile string, minutes int, from, to int64) string {
	return fmt.Sprintf(`{"type":"grant","v":1,"grant_id":"g1","request_id":"r1","issuer":%q,"profile":%q,"package":"com.roblox.client","minutes":%d,"not_before":%d,"not_after":%d}`,
		issuer, profile, minutes, from, to)
}

func TestGrant(t *testing.T) {
	router, parent, stranger := newSigner(t, 1), newSigner(t, 2), newSigner(t, 3)
	issuers := map[string]usign.PublicKey{"router": router.pub, "dad-phone": parent.pub}
	from, to := now.Add(-time.Minute).Unix(), now.Add(-time.Minute).Add(GrantLifetime).Unix()

	for _, who := range []struct {
		id string
		s  signer
	}{{"router", router}, {"dad-phone", parent}} {
		raw := grant(who.id, "alex", 20, from, to)
		g, err := VerifyGrant([]byte(raw), who.s.sign("grant", raw), issuers, "alex", now)
		if err != nil || g.Minutes != 20 {
			t.Fatalf("%s grant refused: %v", who.id, err)
		}
	}
	ok := grant("dad-phone", "alex", 20, from, to)
	cases := []struct {
		name, raw, sig, profile string
		at                      time.Time
		want                    error
	}{
		{"unknown issuer", grant("uncle-phone", "alex", 20, from, to), stranger.sign("grant", grant("uncle-phone", "alex", 20, from, to)), "alex", now, ErrNotParent},
		{"issuer claims a parent but signs with another key", ok, stranger.sign("grant", ok), "alex", now, ErrBadSignature},
		{"another child's grant", ok, parent.sign("grant", ok), "sam", now, ErrWrongProfile},
		{"after not_after", ok, parent.sign("grant", ok), "alex", time.Unix(to, 0), ErrGrantExpired},
		{"too early beyond skew", ok, parent.sign("grant", ok), "alex", time.Unix(from, 0).Add(-6 * time.Minute), ErrGrantExpired},
		{"longer than two hours", grant("dad-phone", "alex", 20, from, from+int64((3*time.Hour).Seconds())), parent.sign("grant", grant("dad-phone", "alex", 20, from, from+int64((3*time.Hour).Seconds()))), "alex", now, ErrMalformed},
		{"request signature reused as grant", ok, parent.sign("request", ok), "alex", now, ErrBadSignature},
	}
	for _, c := range cases {
		if _, err := VerifyGrant([]byte(c.raw), c.sig, issuers, c.profile, c.at); err != c.want {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	// Within the skew window a parent phone whose clock runs slightly ahead still works.
	if _, err := VerifyGrant([]byte(ok), parent.sign("grant", ok), issuers, "alex", time.Unix(from, 0).Add(-4*time.Minute)); err != nil {
		t.Errorf("grant within clock skew refused: %v", err)
	}
}
