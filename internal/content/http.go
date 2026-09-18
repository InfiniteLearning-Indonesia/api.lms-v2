package content

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
	if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" {
		return false
	}
	return parts[3] == "content" || parts[3] == "sections" || parts[3] == "activities"
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := token(16)
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", requestID)
		status, route := http.StatusInternalServerError, "content.unmatched"
		defer func() { logger.Info("content_request", "request_id", requestID, "route", route, "status", status) }()
		reply := func(code int, value any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(value) }
		fail := func(err error) {
			code, message := http.StatusServiceUnavailable, "UNAVAILABLE"
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
		if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" {
			fail(ErrNotFound)
			return
		}
		if len(parts) == 4 && parts[3] == "content" && r.Method == http.MethodGet {
			route = "content.read"
			value, err := store.Read(ctx, session, parts[2])
			if err != nil {
				fail(err)
				return
			}
			reply(http.StatusOK, value)
			return
		}
		if len(parts) == 7 && parts[3] == "activities" && parts[5] == "revisions" && r.Method == http.MethodGet {
			route = "content.resolve_assignment_contract"
			value, err := store.ResolveAssignmentContract(ctx, session, parts[2], parts[4], parts[6])
			if err != nil {
				fail(err)
				return
			}
			reply(http.StatusOK, value)
			return
		}
		if kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type")); parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		command := Command{ClassID: parts[2]}
		switch {
		case len(parts) == 4 && parts[3] == "sections" && r.Method == http.MethodPost:
			route, command.Action = "content.create_section", "create_section"
			var body struct {
				Title    string `json:"title"`
				Position int    `json:"position"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Title, command.Position = body.Title, body.Position
			}
		case len(parts) == 5 && parts[3] == "sections" && r.Method == http.MethodPatch:
			route, command.Action, command.SectionID = "content.update_section", "update_section", parts[4]
			var body struct {
				Title    string `json:"title"`
				Position int    `json:"position"`
				Version  int64  `json:"version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Title, command.Position, command.Version = body.Title, body.Position, body.Version
			}
		case len(parts) == 4 && parts[3] == "activities" && r.Method == http.MethodPost:
			route, command.Action = "content.create_activity", "create_activity"
			var body struct {
				SectionID    string `json:"section_id"`
				ActivityType string `json:"activity_type"`
				Title        string `json:"title"`
				Position     int    `json:"position"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.SectionID, command.ActivityType, command.Title, command.Position = body.SectionID, body.ActivityType, body.Title, body.Position
			}
		case len(parts) == 5 && parts[3] == "activities" && r.Method == http.MethodPatch:
			route, command.Action, command.ActivityID = "content.update_activity", "update_activity", parts[4]
			var body struct {
				Title    string `json:"title"`
				Position int    `json:"position"`
				Version  int64  `json:"version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Title, command.Position, command.Version = body.Title, body.Position, body.Version
			}
		case len(parts) == 6 && parts[3] == "activities" && parts[5] == "revisions" && r.Method == http.MethodPost:
			route, command.Action, command.ActivityID = "content.create_revision", "create_revision", parts[4]
			var body struct {
				RawBody       string               `json:"raw_body"`
				Timezone      string               `json:"timezone"`
				AvailableFrom *time.Time           `json:"available_from"`
				DueAt         *time.Time           `json:"due_at"`
				CutoffAt      *time.Time           `json:"cutoff_at"`
				Assignment    *AssignmentRuleInput `json:"assignment"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.RawBody, command.Timezone = body.RawBody, body.Timezone
				command.AvailableFrom, command.DueAt, command.CutoffAt = body.AvailableFrom, body.DueAt, body.CutoffAt
				command.Assignment = body.Assignment
			}
		case len(parts) == 7 && parts[3] == "activities" && parts[5] == "revisions" && r.Method == http.MethodPatch:
			route, command.Action, command.ActivityID, command.RevisionID = "content.update_revision", "update_revision", parts[4], parts[6]
			var body struct {
				RawBody       string               `json:"raw_body"`
				Timezone      string               `json:"timezone"`
				AvailableFrom *time.Time           `json:"available_from"`
				DueAt         *time.Time           `json:"due_at"`
				CutoffAt      *time.Time           `json:"cutoff_at"`
				Version       int64                `json:"version"`
				Assignment    *AssignmentRuleInput `json:"assignment"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.RawBody, command.Timezone, command.Version = body.RawBody, body.Timezone, body.Version
				command.AvailableFrom, command.DueAt, command.CutoffAt = body.AvailableFrom, body.DueAt, body.CutoffAt
				command.Assignment = body.Assignment
			}
		case len(parts) == 8 && parts[3] == "activities" && parts[5] == "revisions" && (parts[7] == "publish" || parts[7] == "withdraw") && r.Method == http.MethodPost:
			route, command.Action, command.ActivityID, command.RevisionID = "content."+parts[7]+"_revision", parts[7]+"_revision", parts[4], parts[6]
			var body struct {
				Version int64 `json:"version"`
			}
			if err = decoder.Decode(&body); err == nil {
				command.Version = body.Version
			}
		default:
			fail(ErrNotFound)
			return
		}
		var extra any
		if err != nil || decoder.Decode(&extra) != io.EOF {
			fail(ErrInvalid)
			return
		}
		response, replayed, err := store.Execute(ctx, session, r.Header.Get("Idempotency-Key"), requestID, command)
		if err != nil {
			fail(err)
			return
		}
		code := http.StatusOK
		if strings.HasPrefix(command.Action, "create_") && !replayed {
			code = http.StatusCreated
		}
		reply(code, response)
	})
}
