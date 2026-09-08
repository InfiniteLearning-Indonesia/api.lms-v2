package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"testing"
	"time"
)

func TestBridgeVerifier(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	v := Verifier{Issuer: "https://issuer.invalid", Audience: "test", Keys: map[string]ed25519.PublicKey{"one": pub}}
	now := time.Now().Truncate(time.Second)
	active := true
	base := Assertion{RegisteredClaims: jwt.RegisteredClaims{Issuer: v.Issuer, Audience: jwt.ClaimStrings{v.Audience}, Subject: "stable-subject", ID: strings.Repeat("a", 64), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, Purpose: "login", Nonce: strings.Repeat("b", 64), Version: 1, Active: &active}
	sign := func(c Assertion, kid string, signer ed25519.PrivateKey) string {
		x := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
		x.Header["kid"] = kid
		x.Header["typ"] = "lms-bridge+jwt"
		raw, _ := x.SignedString(signer)
		return raw
	}
	raw := sign(base, "one", key)
	if _, err := v.Verify(raw, "login", now); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Assertion){func(c *Assertion) { c.Issuer = "wrong" }, func(c *Assertion) { c.Audience = jwt.ClaimStrings{"wrong"} }, func(c *Assertion) { c.Purpose = "reset" }, func(c *Assertion) { c.Nonce = "" }, func(c *Assertion) { c.Subject = "" }, func(c *Assertion) { c.Active = nil }, func(c *Assertion) { c.Version = 0 }, func(c *Assertion) { c.ExpiresAt = nil }, func(c *Assertion) { c.NotBefore = nil }, func(c *Assertion) { c.IssuedAt = nil }, func(c *Assertion) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second)) }, func(c *Assertion) { c.ExpiresAt = jwt.NewNumericDate(now.Add(2 * time.Minute)) }, func(c *Assertion) { c.NotBefore = jwt.NewNumericDate(now.Add(time.Second)) }, func(c *Assertion) { c.ID = "short" }} {
		c := base
		change(&c)
		if _, err := v.Verify(sign(c, "one", key), "login", now); err == nil {
			t.Fatal("invalid assertion accepted")
		}
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	for _, bad := range []string{raw + "tampered", sign(base, "unknown", key), sign(base, "one", other), "", strings.Repeat("x", 8193)} {
		if _, err := v.Verify(bad, "login", now); err == nil {
			t.Fatal("untrusted input accepted")
		}
	}
	if _, err := v.Verify(raw, "login", now.Add(time.Minute)); err == nil {
		t.Fatal("expiry accepted")
	}
	projection := base
	projection.Purpose = "account_state"
	projection.Nonce = ""
	if _, err := v.Verify(sign(projection, "one", key), "account_state", now); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"reset", ""} {
		if _, err := v.Verify(raw, purpose, now); err == nil {
			t.Fatal("unknown purpose")
		}
	}
	for _, invalid := range []Verifier{{}, {Issuer: v.Issuer, Audience: v.Audience, Keys: map[string]ed25519.PublicKey{"one": {1}}}} {
		if _, err := invalid.Verify(raw, "login", now); err == nil {
			t.Fatal("invalid trust config")
		}
	}
	for _, bad := range []string{"x.!.x", "x.e30.x", "x.bnVsbA.x"} {
		if _, err := v.Verify(bad, "login", now); err == nil {
			t.Fatal("malformed claims")
		}
	}
	for _, alg := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodNone} {
		x := jwt.NewWithClaims(alg, base)
		x.Header["kid"] = "one"
		x.Header["typ"] = "lms-bridge+jwt"
		var signingKey any = []byte("not-an-ed25519-key")
		if alg == jwt.SigningMethodNone {
			signingKey = jwt.UnsafeAllowNoneSignatureType
		}
		bad, err := x.SignedString(signingKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = v.Verify(bad, "login", now); err == nil {
			t.Fatal("algorithm confusion")
		}
	}
	x := jwt.NewWithClaims(jwt.SigningMethodEdDSA, base)
	x.Header["kid"] = "one"
	x.Header["typ"] = "lms-bridge+jwt"
	x.Header["jku"] = "https://attacker.invalid/key"
	bad, _ := x.SignedString(key)
	if _, err := v.Verify(bad, "login", now); err == nil {
		t.Fatal("remote key header")
	}
}
