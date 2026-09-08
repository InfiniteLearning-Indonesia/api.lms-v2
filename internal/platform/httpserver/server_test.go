package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lmsv3/internal/platform/config"
)

func TestHealthAndRedaction(t *testing.T) {
	var logs bytes.Buffer
	h := Handler(slog.New(slog.NewJSONHandler(&logs, nil)))
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/health/live", 200}, {"HEAD", "/health/live", 200}, {"GET", "/health/ready", 503}, {"POST", "/health/live", 405}, {"GET", "/DO_NOT_EMIT?sid=DO_NOT_EMIT", 404}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("DO_NOT_EMIT"))
		r.Header.Set("Authorization", "Bearer DO_NOT_EMIT")
		r.Header.Set("Cookie", "sid=DO_NOT_EMIT")
		r.Header.Set("X-Request-ID", "DO_NOT_EMIT")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
		if len(w.Header().Get("X-Request-ID")) != 32 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing safe headers")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD returned body")
		}
		if strings.Contains(w.Body.String(), "DO_NOT_EMIT") {
			t.Fatal("response reflected untrusted input")
		}
	}
	if strings.Contains(logs.String(), "DO_NOT_EMIT") {
		t.Fatal("credentials or raw URL logged")
	}
}

func TestGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := config.Load(func(k string) string {
		if k == "LMS_MODE" {
			return "local-synthetic"
		}
		return "127.0.0.1:0"
	})
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, c, listener, slog.New(slog.DiscardHandler)) }()
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/health/live")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}
