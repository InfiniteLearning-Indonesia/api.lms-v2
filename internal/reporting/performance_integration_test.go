//go:build qaintegration

package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyntheticReportPerformancePlanAndPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, e := labdb.Owner(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	runtime, e := labdb.Runtime(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	root := os.Getenv("QA_ARTIFACT_ROOT")
	if root == "" {
		t.Fatal("artifact root required")
	}
	var state map[string]struct {
		Class   string `json:"class_id"`
		Learner string `json:"learner_id"`
		Items   int    `json:"items"`
		Version int    `json:"grade_version"`
	}
	data, e := os.ReadFile(filepath.Join(root, "m14cp04-benchmark-state.json"))
	if e != nil || json.Unmarshal(data, &state) != nil || len(state) != 2 {
		t.Fatal("benchmark state missing")
	}
	result := map[string]any{"status": "PASS_SYNTHETIC_ONLY", "query": "releasedGradesSQL", "tuning": "NONE_NO_BOTTLENECK_PROVEN"}
	plans := map[string]json.RawMessage{}
	for _, name := range []string{"small", "boundary"} {
		s, ok := state[name]
		want, version := 2, 1
		if name == "boundary" {
			want, version = 200, 3
		}
		if !ok || !identifier.MatchString(s.Class) || !identifier.MatchString(s.Learner) || s.Items != want || s.Version != version {
			t.Fatal("invalid benchmark workload")
		}
		var raw []byte
		if e = runtime.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+releasedGradesSQL, s.Class, s.Learner).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		// Retain the native plan even when decoding or acceptance fails.
		path := filepath.Join(root, "m14cp04-plan-"+name+".json")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(append(raw, '\n')); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		if err = validatePerformancePlan(raw, want); err != nil {
			t.Fatalf("%s reporting plan: %v; raw evidence: %s", name, err, path)
		}
		plans[name] = json.RawMessage(raw)
		var releases, revisions, latest, items int
		if e = owner.QueryRow(ctx, `SELECT count(*),count(DISTINCT revision_id) FROM v3.grade_releases WHERE class_id=$1 AND learner_user_id=$2`, s.Class, s.Learner).Scan(&releases, &revisions); e != nil {
			t.Fatal(e)
		}
		if e = owner.QueryRow(ctx, `SELECT count(*),min(v) FROM (SELECT grade_item_id,max(grade_version) v FROM v3.grade_releases WHERE class_id=$1 AND learner_user_id=$2 GROUP BY grade_item_id) x`, s.Class, s.Learner).Scan(&items, &latest); e != nil {
			t.Fatal(e)
		}
		if releases != want*version || revisions != releases || items != want || latest != version {
			t.Fatal("release history changed during load")
		}
	}
	// All three export phases include 120 measured + 8 warmup exports each.
	var audits, broken int
	e = owner.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE o.event_id IS NULL OR o.topic<>'reporting.audit.v1' OR length(split_part(a.action,'/',2))<>32 OR length(split_part(a.action,'/',3))<>64)
 FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.class_id IN ($1,$2) AND a.action LIKE 'reporting.grades.export/%'`, state["small"].Class, state["boundary"].Class).Scan(&audits, &broken)
	if e != nil || audits != 384 || broken != 0 {
		t.Fatalf("export audit/outbox integrity: %d %d %v", audits, broken, e)
	}
	if runtime.Config().MaxConns != 4 {
		t.Fatal("runtime pool cap changed")
	}
	var held []*pgxpool.Conn
	defer func() {
		for _, c := range held {
			c.Release()
		}
	}()
	for i := 0; i < 4; i++ {
		c, err := runtime.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	before := runtime.Stat().CanceledAcquireCount()
	deadline, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	start := time.Now()
	fifth, e := runtime.Acquire(deadline)
	stop()
	waitMS := float64(time.Since(start).Nanoseconds()) / 1e6
	if fifth != nil {
		fifth.Release()
	}
	if !errors.Is(e, context.DeadlineExceeded) || runtime.Stat().CanceledAcquireCount() <= before {
		t.Fatal("saturated pool did not respect cancellation")
	}
	for _, c := range held {
		c.Release()
	}
	recovered, stop := context.WithTimeout(ctx, time.Second)
	start = time.Now()
	conn, e := runtime.Acquire(recovered)
	if e != nil {
		stop()
		t.Fatal("pool failed to recover")
	}
	var one int
	e = conn.QueryRow(recovered, "SELECT 1").Scan(&one)
	conn.Release()
	stop()
	recoveryMS := float64(time.Since(start).Nanoseconds()) / 1e6
	if e != nil || one != 1 || recoveryMS > 1000 {
		t.Fatal("pool recovery failed")
	}
	result["plans"] = plans
	result["export_audits"] = audits
	result["pool"] = map[string]any{"max_connections": 4, "cancel_deadline_ms": 50, "observed_cancel_ms": waitMS, "recovery_ms": recoveryMS, "canceled_acquires": runtime.Stat().CanceledAcquireCount() - before, "acquired_after_release": runtime.Stat().AcquiredConns()}
	b, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(root, "m14cp04-db-plan-pool.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
