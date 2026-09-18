package submissions

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
	return len(parts) >= 4 && parts[0] == "v3" && parts[1] == "classes" && (parts[3] == "submissions" || parts[3] == "activities") && strings.Contains(path, "submission")
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := token(16)
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", requestID)
		status, route := 500, "submission.unmatched"
		defer func() { logger.Info("submission_request", "request_id", requestID, "route", route, "status", status) }()
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
			case errors.Is(err, ErrDeadline):
				code, message = 409, "DEADLINE_PASSED"
			}
			reply(code, map[string]string{"error": message, "request_id": requestID})
		}
		if r.URL.RawQuery != "" {
			fail(ErrInvalid)
			return
		}
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 5 || parts[0] != "v3" || parts[1] != "classes" {
			fail(ErrNotFound)
			return
		}
		if len(parts) == 5 && parts[3] == "submissions" && r.Method == http.MethodGet {
			route = "submission.read"
			value, readErr := store.Read(ctx, session, parts[2], parts[4])
			if readErr != nil {
				fail(readErr)
				return
			}
			reply(200, value)
			return
		}
		if kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type")); parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		command := Command{ClassID: parts[2]}
		switch {
		case len(parts) == 8 && parts[3] == "activities" && parts[5] == "revisions" && parts[7] == "submissions" && r.Method == http.MethodPost:
			route, command.Action, command.ActivityID, command.AssignmentRevisionID = "submission.create", "create", parts[4], parts[6]
			var body struct{}
			err = decoder.Decode(&body)
		case len(parts) == 6 && parts[3] == "submissions" && parts[5] == "revisions" && r.Method == http.MethodPost:
			route, command.Action, command.AggregateID = "submission.append_revision", "append_revision", parts[4]
			var body struct {
				Version      int64    `json:"version"`
				Body         string   `json:"body"`
				FileAssetIDs []string `json:"file_asset_ids"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Version, command.Body, command.FileAssetIDs = body.Version, body.Body, body.FileAssetIDs
			}
		case len(parts) == 6 && parts[3] == "submissions" && (parts[5] == "lock" || parts[5] == "return") && r.Method == http.MethodPost:
			route, command.Action, command.AggregateID = "submission."+parts[5], parts[5], parts[4]
			var body struct {
				AttemptVersion int64 `json:"attempt_version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.AttemptVersion = body.AttemptVersion
			}
		case len(parts) == 6 && parts[3] == "submissions" && parts[5] == "resubmit" && r.Method == http.MethodPost:
			route, command.Action, command.AggregateID = "submission.resubmit", "resubmit", parts[4]
			var body struct {
				Version        int64 `json:"version"`
				AttemptVersion int64 `json:"attempt_version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Version, command.AttemptVersion = body.Version, body.AttemptVersion
			}
		case len(parts) == 6 && parts[3] == "submissions" && parts[5] == "handin" && r.Method == http.MethodPost:
			route, command.Action, command.AggregateID = "submission.handin", "handin", parts[4]
			var body struct {
				Version        int64 `json:"version"`
				AttemptVersion int64 `json:"attempt_version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Version, command.AttemptVersion = body.Version, body.AttemptVersion
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
		value, replayed, execErr := store.Execute(ctx, session, key, requestID, command)
		if execErr != nil {
			fail(execErr)
			return
		}
		if replayed {
			w.Header().Set("Idempotency-Replayed", "true")
		}
		var payload any
		if json.Unmarshal(value, &payload) != nil {
			fail(errors.New("encode"))
			return
		}
		reply(200, payload)
	})
}
