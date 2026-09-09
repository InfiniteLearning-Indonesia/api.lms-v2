package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

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

func TestBusinessRuntimeConfig(t *testing.T) {
	root := t.TempDir()
	cert, key := filepath.Join(root, "tls.crt"), filepath.Join(root, "tls.key")
	if os.WriteFile(cert, []byte("certificate"), 0600) != nil || os.WriteFile(key, []byte("private"), 0600) != nil {
		t.Fatal("fixture")
	}
	values := map[string]string{"LMS_MODE": "v3-local-synthetic", "LMS_HTTP_ADDR": "127.0.0.1:9443", "LMS_ORIGIN": "https://127.0.0.1:9443", "LMS_TLS_CERT_FILE": cert, "LMS_TLS_KEY_FILE": key, "LMS_BRIDGE_PUBLIC_KEY": hex.EncodeToString(make([]byte, 32))}
	if _, err := Load(func(k string) string { return values[k] }); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"LMS_ORIGIN", "LMS_TLS_CERT_FILE", "LMS_TLS_KEY_FILE", "LMS_BRIDGE_PUBLIC_KEY"} {
		old := values[missing]
		values[missing] = ""
		if _, err := Load(func(k string) string { return values[k] }); err == nil {
			t.Fatalf("missing %s accepted", missing)
		}
		values[missing] = old
	}
}
