//go:build qaintegration

package classes

import (
	"context"
	"lmsv3/internal/labdb"
	"testing"
	"time"
)

func TestSyntheticClassAcademicAncestry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var total, invalid int
	err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE e.id IS NULL OR ep.id IS NULL OR cr.id IS NULL OR gr.id IS NULL OR gi.id IS NULL OR gr.class_id<>s.class_id OR gr.learner_user_id<>s.owner_user_id OR gi.activity_id<>s.activity_id)
	 FROM v3.grade_releases rel
	 JOIN v3.grade_revisions gr ON gr.id=rel.revision_id
	 JOIN v3.submission_aggregates s ON s.id=gr.aggregate_id
	 LEFT JOIN v3.enrollments e ON e.id=s.enrollment_id AND e.class_id=s.class_id AND e.user_id=s.owner_user_id
	 LEFT JOIN v3.episodes ep ON ep.id=s.episode_id AND ep.enrollment_id=s.enrollment_id
	 LEFT JOIN v3.content_revisions cr ON cr.id=s.assignment_revision_id AND cr.class_id=s.class_id AND cr.activity_id=s.activity_id
	 LEFT JOIN v3.grade_items gi ON gi.id=gr.grade_item_id AND gi.class_id=s.class_id`).Scan(&total, &invalid)
	if err != nil || total == 0 || invalid != 0 {
		t.Fatalf("academic ancestry: total=%d invalid=%d err=%v", total, invalid, err)
	}
}

func TestSyntheticClassAcademicHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var retakes, invalid int
	err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE old.id IS NULL OR old.ended_at IS NULL OR ep.enrollment_id<>old.enrollment_id OR ep.recognition_policy<>'SEPARATE_HISTORY')
	 FROM v3.episodes ep LEFT JOIN v3.episodes old ON old.id=ep.origin_episode_id WHERE ep.kind='RETAKE'`).Scan(&retakes, &invalid)
	if err != nil || retakes == 0 || invalid != 0 {
		t.Fatalf("retake provenance: retakes=%d invalid=%d err=%v", retakes, invalid, err)
	}
	var moved int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM v3.grade_revisions gr JOIN v3.submission_aggregates s ON s.id=gr.aggregate_id JOIN v3.episodes ep ON ep.id=s.episode_id WHERE ep.kind='RETAKE' AND gr.created_at<ep.opened_at`).Scan(&moved)
	if err != nil || moved != 0 {
		t.Fatalf("grade inherited across retake: moved=%d err=%v", moved, err)
	}
}
