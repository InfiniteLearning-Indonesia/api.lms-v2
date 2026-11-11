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

func CredentialOwnsPath(path string) bool {
	p := strings.Split(strings.Trim(path, "/"), "/")
	if len(p) == 3 && p[0] == "v3" && p[1] == "legacy" && p[2] == "completion-observations" {
		return true
	}
	if len(p) == 4 && p[0] == "v3" && p[1] == "credentials" {
		return p[3] == "verify" || p[3] == "revoke" || p[3] == "supersede"
	}
	return len(p) >= 6 && p[0] == "v3" && p[1] == "classes" && p[3] == "learners" &&
		(p[5] == "transcripts" || p[5] == "credentials")
}

func CredentialHandler(store *Store, logger *slog.Logger) http.Handler {
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
		status, route := 500, "credential.unmatched"
		defer func() { logger.Info("credential_request", "request_id", requestID, "route", route, "status", status) }()
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
		if r.URL.RawQuery != "" || !CredentialOwnsPath(r.URL.Path) {
			fail(ErrNotFound)
			return
		}
		p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if len(p) == 4 && p[1] == "credentials" && p[3] == "verify" {
			route = "credential.verify"
			if r.Method != http.MethodGet {
				fail(ErrNotFound)
				return
			}
			result, e := store.Verify(ctx, p[2])
			if e != nil {
				fail(e)
				return
			}
			reply(http.StatusOK, result)
			return
		}
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		if r.Method == http.MethodGet {
			if len(p) != 7 || p[5] != "transcripts" || p[6] != "latest" {
				fail(ErrNotFound)
				return
			}
			route = "transcript.read"
			result, e := store.ReadTranscript(ctx, session, p[2], p[4])
			if e != nil {
				fail(e)
				return
			}
			reply(http.StatusOK, result)
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
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		decode := func(v any) bool {
			if decoder.Decode(v) != nil {
				return false
			}
			var extra any
			return decoder.Decode(&extra) == io.EOF
		}
		key := r.Header.Get("Idempotency-Key")
		replayed := false
		switch {
		case len(p) == 3 && p[1] == "legacy" && p[2] == "completion-observations":
			route = "completion.legacy.observe"
			var body LegacyOutputCommand
			if !decode(&body) {
				fail(ErrInvalid)
				return
			}
			result, replay, e := store.ObserveLegacyOutput(ctx, session, key, requestID, body)
			if e != nil {
				fail(e)
				return
			}
			if replay {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusCreated, result)
		case len(p) == 7 && p[5] == "transcripts" && p[6] == "releases":
			route = "transcript.release"
			var body struct{}
			if !decode(&body) {
				fail(ErrInvalid)
				return
			}
			result, replay, e := store.ReleaseTranscript(ctx, session, key, requestID, TranscriptCommand{ClassID: p[2], LearnerUserID: p[4]})
			if e != nil {
				fail(e)
				return
			}
			replayed = replay
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusCreated, result)
		case len(p) == 6 && p[5] == "credentials":
			route = "credential.issue"
			var body struct {
				TranscriptID string `json:"transcript_id"`
				Privacy      string `json:"privacy"`
			}
			if !decode(&body) {
				fail(ErrInvalid)
				return
			}
			result, replay, e := store.Issue(ctx, session, key, requestID, IssueCommand{ClassID: p[2], LearnerUserID: p[4], TranscriptID: body.TranscriptID, Privacy: body.Privacy})
			if e != nil {
				fail(e)
				return
			}
			replayed = replay
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusCreated, result)
		case len(p) == 4 && p[1] == "credentials" && p[3] == "revoke":
			route = "credential.revoke"
			var body struct {
				ExpectedVersion int64  `json:"expected_version"`
				Reason          string `json:"reason"`
			}
			if !decode(&body) {
				fail(ErrInvalid)
				return
			}
			result, replay, e := store.Revoke(ctx, session, key, requestID, TransitionCommand{CredentialID: p[2], ExpectedVersion: body.ExpectedVersion, Reason: body.Reason})
			if e != nil {
				fail(e)
				return
			}
			replayed = replay
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusOK, result)
		case len(p) == 4 && p[1] == "credentials" && p[3] == "supersede":
			route = "credential.supersede"
			var body struct {
				ExpectedVersion       int64  `json:"expected_version"`
				Reason                string `json:"reason"`
				SuccessorTranscriptID string `json:"successor_transcript_id"`
				Privacy               string `json:"privacy"`
			}
			if !decode(&body) {
				fail(ErrInvalid)
				return
			}
			result, replay, e := store.Supersede(ctx, session, key, requestID, TransitionCommand{CredentialID: p[2], ExpectedVersion: body.ExpectedVersion, Reason: body.Reason, SuccessorTranscriptID: body.SuccessorTranscriptID, Privacy: body.Privacy})
			if e != nil {
				fail(e)
				return
			}
			replayed = replay
			if replayed {
				w.Header().Set("Idempotency-Replayed", "true")
			}
			reply(http.StatusOK, result)
		default:
			fail(ErrNotFound)
		}
	})
}
