package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"lmsv3/internal/identity"
)

func OwnsPath(path string) bool {
	return path == "/v3/notification-preferences/email" || path == "/v3/operations/deliveries" || path == "/v3/operations/notifications"
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		status := 500
		defer func() { logger.Info("notification_request", "path", r.URL.Path, "status", status) }()
		reply := func(code int, v any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(v) }
		fail := func(err error) {
			code, msg := 503, "UNAVAILABLE"
			switch {
			case errors.Is(err, identity.ErrUnauthenticated):
				code, msg = 401, "UNAUTHENTICATED"
			case errors.Is(err, ErrInvalid):
				code, msg = 400, "INVALID_INPUT"
			case errors.Is(err, ErrForbidden):
				code, msg = 403, "FORBIDDEN"
			case errors.Is(err, ErrConflict):
				code, msg = 409, "CONFLICT"
			}
			reply(code, map[string]string{"error": msg})
		}
		if r.URL.RawQuery != "" || !OwnsPath(r.URL.Path) {
			fail(ErrInvalid)
			return
		}
		token := identity.ContextToken(r.Context())
		if token == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch {
		case r.URL.Path == "/v3/operations/deliveries" && r.Method == http.MethodGet:
			v, err := store.Deliveries(ctx, token)
			if err != nil {
				fail(err)
				return
			}
			reply(200, map[string]any{"deliveries": v})
		case r.URL.Path == "/v3/operations/notifications" && r.Method == http.MethodGet:
			v, err := store.Notifications(ctx, token)
			if err != nil {
				fail(err)
				return
			}
			reply(200, map[string]any{"notifications": v})
		case r.URL.Path == "/v3/notification-preferences/email" && r.Method == http.MethodGet:
			v, err := store.Preference(ctx, token)
			if err != nil {
				fail(err)
				return
			}
			reply(200, v)
		case r.URL.Path == "/v3/notification-preferences/email" && r.Method == http.MethodPut:
			kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || kind != "application/json" {
				fail(ErrInvalid)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			var body struct {
				Enabled         *bool `json:"enabled"`
				ExpectedVersion int64 `json:"expected_version"`
			}
			if d.Decode(&body) != nil || body.Enabled == nil {
				fail(ErrInvalid)
				return
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				fail(ErrInvalid)
				return
			}
			v, replayed, err := store.SetPreference(ctx, token, r.Header.Get("Idempotency-Key"), body.ExpectedVersion, *body.Enabled)
			if err != nil {
				fail(err)
				return
			}
			if replayed {
				w.Header().Set("Idempotent-Replay", "true")
			}
			status = 200
			w.WriteHeader(200)
			_, _ = w.Write(v)
		default:
			if strings.HasPrefix(r.URL.Path, "/v3/") {
				reply(404, map[string]string{"error": "NOT_FOUND"})
				return
			}
			fail(ErrInvalid)
		}
	})
}
