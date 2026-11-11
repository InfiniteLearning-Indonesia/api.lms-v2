package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Corrections are append-only. Progress is rebuilt from outcomes and events.
const completionCorrectionSchema = `CREATE TABLE v3.completion_corrections(
 id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,content_revision_id text NOT NULL,rule_version_id text NOT NULL,
 enrollment_id text NOT NULL,episode_id text NOT NULL,learner_user_id text NOT NULL,
 actor_id text NOT NULL REFERENCES v3.users(id),action text NOT NULL CHECK(action IN ('OVERRIDE_COMPLETE','OVERRIDE_INCOMPLETE','REOPEN')),
 state text NOT NULL CHECK(state IN ('COMPLETED','INCOMPLETE')),reason text NOT NULL CHECK(length(reason)>=8 AND length(reason)<=500 AND reason=trim(reason)),
 version bigint NOT NULL CHECK(version>0),base_outcome_count bigint NOT NULL CHECK(base_outcome_count>=0),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(rule_version_id,class_id,activity_id,content_revision_id) REFERENCES v3.completion_rule_versions(id,class_id,activity_id,content_revision_id),
 FOREIGN KEY(enrollment_id,class_id,learner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),
 FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),
 CHECK((action='OVERRIDE_COMPLETE' AND state='COMPLETED') OR (action IN ('OVERRIDE_INCOMPLETE','REOPEN') AND state='INCOMPLETE')),
 UNIQUE(rule_version_id,episode_id,version));
CREATE INDEX completion_corrections_scope_idx ON v3.completion_corrections(rule_version_id,episode_id,version DESC);
CREATE INDEX completion_corrections_class_idx ON v3.completion_corrections(class_id,learner_user_id,activity_id,created_at);
CREATE INDEX completion_corrections_enrollment_idx ON v3.completion_corrections(enrollment_id,episode_id);
CREATE INDEX completion_corrections_actor_idx ON v3.completion_corrections(actor_id,created_at);
CREATE TRIGGER completion_corrections_immutable BEFORE UPDATE ON v3.completion_corrections FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE TABLE v3.completion_correction_commands(
 actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE TRIGGER completion_correction_commands_immutable BEFORE UPDATE ON v3.completion_correction_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_immutable();
CREATE FUNCTION v3.guard_completion_correction_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3,v3identity AS $$
DECLARE outcome_count bigint; correction_count bigint; latest_rule text; permitted boolean; episode_open boolean;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('completion-outcome:'||NEW.rule_version_id||':'||NEW.episode_id,0));
 SELECT count(*) INTO outcome_count FROM v3.activity_completion_outcomes WHERE rule_version_id=NEW.rule_version_id AND episode_id=NEW.episode_id;
 SELECT count(*) INTO correction_count FROM v3.completion_corrections WHERE rule_version_id=NEW.rule_version_id AND episode_id=NEW.episode_id;
 SELECT r.id INTO latest_rule FROM v3.completion_rule_versions r WHERE r.activity_id=NEW.activity_id ORDER BY r.version DESC LIMIT 1;
 SELECT (e.status='ACTIVE' AND ep.ended_at IS NULL AND c.state='PUBLISHED' AND a.state='PUBLISHED') INTO episode_open
 FROM v3.enrollments e JOIN v3.episodes ep ON ep.enrollment_id=e.id
 JOIN v3.classes c ON c.id=e.class_id JOIN v3.content_revisions a ON a.class_id=c.id AND a.activity_id=NEW.activity_id
 WHERE e.id=NEW.enrollment_id AND e.class_id=NEW.class_id AND e.user_id=NEW.learner_user_id AND ep.id=NEW.episode_id AND a.id=NEW.content_revision_id
 AND NOT EXISTS(SELECT 1 FROM v3.class_access x WHERE x.class_id=c.id AND x.closed_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL);
 SELECT (coalesce(sr.admin,false) OR EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles role ON role.enrollment_id=e.id AND role.class_id=e.class_id
 WHERE e.class_id=NEW.class_id AND e.user_id=NEW.actor_id AND e.status='ACTIVE' AND role.role='teacher' AND role.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))) INTO permitted
 FROM v3.users u LEFT JOIN v3identity.links l ON l.user_id=u.id AND l.active AND l.valid_until>clock_timestamp()
 LEFT JOIN v3identity.site_roles sr ON sr.user_id=l.user_id WHERE u.id=NEW.actor_id;
 IF latest_rule IS DISTINCT FROM NEW.rule_version_id OR coalesce(episode_open,false)=false OR coalesce(permitted,false)=false
  OR NEW.base_outcome_count<>outcome_count OR NEW.version<>outcome_count+correction_count+1 THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='COMPLETION_CORRECTION_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_completion_correction_insert() FROM PUBLIC;
CREATE TRIGGER completion_correction_insert_guard BEFORE INSERT ON v3.completion_corrections FOR EACH ROW EXECUTE FUNCTION v3.guard_completion_correction_insert();`

func CompletionCorrectionChecksum() string { return Digest([]byte(completionCorrectionSchema)) }

func InstallCompletionCorrection(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m10-cp02", "pre_m10_cp02.dump", "m10-cp02-1", completionCorrectionSchema, "m10-cp01-1", CompletionFoundationChecksum())
}
