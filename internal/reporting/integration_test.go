//go:build qaintegration

package reporting

import (
	"context"
	"testing"
	"time"

	"lmsv3/internal/labdb"
)

func TestSyntheticReportIntegrity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	queries := []string{
		`SELECT count(*) FROM v3.grade_releases l LEFT JOIN v3.grade_revisions r ON r.id=l.revision_id AND r.class_id=l.class_id WHERE r.id IS NULL OR l.learner_user_id<>r.learner_user_id`,
		`SELECT count(*) FROM v3.grade_releases l LEFT JOIN v3.grade_items gi ON gi.id=l.grade_item_id AND gi.class_id=l.class_id WHERE gi.id IS NULL`,
	}
	for _, query := range queries {
		var count int
		if err = owner.QueryRow(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("report source integrity: %d %v", count, err)
		}
	}
	var index bool
	err = owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='v3' AND tablename='grade_releases' AND indexname='grade_releases_learner_idx')`).Scan(&index)
	if err != nil || !index {
		t.Fatalf("report source index missing: %v", err)
	}
}

func TestSyntheticReportExportAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var count, broken int
	err = owner.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE o.event_id IS NULL OR o.topic<>'reporting.audit.v1' OR a.class_id IS NULL OR length(split_part(a.action,'/',2))<>32 OR length(split_part(a.action,'/',3))<>64)
	FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'reporting.grades.export/%'`).Scan(&count, &broken)
	if err != nil || count < 2 || broken != 0 {
		t.Fatalf("export audit/outbox missing or malformed: %d %d %v", count, broken, err)
	}
	var mutation bool
	err = owner.QueryRow(ctx, `SELECT has_table_privilege('qa_runtime','v3.grade_releases','DELETE')`).Scan(&mutation)
	if err != nil || mutation {
		t.Fatalf("runtime deletion boundary: %v", err)
	}
}
