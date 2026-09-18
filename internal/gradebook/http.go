package gradebook

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
	return len(parts) >= 4 && parts[0] == "v3" && parts[1] == "classes" && parts[3] == "grade-items"
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
		status, route := 500, "gradebook.unmatched"
		defer func() { logger.Info("gradebook_request", "request_id", requestID, "route", route, "status", status) }()
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
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 || parts[0] != "v3" || parts[1] != "classes" || parts[3] != "grade-items" {
			fail(ErrNotFound)
			return
		}
		if len(parts) == 5 && r.Method == http.MethodGet {
			route = "gradebook.definition.read"
			value, e := store.ReadDefinition(ctx, session, parts[2], parts[4])
			if e != nil {
				fail(e)
				return
			}
			reply(200, value)
			return
		}
		if len(parts) == 7 && parts[5] == "grades" && r.Method == http.MethodGet {
			route = "gradebook.grade.read"
			value, e := store.ReadGrade(ctx, session, parts[2], parts[4], parts[6])
			if e != nil {
				fail(e)
				return
			}
			reply(200, value)
			return
		}
		kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil || kind != "application/json" {
			fail(ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		key := r.Header.Get("Idempotency-Key")
		definition := DefinitionCommand{ClassID: parts[2]}
		grade := GradeCommand{ClassID: parts[2]}
		switch {
		case len(parts) == 4 && r.Method == http.MethodPost:
			route, definition.Action = "gradebook.definition.create_item", "create_item"
			var body struct {
				ActivityID string           `json:"activity_id"`
				Title      string           `json:"title"`
				WeightBPS  int              `json:"weight_bps"`
				Scale      ScaleInput       `json:"scale"`
				Criteria   []CriterionInput `json:"criteria"`
			}
			err = decoder.Decode(&body)
			definition.ActivityID, definition.Title, definition.WeightBPS, definition.Scale, definition.Criteria = body.ActivityID, body.Title, body.WeightBPS, body.Scale, body.Criteria
		case len(parts) == 6 && parts[5] == "versions" && r.Method == http.MethodPost:
			route, definition.Action, definition.GradeItemID = "gradebook.definition.create_version", "create_version", parts[4]
			var body struct {
				WeightBPS int              `json:"weight_bps"`
				Scale     ScaleInput       `json:"scale"`
				Criteria  []CriterionInput `json:"criteria"`
			}
			err = decoder.Decode(&body)
			definition.WeightBPS, definition.Scale, definition.Criteria = body.WeightBPS, body.Scale, body.Criteria
		case len(parts) == 8 && parts[5] == "versions" && parts[7] == "publish" && r.Method == http.MethodPost:
			route, definition.Action, definition.GradeItemID, definition.DefinitionVersionID = "gradebook.definition.publish", "publish", parts[4], parts[6]
			var body struct {
				ExpectedPublicationRevision int64 `json:"expected_publication_revision"`
			}
			err = decoder.Decode(&body)
			definition.ExpectedPublicationRevision = body.ExpectedPublicationRevision
		case len(parts) == 6 && parts[5] == "grades" && r.Method == http.MethodPost:
			route, grade.GradeItemID = "gradebook.grade", parts[4]
			var body struct {
				AggregateID          string `json:"aggregate_id"`
				SubmissionRevisionID string `json:"submission_revision_id"`
				ExpectedVersion      int64  `json:"expected_version"`
				ValueMilli           int64  `json:"value_milli"`
				Reason               string `json:"reason"`
			}
			err = decoder.Decode(&body)
			grade.AggregateID, grade.SubmissionRevisionID, grade.ExpectedVersion, grade.ValueMilli, grade.Reason = body.AggregateID, body.SubmissionRevisionID, body.ExpectedVersion, body.ValueMilli, body.Reason
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
		if route == "gradebook.grade" {
			value, replayed, err = store.Grade(ctx, session, key, requestID, grade)
		} else {
			value, replayed, err = store.ExecuteDefinition(ctx, session, key, requestID, definition)
		}
		if err != nil {
			fail(err)
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
		code := 200
		if route == "gradebook.definition.create_item" || route == "gradebook.definition.create_version" {
			code = 201
		}
		reply(code, payload)
	})
}
