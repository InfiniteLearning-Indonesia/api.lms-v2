//go:build password_rehearsal

package identity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
)

const syntheticGoogleClient = "synthetic-google-client.apps.googleusercontent.com"
const syntheticGoogleSecret = "synthetic-google-secret"

type googleCode struct {
	nonce, challenge, email, subject string
}

type googleFixtureTransport struct {
	mu          sync.Mutex
	codes       map[string]googleCode
	exchanges   int
	key         *rsa.PrivateKey
	redirectURI string
}

func (s *googleFixtureTransport) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exchanges
}

func (s *googleFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body any
	switch {
	case r.URL.String() == "https://www.googleapis.com/oauth2/v3/certs" && r.Method == http.MethodGet:
		body = map[string]any{"keys": []map[string]string{{
			"kid": "fixture", "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
		}}}
	case r.URL.String() == "https://oauth2.googleapis.com/token" && r.Method == http.MethodPost:
		if r.ParseForm() != nil || r.PostForm.Get("client_id") != syntheticGoogleClient ||
			r.PostForm.Get("client_secret") != syntheticGoogleSecret || r.PostForm.Get("grant_type") != "authorization_code" ||
			r.PostForm.Get("redirect_uri") != s.redirectURI {
			return nil, errors.New("fixture exchange contract")
		}
		s.mu.Lock()
		s.exchanges++
		code, ok := s.codes[r.PostForm.Get("code")]
		s.mu.Unlock()
		verifier := r.PostForm.Get("code_verifier")
		sum := sha256.Sum256([]byte(verifier))
		if !ok || len(verifier) != 43 || base64.RawURLEncoding.EncodeToString(sum[:]) != code.challenge {
			return nil, errors.New("fixture code or PKCE contract")
		}
		now := time.Now()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": "https://accounts.google.com", "sub": code.subject, "aud": syntheticGoogleClient,
			"nonce": code.nonce, "iat": now.Add(-time.Second).Unix(), "exp": now.Add(time.Minute).Unix(),
			"email": code.email, "email_verified": true,
		})
		token.Header["kid"] = "fixture"
		raw, err := token.SignedString(s.key)
		if err != nil {
			return nil, err
		}
		body = map[string]string{"id_token": raw, "access_token": "unused-synthetic-google-access-token"}
	default:
		return nil, errors.New("unexpected OAuth destination")
	}
	contents, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": {"public, max-age=600"}},
		Body: io.NopCloser(bytes.NewReader(contents))}, nil
}

func TestM04GooglePostgresJourney(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	admin, lifecycle, passwords, sessions, csrf, _ := newPasswordFixture(t, ctx)
	fixtureExec(t, ctx, admin, identity.GoogleSchema, pgx.QueryExecModeSimpleProtocol)
	fixtureExec(t, ctx, admin, `GRANT USAGE ON SCHEMA v3google TO m04_password_writer;
 GRANT SELECT,INSERT ON ALL TABLES IN SCHEMA v3google TO m04_password_writer;
 GRANT UPDATE(consumed_at) ON v3google.flows TO m04_password_writer;
 GRANT UPDATE(window_start,attempts) ON v3google.start_budget TO m04_password_writer;`, pgx.QueryExecModeSimpleProtocol)
	var unsafe bool
	if err := lifecycle.Pool.QueryRow(ctx, `SELECT has_schema_privilege(current_user,'v3google','CREATE') OR
 has_table_privilege(current_user,'v3google.links','UPDATE,DELETE,TRUNCATE')`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("Google runtime role has unexpected privileges")
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	var flowKey [32]byte
	if _, err := rand.Read(flowKey[:]); err != nil {
		t.Fatal("fixture flow key creation failed")
	}
	google, err := identity.NewGoogle(passwords, identity.GoogleConfig{
		Origin: origin, ClientID: syntheticGoogleClient, ClientSecret: syntheticGoogleSecret, FlowKey: flowKey[:],
	})
	if err != nil {
		t.Fatal("fixture Google configuration rejected")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture signing key creation failed")
	}
	provider := &googleFixtureTransport{key: key, redirectURI: origin + "/auth/google/callback", codes: make(map[string]googleCode)}
	identity.SetGoogleFixtureTransport(google, provider)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler, err := identity.GatewayWithGoogle(google, origin, http.NotFoundHandler(), logger)
	if err != nil {
		t.Fatal("fixture gateway configuration rejected")
	}
	server.Config.Handler = handler
	server.StartTLS()
	defer server.Close()

	probe := exec.CommandContext(ctx, os.Getenv("QA_M04_PYTHON"), "-B", os.Getenv("QA_M04_GOOGLE_PROBE"))
	payload, _ := json.Marshal(map[string]string{"origin": origin})
	probe.Stdin = bytes.NewReader(payload)
	if output, err := probe.Output(); err != nil || strings.TrimSpace(string(output)) != "PASS_M04_GOOGLE_PLAYWRIGHT_5" {
		t.Fatal("Playwright Google request smoke failed")
	}

	type outcome struct {
		status           int
		session, binding string
		body             struct {
			ID               string `json:"id"`
			UserID           string `json:"user_id"`
			CSRF             string `json:"csrf_token"`
			AuthorizationURL string `json:"authorization_url"`
		}
	}
	request := func(method, path, session, csrfValue, binding string, input map[string]string) outcome {
		var result outcome
		body, _ := json.Marshal(input)
		r, err := http.NewRequestWithContext(ctx, method, server.URL+"/v3/auth/"+path, bytes.NewReader(body))
		if err != nil {
			return result
		}
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrfValue)
		for name, value := range map[string]string{identity.SessionCookie: session, identity.GoogleBindingCookie: binding} {
			if value != "" {
				r.AddCookie(&http.Cookie{Name: name, Value: value})
			}
		}
		res, err := server.Client().Do(r)
		if err != nil {
			return result
		}
		defer res.Body.Close()
		result.status = res.StatusCode
		for _, c := range res.Cookies() {
			if c.HttpOnly && c.Secure && c.SameSite == http.SameSiteStrictMode && c.Path == "/" && c.Domain == "" {
				switch c.Name {
				case identity.SessionCookie:
					result.session = c.Value
				case identity.GoogleBindingCookie:
					result.binding = c.Value
				}
			}
		}
		if json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&result.body) != nil {
			return outcome{}
		}
		return result
	}
	post := func(expected int, path, session, csrfValue, binding string, input map[string]string) outcome {
		t.Helper()
		result := request(http.MethodPost, path, session, csrfValue, binding, input)
		if result.status != expected {
			t.Fatalf("%s returned %d, expected %d", path, result.status, expected)
		}
		return result
	}
	checkAccount := func(session, id string) {
		t.Helper()
		result := request(http.MethodGet, "me", session, "", "", nil)
		if result.status != 200 || result.body.ID != id || result.session != "" {
			t.Fatal("shared session account mismatch")
		}
	}
	invite := func(email string) (string, string) {
		t.Helper()
		id := post(202, "accounts/invite", sessions[0], csrf[0], "", map[string]string{"email": email}).body.UserID
		return id, fixtureDelivery(t, ctx, admin, passwords, id, "invite")
	}
	start := func(body map[string]string, session, csrfValue, subject, email string) (string, map[string]string) {
		t.Helper()
		result := post(200, "google/start", session, csrfValue, "", body)
		u, err := url.Parse(result.body.AuthorizationURL)
		if err != nil || u.Scheme != "https" || u.Host != "accounts.google.com" || len(result.binding) != 64 {
			t.Fatal("authorization destination or cookie contract rejected")
		}
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != provider.redirectURI ||
			q.Get("scope") != "openid email" || q.Get("response_type") != "code" || q.Get("client_id") != syntheticGoogleClient ||
			q.Get("client_secret") != "" || len(q.Get("nonce")) != 64 || len(q.Get("state")) != 64 {
			t.Fatal("authorization URL contract rejected")
		}
		code, _ := identity.RandomToken()
		provider.mu.Lock()
		provider.codes[code] = googleCode{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), email: email, subject: subject}
		provider.mu.Unlock()
		return result.binding, map[string]string{"state": q.Get("state"), "code": code}
	}

	login := map[string]string{"mode": "login"}
	googleOnlyID, googleOnlyInvite := invite(learnerEmail)
	binding, callback := start(login, "", "", "google-only-id", learnerEmail)
	post(401, "google/complete", "", "", binding, callback)
	accountInvite := map[string]string{"mode": "invite", "invite_token": googleOnlyInvite}
	binding, callback = start(accountInvite, "", "", "google-only-id", "wrong@example.invalid")
	post(401, "google/complete", "", "", binding, callback)
	binding, callback = start(accountInvite, "", "", "google-only-id", learnerEmail)
	before := provider.count()
	post(401, "google/complete", "", "", strings.Repeat("a", 64), callback)
	if provider.count() != before {
		t.Fatal("unbound callback reached provider")
	}
	googleOnly := post(200, "google/complete", "", "", binding, callback)
	checkAccount(googleOnly.session, googleOnlyID)
	post(401, "google/complete", "", "", binding, callback)
	post(401, "google/start", "", "", "", accountInvite)
	binding, callback = start(login, "", "", "google-only-id", "changed@example.invalid")
	checkAccount(post(200, "google/complete", "", "", binding, callback).session, googleOnlyID)

	const bothEmail = "both-methods@example.invalid"
	bothID, bothInvite := invite(bothEmail)
	post(200, "password/invite/accept", "", "", "", map[string]string{"token": bothInvite, "password": firstPassword})
	passwordLogin := map[string]string{"email": bothEmail, "password": firstPassword}
	local := post(200, "password/login", "", "", "", passwordLogin)
	checkAccount(local.session, bothID)
	link := map[string]string{"mode": "link", "email": bothEmail, "password": firstPassword}
	post(403, "google/start", local.session, "", "", link)
	post(401, "google/start", local.session, local.body.CSRF, "", map[string]string{
		"mode": "link", "email": bothEmail, "password": secondPassword,
	})
	binding, callback = start(link, local.session, local.body.CSRF, "google-both-id", bothEmail)
	before = provider.count()
	post(401, "google/complete", sessions[0], "", binding, callback)
	if provider.count() != before {
		t.Fatal("different local session reached provider")
	}
	linked := post(200, "google/complete", local.session, "", binding, callback)
	checkAccount(linked.session, bothID)
	if request(http.MethodGet, "me", local.session, "", "", nil).status != 401 {
		t.Fatal("link did not retire the source session")
	}
	checkAccount(post(200, "password/login", "", "", "", passwordLogin).session, bothID)
	post(200, "logout", linked.session, linked.body.CSRF, "", map[string]string{})
	if request(http.MethodGet, "me", linked.session, "", "", nil).status != 401 {
		t.Fatal("shared logout did not revoke Google session")
	}
	binding, callback = start(login, "", "", "google-both-id", bothEmail)
	beforeReset := post(200, "google/complete", "", "", binding, callback)
	post(202, "password/reset/request", "", "", "", map[string]string{"email": bothEmail})
	reset := fixtureDelivery(t, ctx, admin, passwords, bothID, "reset")
	post(200, "password/reset/confirm", "", "", "", map[string]string{"token": reset, "password": secondPassword})
	if request(http.MethodGet, "me", beforeReset.session, "", "", nil).status != 401 {
		t.Fatal("password reset did not revoke Google session")
	}
	post(401, "password/login", "", "", "", passwordLogin)
	passwordLogin["password"] = secondPassword
	checkAccount(post(200, "password/login", "", "", "", passwordLogin).session, bothID)

	thirdID, thirdInvite := invite("collision@example.invalid")
	third := map[string]string{"mode": "invite", "invite_token": thirdInvite}
	binding, callback = start(third, "", "", "google-both-id", "collision@example.invalid")
	post(409, "google/complete", "", "", binding, callback)
	binding, callback = start(third, "", "", "different-id", "collision@example.invalid")
	checkAccount(post(200, "google/complete", "", "", binding, callback).session, thirdID)

	binding, callback = start(login, "", "", "google-both-id", bothEmail)
	before = provider.count()
	fixtureExec(t, ctx, admin, "UPDATE v3google.flows SET expires_at=clock_timestamp()-interval '1 second' WHERE state_hash=$1", identity.TokenHash(callback["state"]))
	post(401, "google/complete", "", "", binding, callback)
	if provider.count() != before {
		t.Fatal("expired callback reached provider")
	}
	binding, callback = start(login, "", "", "google-both-id", bothEmail)
	results := make(chan outcome, 2)
	for range 2 {
		go func() { results <- request(http.MethodPost, "google/complete", "", "", binding, callback) }()
	}
	var winner outcome
	for range 2 {
		result := <-results
		if result.status == 200 && winner.session == "" {
			winner = result
		} else if result.status != 401 {
			t.Fatal("callback replay did not have exactly one winner")
		}
	}
	if provider.count() != before+1 {
		t.Fatal("callback replay reached provider twice")
	}
	checkAccount(winner.session, bothID)
	post(200, "accounts/suspend", sessions[0], csrf[0], "", map[string]string{"user_id": bothID})
	if request(http.MethodGet, "me", winner.session, "", "", nil).status != 401 {
		t.Fatal("suspension did not revoke Google session")
	}
	binding, callback = start(login, "", "", "google-both-id", bothEmail)
	post(401, "google/complete", "", "", binding, callback)
	post(401, "password/login", "", "", "", passwordLogin)
	var links, audit int
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3google.links),
		(SELECT count(*) FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'identity.google_%')`).Scan(&links, &audit); err != nil || links != 3 || audit != 6 {
		t.Fatal("Google link or audit/outbox reconciliation failed")
	}
	server.Close()
	for _, secret := range []string{syntheticGoogleSecret, learnerEmail, googleOnlyInvite, callback["state"], callback["code"], winner.session, firstPassword} {
		if bytes.Contains(logs.Bytes(), []byte(secret)) {
			t.Fatal("sensitive Google authentication data appeared in logs")
		}
	}
}
