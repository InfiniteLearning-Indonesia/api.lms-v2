//go:build qaintegration

package aisuggestions

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/labdb"
)

func TestSyntheticAISuggestionInstall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := labdb.InstallAISuggestions(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallAISuggestions(ctx, owner); err != nil {
		t.Fatal("migration replay", err)
	}
	_, err = owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.ai_suggestions,v3.ai_suggestion_commands TO qa_runtime`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyntheticAISuggestionProvenance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	queries := []string{
		`SELECT count(*) FROM v3.ai_suggestions s LEFT JOIN v3.submission_revisions r ON r.id=s.submission_revision_id AND r.class_id=s.class_id LEFT JOIN v3.submission_aggregates a ON a.id=s.aggregate_id AND a.class_id=s.class_id LEFT JOIN v3.grade_items gi ON gi.id=s.grade_item_id AND gi.class_id=s.class_id WHERE r.id IS NULL OR a.id IS NULL OR gi.id IS NULL OR a.activity_id<>gi.activity_id OR r.aggregate_id<>a.id OR r.body_sha256<>s.submission_sha256`,
		`SELECT count(*) FROM v3.ai_suggestions s LEFT JOIN v3.grade_definition_versions d ON d.id=s.definition_version_id AND d.grade_item_id=s.grade_item_id AND d.class_id=s.class_id WHERE d.id IS NULL OR s.prompt_sha256 !~ '^[a-f0-9]{64}$' OR s.rubric_sha256 !~ '^[a-f0-9]{64}$'`,
		`SELECT count(*) FROM v3.ai_suggestion_commands c LEFT JOIN v3.ai_suggestions s ON s.id=c.suggestion_id WHERE s.id IS NULL OR c.actor_id<>s.requested_by`,
		`SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='ai.suggestion.created' AND o.event_id IS NULL`,
	}
	for _, query := range queries {
		var count int
		if err := owner.QueryRow(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatal(query, count, err)
		}
	}
	var succeeded, failed int
	if err := owner.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='SUCCEEDED'),count(*) FILTER (WHERE status='FAILED') FROM v3.ai_suggestions`).Scan(&succeeded, &failed); err != nil || succeeded < 1 || failed < 1 {
		t.Fatal("missing successful/failed evidence", succeeded, failed, err)
	}
	type recorded struct{ definitionID, rubricHash, promptHash string }
	var suggestions []recorded
	rows, err := owner.Query(ctx, `SELECT definition_version_id,rubric_sha256,prompt_sha256 FROM v3.ai_suggestions`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var item recorded
		if err := rows.Scan(&item.definitionID, &item.rubricHash, &item.promptHash); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		suggestions = append(suggestions, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range suggestions {
		if item.promptHash != digest([]byte(promptTemplate)) {
			t.Fatal("persisted prompt does not match provider input")
		}
		rubric := RubricSnapshot{DefinitionVersionID: item.definitionID}
		if err := owner.QueryRow(ctx, `SELECT s.minimum_milli,s.maximum_milli FROM v3.grade_definition_versions d JOIN v3.grade_scales s ON s.id=d.scale_id AND s.class_id=d.class_id WHERE d.id=$1`, item.definitionID).Scan(&rubric.MinimumMilli, &rubric.MaximumMilli); err != nil {
			t.Fatal(err)
		}
		criteria, err := owner.Query(ctx, `SELECT id,criterion_key,title,parent_criterion_id,weight_bps,maximum_milli,position FROM v3.rubric_criteria WHERE definition_version_id=$1 ORDER BY position,id`, item.definitionID)
		if err != nil {
			t.Fatal(err)
		}
		for criteria.Next() {
			var criterion RubricCriterion
			if err := criteria.Scan(&criterion.ID, &criterion.Key, &criterion.Title, &criterion.ParentID, &criterion.WeightBPS, &criterion.MaxMilli, &criterion.Position); err != nil {
				criteria.Close()
				t.Fatal(err)
			}
			rubric.Criteria = append(rubric.Criteria, criterion)
		}
		err = criteria.Err()
		criteria.Close()
		if err != nil {
			t.Fatal(err)
		}
		hash, err := rubricDigest(rubric)
		if err != nil || item.rubricHash != hash {
			t.Fatal("persisted rubric does not match published academic context", err)
		}
	}
}

func TestSyntheticAISuggestionBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var invalid int
	err = owner.QueryRow(ctx, `SELECT count(*) FROM v3.grade_revisions g JOIN v3.ai_suggestions s ON s.id=g.ai_suggestion_id WHERE s.status<>'SUCCEEDED' OR s.class_id<>g.class_id OR s.grade_item_id<>g.grade_item_id OR s.aggregate_id<>g.aggregate_id OR s.submission_revision_id<>g.submission_revision_id OR s.definition_version_id<>g.definition_version_id`).Scan(&invalid)
	if err != nil || invalid != 0 {
		t.Fatal("invalid grade source", invalid, err)
	}
	err = owner.QueryRow(ctx, `SELECT count(*) FROM v3.ai_suggestions WHERE status='FAILED' AND (proposed_value_milli IS NOT NULL OR failure_code IS NULL)`).Scan(&invalid)
	if err != nil || invalid != 0 {
		t.Fatal("failure produced grade", invalid, err)
	}
	var linked int
	err = owner.QueryRow(ctx, `SELECT count(*) FROM v3.grade_revisions WHERE ai_suggestion_id IS NOT NULL`).Scan(&linked)
	if err != nil || linked < 1 {
		t.Fatal("human grade provenance absent", linked, err)
	}
	var unsafe bool
	err = owner.QueryRow(ctx, `SELECT has_table_privilege('qa_runtime','v3.ai_suggestions','UPDATE,DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.ai_suggestion_commands','UPDATE,DELETE,TRUNCATE')`).Scan(&unsafe)
	if err != nil || unsafe {
		t.Fatal("runtime mutation privilege leaked", err)
	}
}
