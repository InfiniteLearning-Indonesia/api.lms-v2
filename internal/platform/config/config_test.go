package config

import "testing"

func TestLocalConfigOnly(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "127.0.0.1:8080", "[::1]:8080"} {
		c, err := Load(func(k string) string {
			if k == "LMS_MODE" {
				return "local-synthetic"
			}
			return addr
		})
		if err != nil || c.ReadHeaderTimeout <= 0 || c.ShutdownTimeout <= 0 {
			t.Fatalf("local config rejected: %v", err)
		}
	}
}

func TestFailClosed(t *testing.T) {
	for _, tc := range []struct{ mode, addr string }{{"", "127.0.0.1:0"}, {"production", "127.0.0.1:8080"}, {"staging", "127.0.0.1:8080"}, {"local-synthetic", ""}, {"local-synthetic", ":8080"}, {"local-synthetic", "0.0.0.0:8080"}, {"local-synthetic", "example.invalid:8080"}, {"local-synthetic", "127.0.0.1:99999"}, {"local-synthetic", "127.0.0.1:-1"}, {"local-synthetic", "127.0.0.1:http"}} {
		_, err := Load(func(k string) string {
			if k == "LMS_MODE" {
				return tc.mode
			}
			return tc.addr
		})
		if err == nil {
			t.Fatalf("unsafe config accepted: %+v", tc)
		}
	}
}
