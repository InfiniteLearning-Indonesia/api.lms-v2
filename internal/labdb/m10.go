package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Additive, synthetic-only M10 foundation. Completion is derived from an
// explicit evidence record and an immutable rule snapshot; grade rows are not
// completion state.
const completionFoundationSchema = `CREATE TABLE v3.completion_rule_versions(
 id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,content_revision_id text NOT NULL,
 version bigint NOT NULL CHECK(version>0),requirement text NOT NULL CHECK(requirement='EVIDENCE_PRESENT'),
 evidence_kind text NOT NULL CHECK(evidence_kind='ACTIVITY_ACKNOWLEDGED'),created_by text NOT NULL REFERENCES v3.users(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(class_id,activity_id,content_revision_id) REFERENCES v3.content_revisions(class_id,activity_id,id),
 UNIQUE(id,class_id,activity_id,content_revision_id),UNIQUE(activity_id,version));
CREATE INDEX completion_rule_versions_class_idx ON v3.completion_rule_versions(class_id,activity_id,version DESC);
CREATE INDEX completion_rule_versions_revision_idx ON v3.completion_rule_versions(content_revision_id,class_id,activity_id);
CREATE INDEX completion_rule_versions_actor_idx ON v3.completion_rule_versions(created_by,created_at);
CREATE TABLE v3.learning_evidence(
 id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,content_revision_id text NOT NULL,rule_version_id text NOT NULL,
 enrollment_id text NOT NULL,episode_id text NOT NULL,learner_user_id text NOT NULL,evidence_kind text NOT NULL CHECK(evidence_kind='ACTIVITY_ACKNOWLEDGED'),
 source_ref text NOT NULL CHECK(source_ref ~ '^command:[a-f0-9]{64}$'),recorded_by text NOT NULL REFERENCES v3.users(id),recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(rule_version_id,class_id,activity_id,content_revision_id) REFERENCES v3.completion_rule_versions(id,class_id,activity_id,content_revision_id),
 FOREIGN KEY(enrollment_id,class_id,learner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),
 FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),
 UNIQUE(id,class_id,activity_id,rule_version_id,learner_user_id),UNIQUE(rule_version_id,episode_id,source_ref));
CREATE INDEX learning_evidence_learner_idx ON v3.learning_evidence(learner_user_id,class_id,activity_id,recorded_at,id);
CREATE INDEX learning_evidence_rule_idx ON v3.learning_evidence(rule_version_id,class_id,activity_id,episode_id);
CREATE INDEX learning_evidence_enrollment_idx ON v3.learning_evidence(enrollment_id,episode_id,recorded_at);
CREATE TABLE v3.activity_completion_outcomes(
 id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,content_revision_id text NOT NULL,rule_version_id text NOT NULL,
 enrollment_id text NOT NULL,episode_id text NOT NULL,learner_user_id text NOT NULL,evidence_id text,
 state text NOT NULL CHECK(state IN ('INCOMPLETE','COMPLETED')),reason text NOT NULL CHECK(reason IN ('EVIDENCE_MISSING','EVIDENCE_SATISFIED')),
 evaluated_by text NOT NULL REFERENCES v3.users(id),evaluated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(rule_version_id,class_id,activity_id,content_revision_id) REFERENCES v3.completion_rule_versions(id,class_id,activity_id,content_revision_id),
 FOREIGN KEY(enrollment_id,class_id,learner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),
 FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),
 FOREIGN KEY(evidence_id,class_id,activity_id,rule_version_id,learner_user_id) REFERENCES v3.learning_evidence(id,class_id,activity_id,rule_version_id,learner_user_id),
 CHECK((state='COMPLETED' AND reason='EVIDENCE_SATISFIED' AND evidence_id IS NOT NULL) OR (state='INCOMPLETE' AND reason='EVIDENCE_MISSING' AND evidence_id IS NULL)));
CREATE INDEX activity_completion_outcomes_learner_idx ON v3.activity_completion_outcomes(learner_user_id,class_id,activity_id,evaluated_at,id);
CREATE INDEX activity_completion_outcomes_rule_idx ON v3.activity_completion_outcomes(rule_version_id,class_id,activity_id,episode_id,evaluated_at);
CREATE INDEX activity_completion_outcomes_evidence_idx ON v3.activity_completion_outcomes(evidence_id,class_id,activity_id) WHERE evidence_id IS NOT NULL;
CREATE INDEX activity_completion_outcomes_enrollment_idx ON v3.activity_completion_outcomes(enrollment_id,episode_id,evaluated_at);
CREATE TABLE v3.completion_commands(
 actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,kind text NOT NULL CHECK(kind IN ('CREATE_RULE','EVALUATE')),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_completion_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='COMPLETION_HISTORY_IMMUTABLE';
END$$;
REVOKE ALL ON FUNCTION v3.guard_completion_immutable() FROM PUBLIC;
CREATE TRIGGER completion_rules_immutable BEFORE UPDATE ON v3.completion_rule_versions FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TRIGGER learning_evidence_immutable BEFORE UPDATE ON v3.learning_evidence FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TRIGGER completion_outcomes_immutable BEFORE UPDATE ON v3.activity_completion_outcomes FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TRIGGER completion_commands_immutable BEFORE UPDATE ON v3.completion_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE FUNCTION v3.guard_completion_rule_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3,v3identity AS $$
DECLARE expected_version bigint; allowed boolean; revision_state text;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('completion-rule:'||NEW.activity_id,0));
 SELECT coalesce(max(version),0)+1 INTO expected_version FROM v3.completion_rule_versions WHERE activity_id=NEW.activity_id;
 SELECT r.state,(c.state='PUBLISHED' AND (coalesce(sr.admin,false) OR EXISTS(
  SELECT 1 FROM v3.enrollments e JOIN v3.roles role ON role.enrollment_id=e.id AND role.class_id=e.class_id
  WHERE e.class_id=NEW.class_id AND e.user_id=NEW.created_by AND e.status='ACTIVE' AND role.role='teacher' AND role.revoked_at IS NULL
  AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))))
 INTO revision_state,allowed FROM v3.content_revisions r JOIN v3.classes c ON c.id=r.class_id
 LEFT JOIN v3identity.links l ON l.user_id=NEW.created_by AND l.active AND l.valid_until>clock_timestamp()
 LEFT JOIN v3identity.site_roles sr ON sr.user_id=l.user_id
 WHERE r.id=NEW.content_revision_id AND r.class_id=NEW.class_id AND r.activity_id=NEW.activity_id;
 IF revision_state IS DISTINCT FROM 'PUBLISHED' OR coalesce(allowed,false)=false OR NEW.version<>expected_version THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='COMPLETION_RULE_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_completion_rule_insert() FROM PUBLIC;
CREATE TRIGGER completion_rule_insert_guard BEFORE INSERT ON v3.completion_rule_versions FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_rule_insert();
CREATE FUNCTION v3.guard_learning_evidence_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE enrollment_state text; episode_closed timestamptz; revision_state text; expected_kind text;
BEGIN
 SELECT status INTO enrollment_state FROM v3.enrollments WHERE id=NEW.enrollment_id AND class_id=NEW.class_id AND user_id=NEW.learner_user_id;
 SELECT ended_at INTO episode_closed FROM v3.episodes WHERE id=NEW.episode_id AND enrollment_id=NEW.enrollment_id;
 SELECT r.state,rule.evidence_kind INTO revision_state,expected_kind FROM v3.content_revisions r
 JOIN v3.completion_rule_versions rule ON rule.content_revision_id=r.id AND rule.class_id=r.class_id AND rule.activity_id=r.activity_id
 WHERE rule.id=NEW.rule_version_id AND r.id=NEW.content_revision_id AND r.class_id=NEW.class_id AND r.activity_id=NEW.activity_id;
 IF enrollment_state IS DISTINCT FROM 'ACTIVE' OR episode_closed IS NOT NULL OR revision_state IS DISTINCT FROM 'PUBLISHED'
  OR expected_kind IS DISTINCT FROM NEW.evidence_kind OR NEW.recorded_by IS DISTINCT FROM NEW.learner_user_id THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEARNING_EVIDENCE_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_learning_evidence_insert() FROM PUBLIC;
CREATE TRIGGER learning_evidence_insert_guard BEFORE INSERT ON v3.learning_evidence FOR EACH ROW EXECUTE FUNCTION v3.guard_learning_evidence_insert();
CREATE FUNCTION v3.guard_completion_outcome_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE evidence_ok boolean;
BEGIN
 IF NEW.evaluated_by IS DISTINCT FROM NEW.learner_user_id THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='COMPLETION_OUTCOME_CONTEXT_REJECTED';
 END IF;
 IF NEW.state='COMPLETED' THEN
  SELECT EXISTS(SELECT 1 FROM v3.learning_evidence e WHERE e.id=NEW.evidence_id AND e.class_id=NEW.class_id
   AND e.activity_id=NEW.activity_id AND e.content_revision_id=NEW.content_revision_id AND e.rule_version_id=NEW.rule_version_id
   AND e.enrollment_id=NEW.enrollment_id AND e.episode_id=NEW.episode_id AND e.learner_user_id=NEW.learner_user_id) INTO evidence_ok;
  IF NOT coalesce(evidence_ok,false) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='COMPLETION_OUTCOME_CONTEXT_REJECTED'; END IF;
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_completion_outcome_insert() FROM PUBLIC;
CREATE TRIGGER completion_outcome_insert_guard BEFORE INSERT ON v3.activity_completion_outcomes FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_outcome_insert();`

func CompletionFoundationChecksum() string { return Digest([]byte(completionFoundationSchema)) }

func InstallCompletionFoundation(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m10-cp01", "pre_m10_cp01.dump", "m10-cp01-1", completionFoundationSchema, "m09-cp04-1", HistoricalGradeReconciliationChecksum())
}
