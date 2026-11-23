package identity

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const googleIssuer = "https://accounts.google.com"
const googleCallbackPath = "/auth/google/callback"

type GoogleConfig struct {
	ClientID, ClientSecret, Origin string
	FlowKey                        []byte
}

type googleProvider struct {
	clientID, secret, redirectURI   string
	authorizeURL, tokenURL, keysURL string
	client                          *http.Client
	mu                              sync.Mutex
	keys                            map[string]*rsa.PublicKey
	keysUntil, lastFetch            time.Time
}

func newGoogleProvider(config GoogleConfig) (*googleProvider, error) {
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery ||
		config.Origin != "https://"+origin.Host || config.ClientID == "" || config.ClientSecret == "" {
		return nil, ErrPasswordInput
	}
	return &googleProvider{
		clientID: config.ClientID, secret: config.ClientSecret, redirectURI: config.Origin + googleCallbackPath,
		authorizeURL: googleIssuer + "/o/oauth2/v2/auth", tokenURL: "https://oauth2.googleapis.com/token",
		keysURL: "https://www.googleapis.com/oauth2/v3/certs",
		client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func (p *googleProvider) authorizationURL(state, nonce, verifier string) string {
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"client_id": {p.clientID}, "redirect_uri": {p.redirectURI}, "response_type": {"code"},
		"scope": {"openid email"}, "state": {state}, "nonce": {nonce},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}
	return p.authorizeURL + "?" + query.Encode()
}

type googleClaims struct {
	jwt.RegisteredClaims
	Nonce         string `json:"nonce"`
	Authorized    string `json:"azp"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func (p *googleProvider) exchange(ctx context.Context, code, nonce, verifier string) (googleClaims, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier},
		"client_id": {p.clientID}, "client_secret": {p.secret}, "redirect_uri": {p.redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleClaims{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		IDToken string `json:"id_token"`
	}
	if _, err := p.readResponse(req, &response); err != nil {
		return googleClaims{}, err
	}
	return p.verify(ctx, response.IDToken, nonce)
}

var errGoogleUnavailable = errors.New("GOOGLE_PROVIDER_UNAVAILABLE")

func (p *googleProvider) readResponse(req *http.Request, out any) (http.Header, error) {
	res, err := p.client.Do(req)
	if err != nil {
		return nil, errGoogleUnavailable
	}
	defer res.Body.Close()
	if req.Method == http.MethodPost && res.StatusCode == http.StatusBadRequest {
		return nil, ErrUnauthenticated
	}
	if res.StatusCode != http.StatusOK {
		return nil, errGoogleUnavailable
	}
	const limit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(body) > limit || json.Unmarshal(body, out) != nil {
		return nil, errGoogleUnavailable
	}
	return res.Header, nil
}

func (p *googleProvider) verify(ctx context.Context, raw, nonce string) (googleClaims, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return googleClaims{}, ErrUnauthenticated
	}
	var c googleClaims
	token, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || len(kid) == 0 || len(kid) > 128 {
			return nil, ErrUnauthenticated
		}
		return p.key(ctx, kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(p.clientID), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if errors.Is(err, errGoogleUnavailable) {
		return googleClaims{}, errGoogleUnavailable
	}
	if err != nil || !token.Valid || (c.Issuer != googleIssuer && c.Issuer != "accounts.google.com") ||
		len(c.Audience) != 1 || (c.Authorized != "" && c.Authorized != p.clientID) ||
		c.IssuedAt == nil || c.ExpiresAt == nil || !c.ExpiresAt.After(c.IssuedAt.Time) ||
		len(c.Subject) == 0 || len(c.Subject) > 255 || strings.ContainsAny(c.Subject, "\x00\r\n") ||
		!opaque.MatchString(nonce) || c.Nonce != nonce {
		return googleClaims{}, ErrUnauthenticated
	}
	c.Issuer = googleIssuer
	return c, nil
}

func (p *googleProvider) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	fresh := now.Before(p.keysUntil)
	if key := p.keys[kid]; fresh && key != nil {
		return key, nil
	}
	// Cap refreshes triggered by unknown key IDs. A failed refresh can be
	// retried after the same short backoff, and expired keys fail closed.
	if now.Sub(p.lastFetch) < 5*time.Second {
		if fresh {
			return nil, ErrUnauthenticated
		}
		return nil, errGoogleUnavailable
	}
	p.lastFetch = now
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.keysURL, nil)
	if err != nil {
		return nil, errGoogleUnavailable
	}
	var set struct {
		Keys []struct {
			ID  string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	headers, err := p.readResponse(req, &set)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, jwk := range set.Keys {
		if jwk.Kty != "RSA" || jwk.Use != "sig" || jwk.Alg != "RS256" || jwk.ID == "" || len(jwk.ID) > 128 {
			continue
		}
		n, nErr := base64.RawURLEncoding.DecodeString(jwk.N)
		e, eErr := base64.RawURLEncoding.DecodeString(jwk.E)
		if nErr != nil || eErr != nil || len(n) > 512 || len(e) == 0 || len(e) > 4 {
			return nil, errGoogleUnavailable
		}
		modulus := new(big.Int).SetBytes(n)
		exponent := new(big.Int).SetBytes(e).Int64()
		if modulus.BitLen() < 2048 || modulus.Bit(0) != 1 || exponent < 3 || exponent > 1<<31-1 || exponent%2 == 0 || keys[jwk.ID] != nil {
			return nil, errGoogleUnavailable
		}
		keys[jwk.ID] = &rsa.PublicKey{N: modulus, E: int(exponent)}
	}
	if len(keys) == 0 {
		return nil, errGoogleUnavailable
	}
	p.keys, p.keysUntil = keys, now.Add(googleKeyTTL(headers))
	if key := keys[kid]; key != nil {
		return key, nil
	}
	return nil, ErrUnauthenticated
}

func googleKeyTTL(headers http.Header) time.Duration {
	var seconds int64
	for _, item := range strings.Split(headers.Get("Cache-Control"), ",") {
		if directive := strings.TrimSpace(item); directive == "no-store" || directive == "no-cache" {
			return 0
		} else if value, ok := strings.CutPrefix(directive, "max-age="); ok {
			seconds, _ = strconv.ParseInt(value, 10, 32)
		}
	}
	age, _ := strconv.ParseInt(headers.Get("Age"), 10, 32)
	return time.Duration(min(3600, max(0, seconds-max(0, age)))) * time.Second
}
