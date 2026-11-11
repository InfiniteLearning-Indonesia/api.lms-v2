package aisuggestions

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
	parts := strings.Split(path, "/")
	return (len(parts) == 5 || len(parts) == 6) && parts[1] == "v3" && parts[2] == "classes" && parts[4] == "ai-suggestions"
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		requestID, idErr := newID()
		if idErr != nil {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("X-Request-ID", requestID)
		status := 500
		defer func() { logger.Info("ai_suggestion_request", "status", status) }()
		reply := func(code int, value any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(value) }
		fail := func(err error) {
			code, message := 503, "UNAVAILABLE"
			switch {
			case errors.Is(err, identity.ErrUnauthenticated):
				code, message = 401, "UNAUTHENTICATED"
			case errors.Is(err, ErrInvalid):
				code, message = 400, "INVALID_INPUT"
			case errors.Is(err, ErrForbidden):
				code, message = 403, "FORBIDDEN"
			case errors.Is(err, ErrNotFound):
				code, message = 404, "NOT_FOUND"
			case errors.Is(err, ErrConflict):
				code, message = 409, "CONFLICT"
			}
			reply(code, map[string]string{"error": message})
		}
		if r.URL.RawQuery != "" || !OwnsPath(r.URL.Path) {
			fail(ErrInvalid)
			return
		}
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		switch {
		case len(parts) == 6 && r.Method == http.MethodGet:
			v, err := store.Read(ctx, session, parts[3], parts[5])
			if err != nil {
				fail(err)
				return
			}
			reply(200, v)
		case len(parts) == 5 && r.Method == http.MethodPost:
			kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || kind != "application/json" {
				fail(ErrInvalid)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var body struct {
				GradeItemID          string `json:"grade_item_id"`
				AggregateID          string `json:"aggregate_id"`
				SubmissionRevisionID string `json:"submission_revision_id"`
			}
			if decoder.Decode(&body) != nil {
				fail(ErrInvalid)
				return
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				fail(ErrInvalid)
				return
			}
			v, replayed, err := store.Create(ctx, session, r.Header.Get("Idempotency-Key"), requestID, Command{
				ClassID: parts[3], GradeItemID: body.GradeItemID, AggregateID: body.AggregateID, SubmissionRevisionID: body.SubmissionRevisionID,
			})
			if err != nil {
				fail(err)
				return
			}
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(200, v)
		default:
			reply(404, map[string]string{"error": "NOT_FOUND"})
		}
	})
}
