package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGoogleProviderVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		kid := "first"
		if fetches.Load() > 1 {
			kid = "rotated"
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer server.Close()
	p, err := newGoogleProvider(GoogleConfig{ClientID: "client", ClientSecret: "example", Origin: "https://lms.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	p.keysURL, p.client = server.URL, server.Client()
	now := time.Now()
	nonce := strings.Repeat("a", 64)
	good := googleClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "accounts.google.com", Subject: "subject", Audience: jwt.ClaimStrings{"client"},
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}, Nonce: nonce}
	sign := func(method jwt.SigningMethod, kid string, claims googleClaims, signingKey any) string {
		t.Helper()
		token := jwt.NewWithClaims(method, claims)
		token.Header["kid"], token.Header["jku"] = kid, "https://foreign.invalid/ignored"
		raw, err := token.SignedString(signingKey)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	ctx := context.Background()
	valid := sign(jwt.SigningMethodRS256, "first", good, key)
	if c, err := p.verify(ctx, valid, nonce); err != nil || c.Subject != "subject" || c.Issuer != googleIssuer {
		t.Fatal("valid Google identity rejected")
	}
	if time.Until(p.keysUntil) > time.Hour {
		t.Fatal("JWKS cache exceeds maximum")
	}
	for name, corrupt := range map[string]func(*googleClaims){
		"issuer":             func(c *googleClaims) { c.Issuer = "https://foreign.invalid" },
		"audience":           func(c *googleClaims) { c.Audience = jwt.ClaimStrings{"other"} },
		"multiple audiences": func(c *googleClaims) { c.Audience = jwt.ClaimStrings{"client", "other"} },
		"authorized party":   func(c *googleClaims) { c.Authorized = "other" },
		"expiry":             func(c *googleClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second)) },
		"nonce":              func(c *googleClaims) { c.Nonce = strings.Repeat("b", 64) },
		"subject":            func(c *googleClaims) { c.Subject = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := good
			corrupt(&c)
			if _, err := p.verify(ctx, sign(jwt.SigningMethodRS256, "first", c, key), nonce); err == nil {
				t.Fatal("invalid Google claim accepted")
			}
		})
	}
	if _, err := p.verify(ctx, sign(jwt.SigningMethodHS256, "first", good, []byte("synthetic")), nonce); err == nil {
		t.Fatal("alternate signature algorithm accepted")
	}
	parts := strings.Split(valid, ".")
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	signature[0] ^= 1
	parts[2] = base64.RawURLEncoding.EncodeToString(signature)
	if _, err := p.verify(ctx, strings.Join(parts, "."), nonce); err == nil {
		t.Fatal("altered signature accepted")
	}
	if _, err := p.verify(ctx, sign(jwt.SigningMethodRS256, "unknown", good, key), nonce); err == nil || fetches.Load() != 1 {
		t.Fatal("unknown key bypassed refresh throttle")
	}
	p.lastFetch = now.Add(-time.Minute)
	if _, err := p.verify(ctx, sign(jwt.SigningMethodRS256, "rotated", good, key), nonce); err != nil || fetches.Load() != 2 {
		t.Fatal("new key not loaded")
	}
	if _, err := p.verify(ctx, valid, nonce); err == nil {
		t.Fatal("removed key still trusted")
	}
}
