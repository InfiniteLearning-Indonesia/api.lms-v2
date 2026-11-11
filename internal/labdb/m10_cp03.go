package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A deliberately additive synthetic academic policy. Headers, item provenance,
// credentials and transitions are immutable; a newer release never edits old rows.
const transcriptCredentialSchema = `CREATE TABLE v3.transcript_releases(
 id text PRIMARY KEY,class_id text NOT NULL,enrollment_id text NOT NULL,episode_id text NOT NULL,learner_user_id text NOT NULL,
 version bigint NOT NULL CHECK(version>0),policy_ref text NOT NULL CHECK(policy_ref='SYNTHETIC_M10_CP03_V1'),
 grade_source_hash text NOT NULL CHECK(grade_source_hash ~ '^[a-f0-9]{64}$'),
 released_by text NOT NULL REFERENCES v3.users(id),released_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(enrollment_id,class_id,learner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),
 FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),
 UNIQUE(id,class_id,learner_user_id),UNIQUE(class_id,episode_id,version));
CREATE INDEX transcript_releases_learner_idx ON v3.transcript_releases(class_id,learner_user_id,episode_id,version DESC);
CREATE INDEX transcript_releases_actor_idx ON v3.transcript_releases(released_by,released_at);
CREATE TRIGGER transcript_releases_immutable BEFORE UPDATE ON v3.transcript_releases FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TABLE v3.transcript_grade_items(
 id text PRIMARY KEY,transcript_id text NOT NULL,class_id text NOT NULL,learner_user_id text NOT NULL,
 grade_item_id text NOT NULL,grade_release_id text NOT NULL REFERENCES v3.grade_releases(id),
 grade_revision_id text NOT NULL REFERENCES v3.grade_revisions(id),definition_version_id text NOT NULL REFERENCES v3.grade_definition_versions(id),
 scale_id text NOT NULL REFERENCES v3.grade_scales(id),raw_value_milli bigint NOT NULL,scale_minimum_milli bigint NOT NULL,
 scale_maximum_milli bigint NOT NULL,weight_bps integer NOT NULL CHECK(weight_bps>=1 AND weight_bps<=10000),
 FOREIGN KEY(transcript_id,class_id,learner_user_id) REFERENCES v3.transcript_releases(id,class_id,learner_user_id),
 UNIQUE(transcript_id,grade_item_id,grade_release_id),CHECK(scale_minimum_milli<scale_maximum_milli));
CREATE INDEX transcript_grade_items_release_idx ON v3.transcript_grade_items(grade_release_id,grade_revision_id);
CREATE INDEX transcript_grade_items_definition_idx ON v3.transcript_grade_items(definition_version_id,scale_id);
CREATE INDEX transcript_grade_items_class_idx ON v3.transcript_grade_items(class_id,learner_user_id,grade_item_id);
CREATE TRIGGER transcript_grade_items_immutable BEFORE UPDATE ON v3.transcript_grade_items FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE FUNCTION v3.guard_transcript_item_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE valid boolean;
BEGIN
 SELECT EXISTS(SELECT 1 FROM v3.transcript_releases t
 JOIN v3.grade_releases l ON l.id=NEW.grade_release_id AND l.class_id=t.class_id AND l.learner_user_id=t.learner_user_id
 JOIN v3.grade_revisions r ON r.id=l.revision_id AND r.class_id=l.class_id AND r.grade_item_id=l.grade_item_id AND r.aggregate_id=l.aggregate_id
 JOIN v3.submission_aggregates a ON a.id=r.aggregate_id AND a.class_id=t.class_id AND a.episode_id=t.episode_id
 JOIN v3.grade_definition_versions d ON d.id=r.definition_version_id AND d.class_id=t.class_id AND d.grade_item_id=r.grade_item_id
 JOIN v3.grade_scales s ON s.id=d.scale_id AND s.class_id=t.class_id
 WHERE t.id=NEW.transcript_id AND t.class_id=NEW.class_id AND t.learner_user_id=NEW.learner_user_id
 AND l.grade_item_id=NEW.grade_item_id AND r.id=NEW.grade_revision_id AND d.id=NEW.definition_version_id
 AND s.id=NEW.scale_id AND r.value_milli=NEW.raw_value_milli AND s.minimum_milli=NEW.scale_minimum_milli
 AND s.maximum_milli=NEW.scale_maximum_milli AND d.weight_bps=NEW.weight_bps) INTO valid;
 IF NOT coalesce(valid,false) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='TRANSCRIPT_ITEM_CONTEXT_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_transcript_item_insert() FROM PUBLIC;
CREATE TRIGGER transcript_item_insert_guard BEFORE INSERT ON v3.transcript_grade_items FOR EACH ROW EXECUTE FUNCTION v3.guard_transcript_item_insert();
CREATE TABLE v3.credential_records(
 id text PRIMARY KEY,class_id text NOT NULL,learner_user_id text NOT NULL,transcript_id text NOT NULL,
 privacy text NOT NULL CHECK(privacy IN ('PUBLIC_MINIMAL','PRIVATE')),issued_by text NOT NULL REFERENCES v3.users(id),
 issued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(transcript_id,class_id,learner_user_id) REFERENCES v3.transcript_releases(id,class_id,learner_user_id),
 UNIQUE(transcript_id));
CREATE INDEX credential_records_learner_idx ON v3.credential_records(class_id,learner_user_id,issued_at);
CREATE INDEX credential_records_actor_idx ON v3.credential_records(issued_by,issued_at);
CREATE TRIGGER credential_records_immutable BEFORE UPDATE ON v3.credential_records FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TABLE v3.credential_events(
 id text PRIMARY KEY,credential_id text NOT NULL REFERENCES v3.credential_records(id),action text NOT NULL CHECK(action IN ('ISSUED','REVOKED','SUPERSEDED')),
 version bigint NOT NULL CHECK(version>0),actor_id text NOT NULL REFERENCES v3.users(id),
 reason text,successor_id text REFERENCES v3.credential_records(id),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(credential_id,version),
 CHECK((action='ISSUED' AND version=1 AND reason IS NULL AND successor_id IS NULL)
 OR (action='REVOKED' AND version>1 AND reason IS NOT NULL AND length(reason)>=8 AND length(reason)<=500 AND successor_id IS NULL)
 OR (action='SUPERSEDED' AND version>1 AND reason IS NOT NULL AND length(reason)>=8 AND length(reason)<=500 AND successor_id IS NOT NULL AND successor_id<>credential_id)));
CREATE INDEX credential_events_actor_idx ON v3.credential_events(actor_id,occurred_at);
CREATE INDEX credential_events_successor_idx ON v3.credential_events(successor_id) WHERE successor_id IS NOT NULL;
CREATE TRIGGER credential_events_immutable BEFORE UPDATE ON v3.credential_events FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE FUNCTION v3.guard_credential_event_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE previous_action text; next_version bigint; successor_class text; predecessor_class text;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('credential:'||NEW.credential_id,0));
 SELECT action,version+1 INTO previous_action,next_version FROM v3.credential_events WHERE credential_id=NEW.credential_id ORDER BY version DESC LIMIT 1;
 IF (NEW.action='ISSUED' AND (previous_action IS NOT NULL OR NEW.version<>1))
 OR (NEW.action<>'ISSUED' AND (previous_action IS DISTINCT FROM 'ISSUED' OR NEW.version IS DISTINCT FROM next_version)) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='CREDENTIAL_EVENT_STATE_REJECTED';
 END IF;
 IF NEW.action='SUPERSEDED' THEN
  SELECT class_id INTO successor_class FROM v3.credential_records WHERE id=NEW.successor_id;
  SELECT class_id INTO predecessor_class FROM v3.credential_records WHERE id=NEW.credential_id;
  IF successor_class IS DISTINCT FROM predecessor_class THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='CREDENTIAL_SUCCESSOR_CONTEXT_REJECTED'; END IF;
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_credential_event_insert() FROM PUBLIC;
CREATE TRIGGER credential_event_insert_guard BEFORE INSERT ON v3.credential_events FOR EACH ROW EXECUTE FUNCTION v3.guard_credential_event_insert();
CREATE TABLE v3.credential_commands(
 actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,kind text NOT NULL CHECK(kind IN ('RELEASE_TRANSCRIPT','ISSUE','REVOKE','SUPERSEDE')),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE TRIGGER credential_commands_immutable BEFORE UPDATE ON v3.credential_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();`

func TranscriptCredentialChecksum() string { return Digest([]byte(transcriptCredentialSchema)) }

func InstallTranscriptCredential(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m10-cp03", "pre_m10_cp03.dump", "m10-cp03-1", transcriptCredentialSchema, "m10-cp02-1", CompletionCorrectionChecksum())
}
