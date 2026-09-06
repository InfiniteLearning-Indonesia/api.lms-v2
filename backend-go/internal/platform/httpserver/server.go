package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"lmsv3/internal/platform/config"
)

// Handler exposes only liveness and NOT_READY. No synthetic persistence or
// successful readiness response pretends the database/domain has been built.
func Handler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		requestID := hex.EncodeToString(token[:])
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		status := http.StatusNotFound
		body := map[string]string{"error": "NOT_FOUND", "request_id": requestID}
		route := "unmatched"
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
			route = r.URL.Path
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				status = http.StatusMethodNotAllowed
				w.Header().Set("Allow", "GET, HEAD")
				body = map[string]string{"error": "METHOD_NOT_ALLOWED", "request_id": requestID}
			} else if route == "/health/live" {
				status = http.StatusOK
				body = map[string]string{"status": "alive"}
			} else {
				status = http.StatusServiceUnavailable
				body = map[string]string{"status": "not_ready"}
			}
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_ = json.NewEncoder(w).Encode(body)
		}
		// Deliberately exclude raw URL/query, headers, remote address and body.
		logger.Info("http_request", "request_id", requestID, "route", route, "status", status)
	})
}

func Serve(ctx context.Context, c config.Config, listener net.Listener, logger *slog.Logger) error {
	server := &http.Server{Handler: Handler(logger), ReadHeaderTimeout: c.ReadHeaderTimeout, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			_ = server.Close()
		}
		<-done
		return err
	}
}
