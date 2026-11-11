package reporting

import (
	"context"
	"crypto/rand"
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
	p := strings.Split(strings.Trim(path, "/"), "/")
	return len(p) >= 4 && p[0] == "v3" && p[1] == "classes" && p[3] == "reports"
}

func Handler(store Store, logger *slog.Logger) http.Handler {
	_, keyErr := rand.Read(store.cursorKey[:])
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fail := func(code int, label string) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": label})
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		base := len(parts) >= 6 && parts[0] == "v3" && parts[1] == "classes" && parts[3] == "reports" && parts[4] == "grades"
		read := base && len(parts) == 6 && r.Method == http.MethodGet
		page := base && len(parts) == 7 && parts[6] == "pages" && r.Method == http.MethodPost
		export := base && len(parts) == 7 && parts[6] == "exports" && r.Method == http.MethodPost
		if !read && !page && !export {
			fail(404, "NOT_FOUND")
			return
		}
		if r.URL.RawQuery != "" {
			fail(400, "INVALID_INPUT")
			return
		}
		session := identity.ContextToken(r.Context())
		if session == "" {
			fail(401, "UNAUTHENTICATED")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if keyErr != nil {
			fail(503, "UNAVAILABLE")
			return
		}
		var value any
		var err error
		var download Export
		if page || export {
			kind, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || kind != "application/json" {
				fail(400, "INVALID_INPUT")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var pageRequest PageRequest
			var exportRequest ExportRequest
			var input any = &pageRequest
			if export {
				input = &exportRequest
			}
			var extra any
			if decoder.Decode(input) != nil || decoder.Decode(&extra) != io.EOF {
				fail(400, "INVALID_INPUT")
				return
			}
			if page {
				value, err = store.ReadPage(ctx, session, parts[2], parts[5], pageRequest)
			} else {
				download, err = store.ExportCSV(ctx, session, parts[2], parts[5], exportRequest)
			}
		} else {
			value, err = store.ReleasedGrades(ctx, session, parts[2], parts[5])
		}
		var encoded []byte
		if err == nil && !export {
			encoded, err = boundedJSON(value)
		}
		if err != nil {
			code, label := 503, "UNAVAILABLE"
			switch {
			case errors.Is(err, identity.ErrUnauthenticated):
				code, label = 401, "UNAUTHENTICATED"
			case errors.Is(err, ErrInvalid):
				code, label = 400, "INVALID_INPUT"
			case errors.Is(err, ErrForbidden):
				code, label = 403, "FORBIDDEN"
			case errors.Is(err, ErrNotFound):
				code, label = 404, "NOT_FOUND"
			case errors.Is(err, ErrStale):
				code, label = 409, "STALE_SNAPSHOT"
			case errors.Is(err, ErrTooLarge):
				code, label = 413, "REPORT_TOO_LARGE"
			}
			if code == 503 {
				logger.Error("report_read_failed", "error", err)
			}
			fail(code, label)
			return
		}
		if export {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="grades.csv"`)
			w.Header().Set("X-Export-Audit-ID", download.AuditID)
			w.Header().Set("X-Report-Snapshot", download.Snapshot)
			_, _ = w.Write(download.Data)
			return
		}
		_, _ = w.Write(encoded)
	})
}
