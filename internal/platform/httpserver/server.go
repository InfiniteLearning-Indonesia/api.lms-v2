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
	"time"

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
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
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

// ApplicationHandler keeps health endpoints outside the browser identity
// gateway. Readiness is bounded and read-only; all other routes cross the
// identity boundary.
func ApplicationHandler(logger *slog.Logger, ready func(context.Context) error, business http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/live" && r.URL.Path != "/health/ready" {
			business.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			if r.Method != http.MethodHead {
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "METHOD_NOT_ALLOWED"})
			}
			return
		}
		status := http.StatusOK
		body := map[string]string{"status": "alive"}
		if r.URL.Path == "/health/ready" {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			err := ready(ctx)
			cancel()
			if err != nil {
				status, body = http.StatusServiceUnavailable, map[string]string{"status": "not_ready"}
			} else {
				body = map[string]string{"status": "ready"}
			}
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_ = json.NewEncoder(w).Encode(body)
		}
		logger.Info("health_request", "route", r.URL.Path, "status", status)
	})
}

func Serve(ctx context.Context, c config.Config, listener net.Listener, logger *slog.Logger) error {
	return serve(ctx, c, listener, Handler(logger), logger, false)
}

func ServeApplicationTLS(ctx context.Context, c config.Config, listener net.Listener, handler http.Handler, logger *slog.Logger) error {
	return serve(ctx, c, listener, handler, logger, true)
}

func serve(ctx context.Context, c config.Config, listener net.Listener, handler http.Handler, logger *slog.Logger, withTLS bool) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: c.ReadHeaderTimeout, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() {
		if withTLS {
			done <- server.ServeTLS(listener, c.TLSCertFile, c.TLSKeyFile)
			return
		}
		done <- server.Serve(listener)
	}()
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
