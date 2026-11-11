package completion

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
	return parts[3] == "progress" ||
		(parts[3] == "learners" && len(parts) == 6 && parts[5] == "progress") ||
		(len(parts) >= 6 && parts[3] == "activities" && (parts[5] == "completion-rules" || parts[5] == "completion"))
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
		status, route := 500, "completion.unmatched"
		defer func() { logger.Info("completion_request", "request_id", requestID, "route", route, "status", status) }()
		reply := func(code int, value any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(value) }
		fail := func(e error) {
			code, message := http.StatusServiceUnavailable, "UNAVAILABLE"
			switch {
			case errors.Is(e, identity.ErrUnauthenticated):
				code, message = http.StatusUnauthorized, "UNAUTHENTICATED"
			case errors.Is(e, ErrInvalid):
				code, message = http.StatusBadRequest, "INVALID_INPUT"
			case errors.Is(e, ErrForbidden):
				code, message = http.StatusForbidden, "FORBIDDEN"
			case errors.Is(e, ErrNotFound):
				code, message = http.StatusNotFound, "NOT_FOUND"
			case errors.Is(e, ErrConflict):
				code, message = http.StatusConflict, "CONFLICT"
			}
			reply(code, map[string]string{"error": message, "request_id": requestID})
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.URL.RawQuery != "" || !OwnsPath(r.URL.Path) {
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
		if r.Method == http.MethodGet {
			target := ""
			switch {
			case len(parts) == 4 && parts[3] == "progress":
				route = "completion.progress.self"
			case len(parts) == 6 && parts[3] == "learners" && parts[5] == "progress":
				route, target = "completion.progress.learner", parts[4]
			default:
				fail(ErrNotFound)
				return
			}
			value, readErr := store.Progress(ctx, session, parts[2], target)
			if readErr != nil {
				fail(readErr)
				return
			}
			reply(http.StatusOK, value)
			return
		}
		if r.Method != http.MethodPost || len(parts) < 6 || parts[3] != "activities" {
			fail(ErrNotFound)
			return
		}
		kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		key := r.Header.Get("Idempotency-Key")
		switch {
		case len(parts) == 6 && parts[5] == "completion-rules":
			route = "completion.rule.create"
			var body struct {
				ContentRevisionID string `json:"content_revision_id"`
				ExpectedVersion   int64  `json:"expected_version"`
				Requirement       string `json:"requirement"`
				EvidenceKind      string `json:"evidence_kind"`
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
			value, replayed, executeErr := store.CreateRule(ctx, session, key, requestID, RuleCommand{ClassID: parts[2], ActivityID: parts[4], ContentRevisionID: body.ContentRevisionID, ExpectedVersion: body.ExpectedVersion, Requirement: body.Requirement, EvidenceKind: body.EvidenceKind})
			if executeErr != nil {
				fail(executeErr)
				return
			}
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusCreated, value)
		case len(parts) == 7 && parts[5] == "completion" && parts[6] == "evaluate":
			route = "completion.evaluate"
			var body struct {
				ContentRevisionID string `json:"content_revision_id"`
				RuleVersionID     string `json:"rule_version_id"`
				Acknowledge       bool   `json:"acknowledge"`
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
			value, replayed, executeErr := store.Evaluate(ctx, session, key, requestID, EvaluationCommand{ClassID: parts[2], ActivityID: parts[4], ContentRevisionID: body.ContentRevisionID, RuleVersionID: body.RuleVersionID, Acknowledge: body.Acknowledge})
			if executeErr != nil {
				fail(executeErr)
				return
			}
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusOK, value)
		case len(parts) == 7 && parts[5] == "completion" && parts[6] == "override":
			route = "completion.correct"
			var body struct {
				RuleVersionID   string `json:"rule_version_id"`
				LearnerUserID   string `json:"learner_user_id"`
				Action          string `json:"action"`
				Reason          string `json:"reason"`
				ExpectedVersion int64  `json:"expected_version"`
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
			value, replayed, executeErr := store.Correct(ctx, session, key, requestID, CorrectionCommand{
				ClassID: parts[2], ActivityID: parts[4], RuleVersionID: body.RuleVersionID,
				LearnerUserID: body.LearnerUserID, Action: body.Action, Reason: body.Reason, ExpectedVersion: body.ExpectedVersion,
			})
			if executeErr != nil {
				fail(executeErr)
				return
			}
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusOK, value)
		default:
			fail(ErrNotFound)
		}
	})
}
