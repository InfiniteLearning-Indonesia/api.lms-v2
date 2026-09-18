package logbook

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
	return len(parts) >= 4 && parts[0] == "v3" && parts[1] == "classes" && (parts[3] == "period-sets" || parts[3] == "logbook-entries")
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	periods := PeriodHandler(store, logger)
	entries := EntryHandler(store, logger)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 4 && parts[3] == "logbook-entries" {
			entries.ServeHTTP(w, r)
			return
		}
		periods.ServeHTTP(w, r)
	})
}

func PeriodHandler(store *Store, logger *slog.Logger) http.Handler {
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
		status, route := 500, "logbook.period.unmatched"
		defer func() {
			logger.Info("logbook_period_request", "request_id", requestID, "route", route, "status", status)
		}()
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
		if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" || parts[3] != "period-sets" {
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
		if r.Method == http.MethodGet && (len(parts) == 4 || len(parts) == 5) {
			route = "logbook.period.read"
			setID := ""
			if len(parts) == 5 {
				setID = parts[4]
			}
			value, e := store.Read(ctx, session, parts[2], setID)
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
		r.Body = http.MaxBytesReader(w, r.Body, 16000)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		command := Command{ClassID: parts[2]}
		switch {
		case len(parts) == 4:
			route, command.Action = "logbook.period.create", "create"
			var body struct {
				Title    string   `json:"title"`
				Timezone string   `json:"timezone"`
				Periods  []Period `json:"periods"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Title, command.Timezone, command.Periods = body.Title, body.Timezone, body.Periods
			}
		case len(parts) == 6 && parts[5] == "publish":
			route, command.Action, command.SetID = "logbook.period.publish", "publish", parts[4]
			var body struct {
				ExpectedActiveID string `json:"expected_active_id"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.ExpectedActiveID = body.ExpectedActiveID
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

func EntryHandler(store *Store, logger *slog.Logger) http.Handler {
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
		status, route := 500, "logbook.entry.unmatched"
		defer func() {
			logger.Info("logbook_entry_request", "request_id", requestID, "route", route, "status", status)
		}()
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
		if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" || parts[3] != "logbook-entries" {
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
		if r.Method == http.MethodGet && len(parts) == 5 {
			route = "logbook.entry.read"
			value, e := store.ReadEntry(ctx, session, parts[2], parts[4])
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
		r.Body = http.MaxBytesReader(w, r.Body, 70000)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		entry := EntryCommand{ClassID: parts[2]}
		var review ReviewCommand
		isReview := false
		switch {
		case len(parts) == 4:
			route, entry.Action = "logbook.entry.save", "save"
			var body struct {
				PeriodID     string   `json:"period_id"`
				Body         string   `json:"body"`
				FileAssetIDs []string `json:"file_asset_ids"`
			}
			if err = decoder.Decode(&body); err == nil {
				entry.PeriodID, entry.Body, entry.FileAssetIDs = body.PeriodID, body.Body, body.FileAssetIDs
			}
		case len(parts) == 6 && parts[5] == "submit":
			route, entry.Action, entry.EntryID = "logbook.entry.submit", "submit", parts[4]
			var body struct {
				RevisionID      string `json:"revision_id"`
				ExpectedVersion int64  `json:"expected_version"`
			}
			if err = decoder.Decode(&body); err == nil {
				entry.RevisionID, entry.ExpectedVersion = body.RevisionID, body.ExpectedVersion
			}
		case len(parts) == 6 && parts[5] == "review":
			route, isReview = "logbook.entry.review", true
			review.ClassID, review.EntryID = parts[2], parts[4]
			var body struct {
				RevisionID      string `json:"revision_id"`
				ExpectedVersion int64  `json:"expected_version"`
				Decision        string `json:"decision"`
				Feedback        string `json:"feedback"`
			}
			if err = decoder.Decode(&body); err == nil {
				review.RevisionID, review.ExpectedVersion, review.Decision, review.Feedback = body.RevisionID, body.ExpectedVersion, body.Decision, body.Feedback
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
		var value json.RawMessage
		var replayed bool
		var executeErr error
		if isReview {
			value, replayed, executeErr = store.Review(ctx, session, r.Header.Get("Idempotency-Key"), requestID, review)
		} else {
			value, replayed, executeErr = store.ExecuteEntry(ctx, session, r.Header.Get("Idempotency-Key"), requestID, entry)
		}
		if executeErr != nil {
			fail(executeErr)
			return
		}
		if replayed {
			w.Header().Set("Idempotent-Replay", "true")
		}
		code := 200
		if entry.Action == "save" && !replayed {
			code = 201
		}
		status = code
		w.WriteHeader(code)
		_, _ = w.Write(value)
	})
}
