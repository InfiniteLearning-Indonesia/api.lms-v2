//go:build qaintegration

package content

import (
	"context"
	"lmsv3/internal/labdb"
	"testing"
	"time"
)

func TestSyntheticContentClosureOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var referenced, invalid int
	err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE e.id IS NULL OR ep.id IS NULL OR r.id IS NULL)
		FROM v3.submission_aggregates s
		LEFT JOIN v3.enrollments e ON e.id=s.enrollment_id AND e.class_id=s.class_id AND e.user_id=s.owner_user_id
		LEFT JOIN v3.episodes ep ON ep.id=s.episode_id AND ep.enrollment_id=s.enrollment_id
		LEFT JOIN v3.content_revisions r ON r.id=s.assignment_revision_id AND r.class_id=s.class_id AND r.activity_id=s.activity_id
		WHERE r.state='WITHDRAWN'`).Scan(&referenced, &invalid)
	if err != nil || referenced == 0 || invalid != 0 {
		t.Fatalf("historical ownership: referenced=%d invalid=%d err=%v", referenced, invalid, err)
	}
}

func TestSyntheticContentClosureClone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var cloned, invalid int
	err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE source.id IS NULL OR target.id IS NULL OR target.state<>'DRAFT'
		OR (SELECT count(*) FROM v3.content_sections s WHERE s.class_id=target.id)<>jsonb_array_length(c.response->'sections')
		OR (SELECT count(*) FROM v3.content_activities a WHERE a.class_id=target.id)<>jsonb_array_length(c.response->'activities')
		OR EXISTS(SELECT 1 FROM v3.content_revisions r WHERE r.class_id=target.id)
		OR EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles role ON role.enrollment_id=e.id WHERE e.class_id=target.id AND role.role='student')
		OR NOT EXISTS(SELECT 1 FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id WHERE a.class_id=target.id AND a.action='content.clone_structure'))
		FROM v3.content_commands c
		LEFT JOIN v3.classes source ON source.id=c.response->>'source_class_id'
		LEFT JOIN v3.classes target ON target.id=c.response->>'target_class_id'
		WHERE c.response ? 'source_class_id'`).Scan(&cloned, &invalid)
	if err != nil || cloned < 2 || invalid != 0 {
		t.Fatalf("structure clone: cloned=%d invalid=%d err=%v", cloned, invalid, err)
	}
}
