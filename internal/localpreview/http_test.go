package localpreview

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLocalPreviewWebBoundaries(t *testing.T) {
	web := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>Preview</title>")},
		"assets/main.js":  {Data: []byte("let test = 1;")},
		"assets/main.map": {Data: []byte("private source map")},
		"assets/.private": {Data: []byte("private fixture")},
		"private.env":     {Data: []byte("private fixture")},
		"favicon.svg":     {Data: []byte("<svg></svg>")},
		"icons.svg":       {Data: []byte("<svg></svg>")},
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	h := withWeb(api, web)
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 200},
		{"GET", "/auth/google/callback?code=fixture", 200},
		{"GET", "/v3/me/classes", 401},
		{"GET", "/health/unmatched", 401},
		{"GET", "/assets/main.js", 200},
		{"GET", "/assets/main.map", 404},
		{"GET", "/assets/.private", 404},
		{"GET", "/assets/", 404},
		{"GET", "/private.env", 404},
		{"GET", "/favicon.svg", 200},
		{"GET", "/icons.svg", 200},
		{"GET", "/assets/../private.env", 404},
		{"POST", "/auth/google/callback", 404},
	} {
		result := httptest.NewRecorder()
		h.ServeHTTP(result, httptest.NewRequest(test.method, test.path, nil))
		if result.Code != test.status || strings.Contains(result.Body.String(), "private") {
			t.Fatalf("%s %s: status %d, expected %d", test.method, test.path, result.Code, test.status)
		}
		if strings.HasPrefix(test.path, "/auth/") && result.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing callback referrer protection")
		}
	}
}
