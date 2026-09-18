//go:build qaintegration

package foundation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"lmsv3/internal/labdb"
)

func mappingPlanA() Plan {
	p := NewSyntheticPlan(labdb.TargetFromEnvironment(), "cp03-mapping-a")
	withRows(&p,
		Row{Table: "classes", PK: "class-one", Version: "v1", RawSHA256: digest("class-one-raw"), SealedRef: "sealed/classes/one"},
		Row{Table: "classes", PK: "class-two", Version: "v1", RawSHA256: digest("class-two-raw"), SealedRef: "sealed/classes/two"},
		Row{Table: "competency_scores", PK: "score-one", Version: "v1", RawSHA256: digest("score-one-raw"), SealedRef: "sealed/scores/one"},
		Row{Table: "batches", PK: "batch-one", Version: "v1", RawSHA256: digest("batch-one-raw"), SealedRef: "sealed/batches/one"},
		Row{Table: "users", PK: "user-one", Version: "v1", RawSHA256: digest("user-one-raw"), SealedRef: "sealed/users/one"})
	return p
}

func mappingPlanB() Plan {
	p := NewSyntheticPlan(labdb.TargetFromEnvironment(), "cp03-mapping-b")
	withRows(&p,
		Row{Table: "classes", PK: "class-ambiguous", Version: "v1", RawSHA256: digest("class-ambiguous-v1"), SealedRef: "sealed/classes/ambiguous-v1"},
		Row{Table: "classes", PK: "class-ambiguous", Version: "v2", RawSHA256: digest("class-ambiguous-v2"), SealedRef: "sealed/classes/ambiguous-v2"},
		Row{Table: "classes", PK: "class-three", Version: "v1", RawSHA256: digest("class-three-raw"), SealedRef: "sealed/classes/three"},
		Row{Table: "rubrik_assessment_scores", PK: "rubric-score", Version: "v1", RawSHA256: digest("rubric-score-raw"), SealedRef: "sealed/scores/rubric"},
		Row{Table: "custom_tracking", PK: "unknown-one", Version: "v1", RawSHA256: digest("unknown-table-raw"), SealedRef: "sealed/unknown/one"},
		Row{Table: "session", PK: "session-one", Version: "v1", RawSHA256: digest("session-one-raw"), SealedRef: "sealed/session/one"})
	return p
}

func TestSyntheticMappingDecisions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = labdb.InstallMigrationStage(ctx, pool, "m15-cp03"); err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallMigrationStage(ctx, pool, "m15-cp03"); err != nil {
		t.Fatal("schema replay", err)
	}
	p := mappingPlanA()
	if err = Capture(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	var beforeClass, beforeOutbox int
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&beforeClass) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3.outbox").Scan(&beforeOutbox) != nil {
		t.Fatal("domain baseline")
	}
	decisions, err := Decide(ctx, pool, p, "cp03-run-a", "v1", "qa-domain-owner")
	if err != nil || len(decisions) != 5 {
		t.Fatal("mapping incomplete", err, len(decisions))
	}
	counts := map[string]int{}
	targets := map[string]bool{}
	reasons := map[string]bool{}
	for _, d := range decisions {
		counts[d.Disposition]++
		reasons[d.ReasonCode] = true
		if d.TargetID != "" {
			if targets[d.TargetID] {
				t.Fatal("Class identities merged")
			}
			targets[d.TargetID] = true
		}
	}
	if counts["MAP_CLASS"] != 2 || counts["QUARANTINE"] != 2 || counts["ARCHIVE_ONLY"] != 1 || !reasons["SCORE_CLASS_AMBIGUOUS"] || !reasons["BATCH_CHRONOLOGY_AMBIGUOUS"] {
		t.Fatal("wrong mapping decisions", counts, reasons)
	}
	if again, e := Decide(ctx, pool, p, "cp03-run-a", "v1", "qa-domain-owner"); e != nil || len(again) != len(decisions) {
		t.Fatal("mapping replay", e)
	}
	var mapped, afterClass, afterOutbox int
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.mapping_decisions WHERE run_id='cp03-run-a'").Scan(&mapped) != nil || mapped != 5 {
		t.Fatal("decision ledger incomplete", mapped)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&afterClass) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3.outbox").Scan(&afterOutbox) != nil || beforeClass != afterClass || beforeOutbox != afterOutbox {
		t.Fatal("mapping wrote operational domain")
	}
}

func TestSyntheticMappingConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	p := mappingPlanB()
	if err = Capture(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	decisions, err := Decide(ctx, pool, p, "cp03-run-b", "v1", "qa-domain-owner")
	if err != nil || len(decisions) != 6 {
		t.Fatal("mapping incomplete", err, len(decisions))
	}
	counts := map[string]int{}
	reasons := map[string]int{}
	for _, d := range decisions {
		counts[d.Disposition]++
		reasons[d.ReasonCode]++
		if d.PK == "class-ambiguous" && d.TargetID != "" {
			t.Fatal("ambiguous Class mapped")
		}
	}
	if counts["MAP_CLASS"] != 1 || counts["QUARANTINE"] != 4 || counts["ARCHIVE_ONLY"] != 1 || reasons["CLASS_VERSION_AMBIGUOUS"] != 2 || reasons["SCORE_CLASS_AMBIGUOUS"] != 1 || reasons["UNREVIEWED_SOURCE_TABLE"] != 1 || reasons["AUTH_SESSION_RESTRICTED"] != 1 {
		t.Fatal("conflict disposition missing", counts, reasons)
	}
	if _, err = Decide(ctx, pool, p, "different-run", "v1", "qa-domain-owner"); !errors.Is(err, ErrConflict) {
		t.Fatal("same version different run accepted", err)
	}
	if _, err = Decide(ctx, pool, p, "cp03-run-b2", "v2", "qa-domain-owner"); err != nil {
		t.Fatal("new mapping version rejected", err)
	}
	var reviewer string
	if err = pool.QueryRow(ctx, "SELECT reviewer FROM v3migration.mapping_runs WHERE run_id='cp03-run-b'").Scan(&reviewer); err != nil || reviewer != "qa-domain-owner" {
		t.Fatal("review provenance missing", err)
	}
}

func TestSyntheticAtomicBackfill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = labdb.InstallMigrationStage(ctx, pool, "m15-cp04"); err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallMigrationStage(ctx, pool, "m15-cp04"); err != nil {
		t.Fatal("schema replay", err)
	}
	var beforeClass, beforeOutbox int
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&beforeClass) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3.outbox").Scan(&beforeOutbox) != nil {
		t.Fatal("domain baseline")
	}
	if err = backfill(ctx, pool, "cp03-run-a", true); !errors.Is(err, ErrBackfill) {
		t.Fatal("injected failure accepted", err)
	}
	for _, table := range []string{"shadow_classes", "origin_mappings", "import_receipts", "import_checkpoints"} {
		var count int
		query := "SELECT count(*) FROM v3migration." + table + " WHERE " + map[string]string{"shadow_classes": "mapped_run_id", "origin_mappings": "run_id", "import_receipts": "run_id", "import_checkpoints": "run_id"}[table] + "='cp03-run-a'"
		if pool.QueryRow(ctx, query).Scan(&count) != nil || count != 0 {
			t.Fatal("partial transaction persisted", table, count)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- Backfill(ctx, pool, "cp03-run-a") }()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrBackfill) {
			t.Fatal("concurrent import failed unexpectedly", e)
		}
	}
	if success < 1 {
		t.Fatal("no import committed")
	}
	if err = Backfill(ctx, pool, "cp03-run-a"); err != nil {
		t.Fatal("same-run replay", err)
	}
	var shadows, maps, receipts, checkpoints, afterClass, afterOutbox int
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.shadow_classes WHERE mapped_run_id='cp03-run-a'").Scan(&shadows) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.origin_mappings WHERE run_id='cp03-run-a'").Scan(&maps) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.import_receipts WHERE run_id='cp03-run-a'").Scan(&receipts) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.import_checkpoints WHERE run_id='cp03-run-a'").Scan(&checkpoints) != nil {
		t.Fatal("import accounting query")
	}
	if shadows != 2 || maps != 2 || receipts != 2 || checkpoints != 1 {
		t.Fatal("atomic accounting mismatch", shadows, maps, receipts, checkpoints)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&afterClass) != nil || pool.QueryRow(ctx, "SELECT count(*) FROM v3.outbox").Scan(&afterOutbox) != nil || beforeClass != afterClass || beforeOutbox != afterOutbox {
		t.Fatal("shadow import changed operational state")
	}
}

func TestSyntheticOwnerFenceReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	p := mappingPlanB()
	if err = AdvanceOwner(ctx, pool, p.SourceEnvironment, p.SourceInstanceID, "class-three", p.TargetNamespace, 0, "SHADOW_ONLY"); err != nil {
		t.Fatal("shadow epoch", err)
	}
	if err = Backfill(ctx, pool, "cp03-run-b"); err != nil {
		t.Fatal("shadow owner import", err)
	}
	if err = AdvanceOwner(ctx, pool, p.SourceEnvironment, p.SourceInstanceID, "class-three", p.TargetNamespace, 1, "V3"); err != nil {
		t.Fatal("v3 epoch", err)
	}
	if err = Backfill(ctx, pool, "cp03-run-b"); !errors.Is(err, ErrOwnerFence) {
		t.Fatal("V3 owner replay accepted", err)
	}
	if err = AdvanceOwner(ctx, pool, p.SourceEnvironment, p.SourceInstanceID, "class-three", p.TargetNamespace, 2, "SHADOW_ONLY"); !errors.Is(err, ErrOwnerFence) {
		t.Fatal("V3 downgraded", err)
	}
	if _, err = Decide(ctx, pool, p, "cp03-run-b3", "v3", "qa-domain-owner"); err != nil {
		t.Fatal("new reviewed run", err)
	}
	if err = Backfill(ctx, pool, "cp03-run-b3"); !errors.Is(err, ErrBackfill) {
		t.Fatal("new mapping reused stale backup", err)
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.import_checkpoints WHERE run_id='cp03-run-b3'").Scan(&count) != nil || count != 0 {
		t.Fatal("stale correction committed")
	}
}
