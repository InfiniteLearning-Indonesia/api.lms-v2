package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Historical output is evidence of legacy UI/state, not canonical completion or
// issuance. This synthetic landing is deliberately separate from credential rows.
const legacyCompletionObservationSchema = `CREATE TABLE v3.legacy_completion_runs(
 id text PRIMARY KEY,actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 source_instance_id text NOT NULL CHECK(length(source_instance_id) BETWEEN 1 AND 128),
 source_snapshot_sha256 text NOT NULL CHECK(source_snapshot_sha256 ~ '^[a-f0-9]{64}$'),
 response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(actor_id,key));
CREATE INDEX legacy_completion_runs_source_idx ON v3.legacy_completion_runs(source_instance_id,created_at,id);
CREATE TRIGGER legacy_completion_runs_immutable BEFORE UPDATE ON v3.legacy_completion_runs FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TABLE v3.legacy_completion_observations(
 id text PRIMARY KEY,run_id text NOT NULL REFERENCES v3.legacy_completion_runs(id),ordinal integer NOT NULL CHECK(ordinal BETWEEN 1 AND 100),
 source_instance_id text NOT NULL CHECK(length(source_instance_id) BETWEEN 1 AND 128),
 source_table text NOT NULL CHECK(source_table IN ('completion_projection','transcript_projection','certificate_projection')),
 source_pk text NOT NULL CHECK(length(source_pk) BETWEEN 1 AND 160),source_version text NOT NULL CHECK(length(source_version) BETWEEN 1 AND 128),
 raw_payload jsonb NOT NULL,raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),
 observed_state text NOT NULL CHECK(observed_state IN ('TRUE','FALSE','UNKNOWN','INVALID')),
 disposition text NOT NULL CHECK(disposition IN ('OBSERVED_UNVERIFIED','UNKNOWN_HOLD')),
 candidate_class_id text REFERENCES v3.classes(id),candidate_learner_user_id text REFERENCES v3.users(id),
 observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,ordinal),UNIQUE(source_instance_id,source_table,source_pk,source_version),
 CHECK((candidate_class_id IS NULL AND candidate_learner_user_id IS NULL) OR
       (candidate_class_id IS NOT NULL AND candidate_learner_user_id IS NOT NULL)),
 CHECK((observed_state IN ('TRUE','FALSE') AND disposition='OBSERVED_UNVERIFIED') OR
       (observed_state IN ('UNKNOWN','INVALID') AND disposition='UNKNOWN_HOLD')));
CREATE INDEX legacy_completion_observations_run_idx ON v3.legacy_completion_observations(run_id,ordinal);
CREATE INDEX legacy_completion_observations_candidate_idx ON v3.legacy_completion_observations(candidate_class_id,candidate_learner_user_id,observed_at) WHERE candidate_class_id IS NOT NULL;
CREATE TRIGGER legacy_completion_observations_immutable BEFORE UPDATE ON v3.legacy_completion_observations FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE FUNCTION v3.guard_legacy_completion_candidate() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
BEGIN
 IF NEW.candidate_class_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM v3.enrollments e WHERE e.class_id=NEW.candidate_class_id AND e.user_id=NEW.candidate_learner_user_id
 ) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_COMPLETION_CANDIDATE_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_legacy_completion_candidate() FROM PUBLIC;
CREATE TRIGGER legacy_completion_candidate_guard BEFORE INSERT ON v3.legacy_completion_observations FOR EACH ROW EXECUTE FUNCTION v3.guard_legacy_completion_candidate();`

func LegacyCompletionObservationChecksum() string {
	return Digest([]byte(legacyCompletionObservationSchema))
}

func InstallLegacyCompletionObservation(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m10-cp04", "pre_m10_cp04.dump", "m10-cp04-1", legacyCompletionObservationSchema, "m10-cp03-1", TranscriptCredentialChecksum())
}
