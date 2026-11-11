package reportbench

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func fixture(url string) Config {
	return Config{BaseURL: url, Session: strings.Repeat("a", 64), CSRF: strings.Repeat("b", 64), ClassID: strings.Repeat("c", 32), LearnerID: strings.Repeat("d", 32), Kind: "report", Requests: 8, Concurrency: 4, Warmups: 2, ExpectedItems: 2, ExpectedVersion: 1}
}
func TestLoadConfigurationBoundary(t *testing.T) {
	c := fixture("https://127.0.0.1:443")
	if c.Validate() != nil {
		t.Fatal("valid loopback rejected")
	}
	for _, base := range []string{"http://127.0.0.1:443", "https://example.com:443", "https://localhost:443", "https://127.0.0.1", "https://127.0.0.1:443/path", "https://127.0.0.1:443?x=1", "https://user@127.0.0.1:443"} {
		v := c
		v.BaseURL = base
		if v.Validate() == nil {
			t.Fatal("unsafe target accepted")
		}
	}
	for _, edit := range []func(*Config){func(v *Config) { v.Requests = 31 }, func(v *Config) { v.Concurrency = 8 }, func(v *Config) { v.Kind = "delete" }, func(v *Config) { v.Session = "x" }, func(v *Config) { v.ExpectedItems = 201 }, func(v *Config) { v.Warmups = 0 }} {
		v := c
		edit(&v)
		if v.Validate() == nil {
			t.Fatal("unsafe bound accepted")
		}
	}
	b, _ := json.Marshal(c)
	if _, e := Decode(strings.NewReader(string(b))); e != nil {
		t.Fatal("decode rejected valid config")
	}
	for _, b := range []string{string(b) + ` {}`, `{"unknown":1}`, strings.Repeat("x", 4097)} {
		if _, e := Decode(strings.NewReader(b)); e == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
func TestNearestRankPercentile(t *testing.T) {
	v := []float64{5, 1, 4, 2, 3}
	if Percentile(v, .5) != 3 || Percentile(v, .95) != 5 || Percentile(v, .99) != 5 || v[0] != 5 || Percentile(nil, .5) != 0 {
		t.Fatal("nearest-rank changed")
	}
}
func TestLoadMetricsAndContract(t *testing.T) {
	var mode atomic.Value
	mode.Store("valid")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := mode.Load().(string)
		cookie, e := r.Cookie("__Host-lms-v3")
		if e != nil || cookie.Value != strings.Repeat("a", 64) || r.Header.Get("Origin") == "" || r.Header.Get("X-CSRF-Token") != strings.Repeat("b", 64) {
			t.Error("request authority omitted")
		}
		if current == "redirect" {
			http.Redirect(w, r, "https://example.invalid", 302)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if current == "invalid" {
			w.Write([]byte(`{}`))
			return
		}
		if current == "oversized" {
			w.Write([]byte(strings.Repeat("x", 262145)))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/exports") {
			if r.Method != "POST" {
				t.Error("export method changed")
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="grades.csv"`)
			w.Header().Set("X-Export-Audit-ID", strings.Repeat("e", 64))
			w.Header().Set("X-Report-Snapshot", strings.Repeat("f", 64))
			writer := csv.NewWriter(w)
			writer.Write([]string{"class_id", "learner_user_id", "grade_item_id", "title", "value_milli", "grade_version", "release_id", "released_at"})
			for _, id := range []string{"1", "3"} {
				writer.Write([]string{strings.Repeat("c", 32), strings.Repeat("d", 32), strings.Repeat(id, 32), "Essay", "70000", "1", strings.Repeat("2", 32), "2026-11-02T00:00:00Z"})
			}
			writer.Flush()
			return
		}
		v := map[string]any{"contract_version": 1, "class_id": strings.Repeat("c", 32), "learner_user_id": strings.Repeat("d", 32), "snapshot": strings.Repeat("e", 64), "next_cursor": "", "grades": []map[string]any{{"grade_item_id": strings.Repeat("1", 32), "grade_version": 1, "release_id": strings.Repeat("2", 32)}, {"grade_item_id": strings.Repeat("3", 32), "grade_version": 1, "release_id": strings.Repeat("4", 32)}}}
		json.NewEncoder(w).Encode(v)
	}))
	defer server.Close()
	c := fixture(server.URL)
	for _, kind := range []string{"report", "page", "export"} {
		c.Kind = kind
		r, e := Run(context.Background(), c)
		if e != nil || r.Errors != 0 || r.WarmupErrors != 0 || len(r.Latencies) != 8 || r.P95 <= 0 || r.Throughput <= 0 {
			t.Fatal("native TLS metrics/contract failed")
		}
	}
	for _, bad := range []string{"invalid", "redirect", "oversized"} {
		mode.Store(bad)
		r, e := Run(context.Background(), c)
		if e != nil || r.Errors != 8 || r.WarmupErrors != 2 {
			t.Fatal("invalid response accepted")
		}
	}
}
