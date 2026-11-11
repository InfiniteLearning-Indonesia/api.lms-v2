package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const historicalGradeReconciliationSchema = `CREATE TABLE v3.legacy_grade_reconciliation_runs(
 id text PRIMARY KEY,actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),source_snapshot_sha256 text NOT NULL CHECK(source_snapshot_sha256 ~ '^[a-f0-9]{64}$'),
 policy_reference text NOT NULL CHECK(length(policy_reference) BETWEEN 3 AND 160),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(actor_id,key));
CREATE INDEX legacy_grade_runs_actor_idx ON v3.legacy_grade_reconciliation_runs(actor_id,created_at,id);
CREATE TABLE v3.legacy_grade_observations(
 id text PRIMARY KEY,run_id text NOT NULL REFERENCES v3.legacy_grade_reconciliation_runs(id),ordinal integer NOT NULL CHECK(ordinal BETWEEN 1 AND 100),
 source_table text NOT NULL CHECK(source_table IN ('submissions','competency_scores','rubrik_assessment_scores','transcript_projection','ai_evaluation','attendance_projection')),
 source_row_id text NOT NULL CHECK(length(source_row_id) BETWEEN 1 AND 160),source_scope text NOT NULL CHECK(source_scope IN ('CLASS','PROGRAM_GLOBAL')),
 raw_state text NOT NULL CHECK(raw_state IN ('MISSING_ROW','EXPLICIT_NULL','VALUE','INVALID')),raw_value_text text CHECK(raw_value_text IS NULL OR length(raw_value_text)<=128),
 raw_score_milli bigint,observed_score_milli bigint,origin text NOT NULL CHECK(origin IN ('RECORDED','COMPUTED','LEGACY_MINIMUM_65','AI_RETURNED','AI_FALLBACK_75')),
 candidate_class_id text,candidate_grade_item_id text,candidate_aggregate_id text,canonical_release_id text REFERENCES v3.grade_releases(id),canonical_score_milli bigint,
 outcome text NOT NULL CHECK(outcome IN ('MATCHED','APPROVED_DIFFERENCE','DIFFERENCE_HOLD','NO_CANONICAL_HOLD','POLICY_DERIVED_HOLD','AI_UNREVIEWED_HOLD','AMBIGUOUS_HOLD')),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(candidate_grade_item_id,candidate_class_id) REFERENCES v3.grade_items(id,class_id),
 FOREIGN KEY(candidate_aggregate_id,candidate_class_id) REFERENCES v3.submission_aggregates(id,class_id),
 UNIQUE(run_id,ordinal),UNIQUE(run_id,source_table,source_row_id),
 CHECK((source_scope='PROGRAM_GLOBAL' AND candidate_class_id IS NULL AND candidate_grade_item_id IS NULL AND candidate_aggregate_id IS NULL) OR
       (source_scope='CLASS' AND candidate_class_id IS NOT NULL AND candidate_grade_item_id IS NOT NULL AND candidate_aggregate_id IS NOT NULL)),
 CHECK((raw_state='MISSING_ROW' AND raw_value_text IS NULL AND raw_score_milli IS NULL) OR
       (raw_state='EXPLICIT_NULL' AND raw_value_text='null' AND raw_score_milli IS NULL) OR
       (raw_state='VALUE' AND raw_value_text IS NOT NULL AND raw_score_milli IS NOT NULL) OR
       (raw_state='INVALID' AND raw_value_text IS NOT NULL AND raw_score_milli IS NULL)),
 CHECK(raw_score_milli IS NULL OR raw_score_milli BETWEEN -1000000000 AND 1000000000),
 CHECK(observed_score_milli IS NULL OR observed_score_milli BETWEEN -1000000000 AND 1000000000),
 CHECK(canonical_score_milli IS NULL OR canonical_score_milli BETWEEN -1000000000 AND 1000000000));
CREATE INDEX legacy_grade_observations_run_idx ON v3.legacy_grade_observations(run_id,ordinal);
CREATE INDEX legacy_grade_observations_candidate_idx ON v3.legacy_grade_observations(candidate_grade_item_id,candidate_class_id,candidate_aggregate_id) WHERE candidate_grade_item_id IS NOT NULL;
CREATE INDEX legacy_grade_observations_aggregate_idx ON v3.legacy_grade_observations(candidate_aggregate_id,candidate_class_id) WHERE candidate_aggregate_id IS NOT NULL;
CREATE INDEX legacy_grade_observations_release_idx ON v3.legacy_grade_observations(canonical_release_id) WHERE canonical_release_id IS NOT NULL;
CREATE FUNCTION v3.guard_legacy_grade_observation_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE latest_release text; latest_score bigint; BEGIN
 IF NEW.source_scope='PROGRAM_GLOBAL' THEN
  IF NEW.outcome<>'AMBIGUOUS_HOLD' OR NEW.canonical_release_id IS NOT NULL OR NEW.canonical_score_milli IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_GLOBAL_AMBIGUOUS'; END IF;
  RETURN NEW;
 END IF;
 SELECT l.id,r.value_milli INTO latest_release,latest_score FROM v3.grade_releases l JOIN v3.grade_revisions r ON r.id=l.revision_id
 WHERE l.class_id=NEW.candidate_class_id AND l.grade_item_id=NEW.candidate_grade_item_id AND l.aggregate_id=NEW.candidate_aggregate_id
 ORDER BY l.grade_version DESC LIMIT 1;
 IF latest_release IS NULL THEN
  IF NEW.canonical_release_id IS NOT NULL OR NEW.canonical_score_milli IS NOT NULL OR NEW.outcome NOT IN ('NO_CANONICAL_HOLD','POLICY_DERIVED_HOLD','AI_UNREVIEWED_HOLD') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_CANONICAL_MISSING'; END IF;
 ELSE
  IF NEW.canonical_release_id IS DISTINCT FROM latest_release OR NEW.canonical_score_milli IS DISTINCT FROM latest_score THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_CANONICAL_STALE'; END IF;
 END IF;
 IF NEW.origin='LEGACY_MINIMUM_65' AND (NEW.observed_score_milli IS DISTINCT FROM 65000 OR NEW.outcome<>'POLICY_DERIVED_HOLD') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_65_HOLD_REQUIRED'; END IF;
 IF NEW.origin='AI_FALLBACK_75' AND (NEW.observed_score_milli IS DISTINCT FROM 75000 OR NEW.outcome<>'AI_UNREVIEWED_HOLD') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_AI_HOLD_REQUIRED'; END IF;
 IF NEW.outcome='MATCHED' AND (NEW.raw_state<>'VALUE' OR NEW.raw_score_milli IS DISTINCT FROM NEW.canonical_score_milli OR NEW.observed_score_milli IS DISTINCT FROM NEW.canonical_score_milli) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_MATCH_REJECTED'; END IF;
 IF NEW.outcome='APPROVED_DIFFERENCE' AND (latest_release IS NULL OR NEW.origin IN ('LEGACY_MINIMUM_65','AI_FALLBACK_75') OR NEW.raw_state<>'VALUE' OR length(NEW.reason)<8) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_GRADE_APPROVAL_REJECTED'; END IF;
 RETURN NEW; END$$;
REVOKE ALL ON FUNCTION v3.guard_legacy_grade_observation_insert() FROM PUBLIC;
CREATE TRIGGER legacy_grade_observation_insert_guard BEFORE INSERT ON v3.legacy_grade_observations FOR EACH ROW EXECUTE FUNCTION v3.guard_legacy_grade_observation_insert();
CREATE TRIGGER legacy_grade_runs_immutable BEFORE UPDATE ON v3.legacy_grade_reconciliation_runs FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();
CREATE TRIGGER legacy_grade_observations_immutable BEFORE UPDATE ON v3.legacy_grade_observations FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();`

func HistoricalGradeReconciliationChecksum() string {
	return Digest([]byte(historicalGradeReconciliationSchema))
}

func InstallHistoricalGradeReconciliation(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m09-cp04", "pre_m09_cp04.dump", "m09-cp04-1", historicalGradeReconciliationSchema, "m09-cp03-1", GradeImportReleaseChecksum())
}
