package attendance

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
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) >= 4 && parts[0] == "v3" && parts[1] == "classes" && parts[3] == "meetings"
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := token()
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", requestID)
		status, route := 500, "meeting.unmatched"
		defer func() { logger.Info("meeting_request", "request_id", requestID, "route", route, "status", status) }()
		reply := func(code int, value any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(value) }
		fail := func(e error) {
			code, message := 503, "UNAVAILABLE"
			switch {
			case errors.Is(e, identity.ErrUnauthenticated):
				code, message = 401, "UNAUTHENTICATED"
			case errors.Is(e, ErrInvalid):
				code, message = 400, "INVALID_INPUT"
			case errors.Is(e, ErrForbidden):
				code, message = 403, "FORBIDDEN"
			case errors.Is(e, ErrNotFound):
				code, message = 404, "NOT_FOUND"
			case errors.Is(e, ErrConflict):
				code, message = 409, "CONFLICT"
			}
			reply(code, map[string]string{"error": message, "request_id": requestID})
		}
		if r.URL.RawQuery != "" {
			fail(ErrInvalid)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" || parts[3] != "meetings" {
			fail(ErrNotFound)
			return
		}
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if len(parts) == 4 && r.Method == http.MethodGet {
			route = "meeting.read"
			value, e := store.Read(ctx, session, parts[2])
			if e != nil {
				fail(e)
				return
			}
			reply(200, value)
			return
		}
		if r.Method != http.MethodPost {
			fail(ErrNotFound)
			return
		}
		kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		command := Command{ClassID: parts[2]}
		switch {
		case len(parts) == 4:
			route, command.Action = "meeting.create", "create"
			var body struct {
				Title     string    `json:"title"`
				LocalDate string    `json:"local_date"`
				Timezone  string    `json:"timezone"`
				StartsAt  time.Time `json:"starts_at"`
				EndsAt    time.Time `json:"ends_at"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Title, command.LocalDate, command.Timezone, command.StartsAt, command.EndsAt = body.Title, body.LocalDate, body.Timezone, body.StartsAt, body.EndsAt
			}
		case len(parts) == 6 && (parts[5] == "cancel" || parts[5] == "reinstate"):
			route, command.Action, command.MeetingID = "meeting."+parts[5], parts[5], parts[4]
			var body struct {
				Version int64  `json:"version"`
				Reason  string `json:"reason"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Version, command.Reason = body.Version, body.Reason
			}
		default:
			fail(ErrNotFound)
			return
		}
		if err != nil {
			fail(ErrInvalid)
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			fail(ErrInvalid)
			return
		}
		value, replayed, e := store.Execute(ctx, session, r.Header.Get("Idempotency-Key"), requestID, command)
		if e != nil {
			fail(e)
			return
		}
		if replayed {
			w.Header().Set("Idempotent-Replay", "true")
		}
		code := 200
		if command.Action == "create" && !replayed {
			code = 201
		}
		status = code
		w.WriteHeader(code)
		_, _ = w.Write(value)
	})
}
