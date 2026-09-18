package files

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lmsv3/internal/identity"
)

func OwnsPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) >= 2 && parts[0] == "v3" && (parts[1] == "files" || parts[1] == "file-downloads" || (parts[1] == "classes" && len(parts) == 4 && (parts[3] == "files" || parts[3] == "file-imports")))
}

func Handler(store *Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, randomErr := randomHex(16)
		if randomErr != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", requestID)
		status, route := http.StatusInternalServerError, "file.unmatched"
		defer func() { logger.Info("file_request", "request_id", requestID, "route", route, "status", status) }()
		jsonReply := func(code int, value any) {
			status = code
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(value)
		}
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
			jsonReply(code, map[string]string{"error": message, "request_id": requestID})
		}
		if r.URL.RawQuery != "" {
			fail(ErrInvalid)
			return
		}
		token := identity.ContextToken(r.Context())
		if token == "" {
			fail(identity.ErrUnauthenticated)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case len(parts) == 4 && parts[0] == "v3" && parts[1] == "classes" && parts[3] == "file-imports" && r.Method == http.MethodPost:
			route = "file.remote_import"
			if kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || kind != "application/json" {
				fail(ErrInvalid)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var body struct {
				URL          string `json:"url"`
				OwnerUserID  string `json:"owner_user_id"`
				Purpose      string `json:"purpose"`
				OriginalName string `json:"original_name"`
				DeclaredType string `json:"declared_media_type"`
			}
			var extra any
			if decoder.Decode(&body) != nil || decoder.Decode(&extra) != io.EOF {
				fail(ErrInvalid)
				return
			}
			asset, replayed, err := store.ImportRemote(ctx, token, r.Header.Get("Idempotency-Key"), requestID, body.URL, Upload{ClassID: parts[2], OwnerUserID: body.OwnerUserID, Purpose: body.Purpose, OriginalName: body.OriginalName, DeclaredType: body.DeclaredType})
			if err != nil {
				fail(err)
				return
			}
			code := http.StatusCreated
			if replayed {
				code = http.StatusOK
			} else if asset.State != "AVAILABLE" {
				code = http.StatusAccepted
			}
			jsonReply(code, asset)
		case len(parts) == 4 && parts[0] == "v3" && parts[1] == "classes" && parts[3] == "files" && r.Method == http.MethodPost:
			route = "file.upload"
			kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				fail(ErrInvalid)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, MaxBytes+1)
			data, err := io.ReadAll(r.Body)
			if err != nil || len(data) > MaxBytes {
				fail(ErrInvalid)
				return
			}
			asset, replayed, err := store.Upload(ctx, token, r.Header.Get("Idempotency-Key"), requestID, Upload{ClassID: parts[2], OwnerUserID: r.Header.Get("X-File-Owner"), Purpose: r.Header.Get("X-File-Purpose"), OriginalName: r.Header.Get("X-File-Name"), DeclaredType: kind, Data: data})
			if err != nil {
				fail(err)
				return
			}
			code := http.StatusCreated
			if replayed {
				code = http.StatusOK
			} else if asset.State != "AVAILABLE" {
				code = http.StatusAccepted
			}
			jsonReply(code, asset)
		case len(parts) == 3 && parts[0] == "v3" && parts[1] == "files" && r.Method == http.MethodGet:
			route = "file.metadata"
			asset, err := store.Metadata(ctx, token, parts[2])
			if err != nil {
				fail(err)
				return
			}
			jsonReply(http.StatusOK, asset)
		case len(parts) == 4 && parts[0] == "v3" && parts[1] == "files" && parts[3] == "scan" && r.Method == http.MethodPost:
			route = "file.scan"
			asset, err := store.Rescan(ctx, token, parts[2], requestID)
			if err != nil {
				fail(err)
				return
			}
			jsonReply(http.StatusOK, asset)
		case len(parts) == 4 && parts[0] == "v3" && parts[1] == "files" && parts[3] == "download-grants" && r.Method == http.MethodPost:
			route = "file.grant"
			if kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || kind != "application/json" {
				fail(ErrInvalid)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var body struct{}
			var extra any
			if decoder.Decode(&body) != nil || decoder.Decode(&extra) != io.EOF {
				fail(ErrInvalid)
				return
			}
			grant, err := store.Grant(ctx, token, parts[2], requestID)
			if err != nil {
				fail(err)
				return
			}
			jsonReply(http.StatusCreated, grant)
		case len(parts) == 3 && parts[0] == "v3" && parts[1] == "file-downloads" && r.Method == http.MethodGet:
			route = "file.download"
			asset, data, err := store.Download(ctx, token, parts[2], requestID)
			if err != nil {
				fail(err)
				return
			}
			status = http.StatusOK
			w.Header().Set("Content-Type", asset.DetectedMediaType)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": asset.OriginalName}))
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.WriteHeader(status)
			_, _ = w.Write(data)
		default:
			fail(ErrNotFound)
		}
	})
}
