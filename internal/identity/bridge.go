package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"strings"
	"time"
)

type Assertion struct {
	jwt.RegisteredClaims
	Purpose string `json:"purpose"`
	Nonce   string `json:"nonce,omitempty"`
	Version int64  `json:"version"`
	Active  *bool  `json:"active"`
}

type Verifier struct {
	Issuer, Audience string
	Keys             map[string]ed25519.PublicKey
}

// Verify never follows a key URL or chooses an algorithm from untrusted input.
// The returned claims prove a bounded authority observation, not eternal access.
func (v Verifier) Verify(raw, purpose string, now time.Time) (Assertion, error) {
	var claims Assertion
	if v.Issuer == "" || v.Audience == "" || len(v.Keys) == 0 || len(raw) > 8192 || (purpose != "login" && purpose != "account_state") {
		return claims, ErrUnauthenticated
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return claims, ErrUnauthenticated
	}
	// Reject unknown/authority-bearing fields even if a trusted issuer signs them.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrUnauthenticated
	}
	d := json.NewDecoder(bytes.NewReader(payload))
	d.DisallowUnknownFields()
	if d.Decode(&claims) != nil {
		return Assertion{}, ErrUnauthenticated
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Assertion{}, ErrUnauthenticated
	}
	token, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		key := v.Keys[kid]
		if !ok || len(key) != ed25519.PublicKeySize || t.Header["typ"] != "lms-bridge+jwt" || len(t.Header) != 3 {
			return nil, errors.New("UNTRUSTED_KEY")
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(v.Issuer), jwt.WithAudience(v.Audience), jwt.WithExpirationRequired(), jwt.WithNotBeforeRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !token.Valid || claims.Purpose != purpose || claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.NotBefore == nil || claims.Active == nil || claims.Version < 1 || claims.Subject == "" || len(claims.Subject) > 128 || !opaque.MatchString(claims.ID) || len(claims.Audience) != 1 || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > time.Minute || !claims.ExpiresAt.After(claims.IssuedAt.Time) || claims.NotBefore.Before(claims.IssuedAt.Time) || (purpose == "login" && (!*claims.Active || !opaque.MatchString(claims.Nonce))) || (purpose == "account_state" && claims.Nonce != "") {
		return Assertion{}, ErrUnauthenticated
	}
	return claims, nil
}
