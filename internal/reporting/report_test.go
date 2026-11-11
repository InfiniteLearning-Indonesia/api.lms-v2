package reporting

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReportingContract(t *testing.T) {
	content, err := os.ReadFile("../../api/openapi/reporting-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(content, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.OpenAPI != "3.1.0" || len(spec.Paths) != 3 || spec.Paths["/v3/classes/{class_id}/reports/grades/{learner_user_id}"] == nil || spec.Paths["/v3/classes/{class_id}/reports/grades/{learner_user_id}/pages"] == nil || spec.Paths["/v3/classes/{class_id}/reports/grades/{learner_user_id}/exports"] == nil {
		t.Fatal("report contract drift")
	}
	if !OwnsPath("/v3/classes/11111111111111111111111111111111/reports/grades/22222222222222222222222222222222") {
		t.Fatal("route missing")
	}
	if identifier.MatchString("not-an-id") || !identifier.MatchString("11111111111111111111111111111111") {
		t.Fatal("identifier boundary")
	}
}

func TestReportingHTTPBoundary(t *testing.T) {
	handler := Handler(Store{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	base := "/v3/classes/11111111111111111111111111111111/reports/grades/22222222222222222222222222222222"
	for _, sample := range []struct {
		path  string
		code  int
		label string
	}{
		{base + "/export", 404, "NOT_FOUND"},
		{base + "?unexpected=1", 400, "INVALID_INPUT"},
		{base, 401, "UNAUTHENTICATED"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, sample.path, nil))
		if rec.Code != sample.code || !strings.Contains(rec.Body.String(), sample.label) || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("boundary %s: %d %s", sample.path, rec.Code, rec.Body.String())
		}
	}
}
