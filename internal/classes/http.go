package classes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"lmsv3/internal/identity"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"
)

// Handler has no token-issuance/login bypass and never trusts X-User/X-Role.
// Bearer sessions only for this slice; cookie auth/CSRF/browser integration is
// deliberately not enabled. Do not deploy before the M04 issuer contract closes.
func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := ID()
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", requestID)
		status := 500
		route := "unmatched"
		defer func() { logger.Info("class_request", "request_id", requestID, "route", route, "status", status) }()
		respond := func(code int, body any) { status = code; w.WriteHeader(code); _ = json.NewEncoder(w).Encode(body) }
		fail := func(e error) {
			code := 503
			message := "UNAVAILABLE"
			switch {
			case errors.Is(e, identity.ErrUnauthenticated):
				code = 401
				message = "UNAUTHENTICATED"
			case errors.Is(e, ErrInvalid):
				code = 400
				message = "INVALID_INPUT"
			case errors.Is(e, ErrForbidden):
				code = 403
				message = "FORBIDDEN"
			case errors.Is(e, ErrNotFound):
				code = 404
				message = "NOT_FOUND"
			case errors.Is(e, ErrConflict):
				code = 409
				message = "CONFLICT"
			}
			respond(code, map[string]string{"error": message, "request_id": requestID})
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if r.URL.Path == "/health/live" && r.Method == "GET" {
			route = "health"
			respond(200, map[string]string{"status": "alive"})
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") || !opaqueToken(token) {
			fail(identity.ErrUnauthenticated)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 2 || parts[0] != "v3" || parts[1] != "classes" {
			fail(ErrNotFound)
			return
		}
		if r.URL.RawQuery != "" {
			fail(ErrInvalid)
			return
		}
		if len(parts) == 3 && r.Method == "GET" {
			route = "class.read"
			value, err := store.Read(ctx, token, parts[2])
			if err != nil {
				fail(err)
				return
			}
			respond(200, value)
			return
		}
		// The lab API is not a cross-origin browser service. No permissive CORS.
		if r.Header.Get("Origin") != "" {
			fail(ErrForbidden)
			return
		}
		kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		decode := func(v any) error {
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if err := d.Decode(v); err != nil {
				return ErrInvalid
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				return ErrInvalid
			}
			return nil
		}
		cmd := Command{}
		if len(parts) == 2 && r.Method == "POST" {
			cmd.Action = "create"
		} else if len(parts) == 3 && r.Method == "PATCH" {
			cmd.Action = "edit"
			cmd.ClassID = parts[2]
		} else if len(parts) == 4 && r.Method == "POST" {
			cmd.ClassID = parts[2]
			switch parts[3] {
			case "participants":
				cmd.Action = "add"
			case "publish", "archive":
				cmd.Action = parts[3]
			default:
				fail(ErrNotFound)
				return
			}
		} else if len(parts) == 6 && parts[3] == "participants" && parts[5] == "end" && r.Method == "POST" {
			cmd.Action = "end"
			cmd.ClassID = parts[2]
			cmd.UserID = parts[4]
		} else {
			fail(ErrNotFound)
			return
		}
		switch cmd.Action {
		case "create":
			var v struct {
				Name    string `json:"name"`
				Program string `json:"program_label"`
				Cohort  string `json:"cohort_label"`
			}
			if err = decode(&v); err == nil {
				cmd.Name = v.Name
				cmd.ProgramLabel = v.Program
				cmd.CohortLabel = v.Cohort
			}
		case "edit":
			var v struct {
				Name    string  `json:"name"`
				Program *string `json:"program_label"`
				Cohort  *string `json:"cohort_label"`
				Version int64   `json:"version"`
			}
			if err = decode(&v); err == nil {
				cmd.Name = v.Name
				if v.Program != nil {
					cmd.ProgramLabel = *v.Program
					cmd.UpdateProgram = true
				}
				if v.Cohort != nil {
					cmd.CohortLabel = *v.Cohort
					cmd.UpdateCohort = true
				}
				cmd.Version = v.Version
			}
		case "add":
			var v struct {
				User string `json:"user_id"`
				Role string `json:"role"`
			}
			if err = decode(&v); err == nil {
				cmd.UserID = v.User
				cmd.Role = v.Role
			}
		case "publish", "archive":
			var v struct {
				Version int64 `json:"version"`
			}
			if err = decode(&v); err == nil {
				cmd.Version = v.Version
			}
		case "end":
			var v struct{}
			err = decode(&v)
		}
		if err != nil {
			fail(err)
			return
		}
		route = "class." + cmd.Action
		response, err := store.Execute(ctx, token, r.Header.Get("Idempotency-Key"), requestID, cmd)
		if err != nil {
			fail(err)
			return
		}
		respond(200, response)
	})
}
func opaqueToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
