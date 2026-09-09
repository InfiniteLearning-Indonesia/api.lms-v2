package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

const SessionCookie = "__Host-lms-v3"
const BindingCookie = "__Host-lms-v3-challenge"

type sessionKey struct{}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func ContextToken(ctx context.Context) string { v, _ := ctx.Value(sessionKey{}).(string); return v }

func cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func uniqueCookie(r *http.Request, name string) string {
	var value string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			value = c.Value
			count++
		}
	}
	if count != 1 {
		return ""
	}
	return value
}

// Gateway is cookie-only. CP10 bearer transport remains in its separate lab;
// it is never a fallback in this gateway. No X-User/X-Role/forwarded-origin trust.
func Gateway(service *Lifecycle, origin string, next http.Handler, logger *slog.Logger) http.Handler {
	var mu sync.Mutex
	window := time.Now()
	attempts := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := RandomToken()
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		w.Header().Set("X-Request-ID", requestID)
		status := 503
		route := "identity.unmatched"
		defer func() { logger.Info("identity_request", "request_id", requestID, "route", route, "status", status) }()
		reply := func(code int, v any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(v) }
		fail := func(err error) {
			code := 503
			message := "UNAVAILABLE"
			switch {
			case errors.Is(err, ErrUnauthenticated):
				code = 401
				message = "UNAUTHENTICATED"
			case errors.Is(err, ErrForbidden):
				code = 403
				message = "FORBIDDEN"
			case errors.Is(err, ErrConflict):
				code = 409
				message = "CONFLICT"
			}
			reply(code, map[string]string{"error": message})
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if r.TLS == nil || !strings.HasPrefix(origin, "https://") || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			fail(ErrUnauthenticated)
			return
		}
		mutation := r.Method != "GET" && r.Method != "HEAD"
		if mutation && (r.Header.Get("Origin") != origin || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
			fail(ErrForbidden)
			return
		}
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin {
			fail(ErrForbidden)
			return
		}
		decode := func(v any) bool {
			kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || kind != "application/json" {
				reply(400, map[string]string{"error": "INVALID_INPUT"})
				return false
			}
			r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			var extra any
			if d.Decode(v) != nil || d.Decode(&extra) != io.EOF {
				reply(400, map[string]string{"error": "INVALID_INPUT"})
				return false
			}
			return true
		}
		session := uniqueCookie(r, SessionCookie)
		csrf := r.Header.Get("X-CSRF-Token")
		switch {
		case r.URL.Path == "/v3/auth/challenge" && r.Method == "POST":
			route = "identity.challenge"
			var body struct{}
			if !decode(&body) {
				return
			}
			mu.Lock()
			if time.Since(window) > time.Minute {
				window = time.Now()
				attempts = 0
			}
			attempts++
			limited := attempts > 120
			mu.Unlock()
			if limited {
				reply(429, map[string]string{"error": "RATE_LIMITED"})
				return
			}
			nonce, binding, err := service.Challenge(ctx)
			if err != nil {
				fail(err)
				return
			}
			cookie(w, BindingCookie, binding, 60)
			reply(200, map[string]string{"nonce": nonce})
		case r.URL.Path == "/v3/auth/exchange" && r.Method == "POST":
			route = "identity.exchange"
			var body struct {
				Assertion string `json:"assertion"`
			}
			if !decode(&body) {
				return
			}
			c, err := service.Exchange(ctx, body.Assertion, uniqueCookie(r, BindingCookie))
			if err != nil {
				fail(err)
				return
			}
			cookie(w, BindingCookie, "", -1)
			cookie(w, SessionCookie, c.Token, 900)
			reply(200, map[string]string{"csrf_token": c.CSRF})
		case r.URL.Path == "/v3/auth/account-state" && r.Method == "POST":
			// A signed, purpose-specific authority message; a browser's self-profile
			// request cannot supply account status/roles directly. No outbound owner call.
			route = "identity.account_state"
			var body struct {
				Assertion string `json:"assertion"`
			}
			if !decode(&body) {
				return
			}
			if err := service.Project(ctx, body.Assertion); err != nil {
				fail(err)
				return
			}
			reply(200, map[string]string{"status": "projected"})
		case r.URL.Path == "/v3/auth/me" && r.Method == "GET":
			route = "identity.me"
			a, err := service.Validate(ctx, session, "", false)
			if err != nil {
				fail(err)
				return
			}
			reply(200, map[string]any{"id": a.ID, "site_admin": a.Admin})
		case r.Method == "POST" && (r.URL.Path == "/v3/auth/rotate" || r.URL.Path == "/v3/auth/logout" || r.URL.Path == "/v3/auth/revoke-all"):
			action := strings.TrimPrefix(r.URL.Path, "/v3/auth/")
			route = "identity." + action
			var body struct{}
			if !decode(&body) {
				return
			}
			c, err := service.SessionCommand(ctx, session, csrf, action)
			if err != nil {
				fail(err)
				return
			}
			if action == "rotate" {
				cookie(w, SessionCookie, c.Token, int(time.Until(c.Expires).Seconds()))
				reply(200, map[string]string{"csrf_token": c.CSRF})
			} else {
				cookie(w, SessionCookie, "", -1)
				reply(200, map[string]string{"status": "revoked"})
			}
		case r.URL.Path == "/v3/auth/site-role" && r.Method == "POST":
			route = "identity.site_role"
			var body struct {
				User    string `json:"user_id"`
				Admin   *bool  `json:"admin"`
				Version int64  `json:"version"`
			}
			if !decode(&body) {
				return
			}
			if body.Admin == nil {
				reply(400, map[string]string{"error": "INVALID_INPUT"})
				return
			}
			if err := service.SiteRole(ctx, session, csrf, body.User, *body.Admin, body.Version); err != nil {
				fail(err)
				return
			}
			reply(200, map[string]string{"status": "updated"})
		case strings.HasPrefix(r.URL.Path, "/v3/auth/"):
			// Invite/reset/profile and global identity administration belong to the
			// hardened legacy owner. There is no local duplicate password authority.
			reply(404, map[string]string{"error": "OWNER_ROUTE_NOT_AVAILABLE"})
		default:
			if _, err := service.Validate(ctx, session, csrf, mutation); err != nil {
				fail(err)
				return
			}
			route = "class.gateway"
			observed := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(observed, r.WithContext(context.WithValue(ctx, sessionKey{}, session)))
			status = observed.status
		}
	})
}
