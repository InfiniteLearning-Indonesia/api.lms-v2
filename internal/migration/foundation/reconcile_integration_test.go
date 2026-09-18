//go:build qaintegration

package foundation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
)

func TestSyntheticReconciliationComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	before, err := countOperational(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(ctx, pool, "cp03-run-a")
	if err != nil {
		t.Fatal(err)
	}
	if report.RunID != "cp03-run-a" || report.TableScopes != len(BaselineTables)+1 || report.FileScopes != 1 || report.LandingRows != 5 || report.DecisionRows != 5 || report.MappedClasses != 2 || report.Quarantined != 2 || report.Archived != 1 || report.ShadowClasses != 2 || report.OriginMappings != 2 || report.Receipts != 2 || !validHash(report.EvidenceHash) {
		t.Fatal("reconciliation coverage incomplete", report)
	}
	again, err := Reconcile(ctx, pool, "cp03-run-a")
	if err != nil || again != report {
		t.Fatal("read-only replay changed result", err, again)
	}
	after, err := countOperational(ctx, pool)
	if err != nil || after != before {
		t.Fatal("reconciliation wrote operational state", err, before, after)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	var forbidden int
	if err = runtime.QueryRow(ctx, "SELECT count(*) FROM v3migration.shadow_classes").Scan(&forbidden); err == nil {
		t.Fatal("runtime role can read private migration shadow")
	}
}

func TestSyntheticReconciliationRejectsIncomplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = Reconcile(ctx, pool, "cp03-run-b2"); !errors.Is(err, ErrReconciliation) {
		t.Fatal("unimported mapping accepted", err)
	}
	if _, err = Reconcile(ctx, pool, "cp03-run-b"); !errors.Is(err, ErrReconciliation) {
		t.Fatal("V3-owned unpublished shadow accepted", err)
	}
	if _, err = Reconcile(ctx, pool, "missing-run"); !errors.Is(err, ErrReconciliation) {
		t.Fatal("missing run accepted", err)
	}
}

func countOperational(ctx context.Context, pool *pgxpool.Pool) ([2]int, error) {
	var c [2]int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&c[0]); err != nil {
		return c, err
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM v3.outbox").Scan(&c[1]); err != nil {
		return c, err
	}
	return c, nil
}
