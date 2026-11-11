package reporting

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Shared by the native DB probe and this database-free regression.
func validatePerformancePlan(raw []byte, want int) error {
	var plan []struct {
		Plan struct {
			Rows *float64 `json:"Actual Rows"`
		} `json:"Plan"`
		MS *float64 `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("EXPLAIN JSON decode: %w", err)
	}
	if len(plan) != 1 {
		return fmt.Errorf("EXPLAIN expected one plan, got %d", len(plan))
	}
	if plan[0].Plan.Rows == nil || plan[0].MS == nil {
		return fmt.Errorf("EXPLAIN missing/null Actual Rows or Execution Time")
	}
	// PostgreSQL 18 emits decimal row counts, e.g. 2.00. Compare exactly:
	// converting to int would silently accept a wrong fractional count.
	if *plan[0].Plan.Rows != float64(want) {
		return fmt.Errorf("EXPLAIN rows=%g expected=%d", *plan[0].Plan.Rows, want)
	}
	if *plan[0].MS < 0 || *plan[0].MS > 500 {
		return fmt.Errorf("EXPLAIN execution_ms=%g outside frozen [0,500] bound", *plan[0].MS)
	}
	return nil
}

func TestPerformanceExplainNumbers(t *testing.T) {
	for _, sample := range []struct {
		name string
		raw  string
		want int
		pass bool
	}{
		{"legacy-integer", `[{"Plan":{"Actual Rows":2},"Execution Time":1.25}]`, 2, true},
		{"postgres18-decimal", `[{"Plan":{"Actual Rows":2.00},"Execution Time":1.25}]`, 2, true},
		{"boundary-decimal", `[{"Plan":{"Actual Rows":200.00},"Execution Time":500.00}]`, 200, true},
		{"wrong-fraction-no-truncation", `[{"Plan":{"Actual Rows":2.01},"Execution Time":1}]`, 2, false},
		{"wrong-row-count", `[{"Plan":{"Actual Rows":3},"Execution Time":1}]`, 2, false},
		{"too-slow", `[{"Plan":{"Actual Rows":2},"Execution Time":500.01}]`, 2, false},
		{"negative-time", `[{"Plan":{"Actual Rows":2},"Execution Time":-1}]`, 2, false},
		{"missing-time", `[{"Plan":{"Actual Rows":2}}]`, 2, false},
		{"null-time", `[{"Plan":{"Actual Rows":2},"Execution Time":null}]`, 2, false},
		{"missing-plan", `[{"Execution Time":1}]`, 2, false},
		{"empty", `[]`, 2, false},
		{"multiple-plans", `[{},{}]`, 2, false},
		{"quoted-row", `[{"Plan":{"Actual Rows":"2"},"Execution Time":1}]`, 2, false},
		{"malformed", `not json`, 2, false},
	} {
		if err := validatePerformancePlan([]byte(sample.raw), sample.want); (err == nil) != sample.pass {
			t.Errorf("%s: accepted=%t want=%t error=%v", sample.name, err == nil, sample.pass, err)
		}
	}
}
