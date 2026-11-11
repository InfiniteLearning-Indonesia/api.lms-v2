package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Additive synthetic installment. Existing M13/M09/M10 checksums are immutable.
const aiSuggestionSchema = `CREATE TABLE v3.ai_suggestions(
 id text PRIMARY KEY CHECK(id ~ '^[a-f0-9]{32}$'),
 class_id text NOT NULL,grade_item_id text NOT NULL,aggregate_id text NOT NULL,submission_revision_id text NOT NULL,
 definition_version_id text NOT NULL,requested_by text NOT NULL REFERENCES v3.users(id),
 model_key text NOT NULL CHECK(model_key ~ '^[a-z][a-z0-9._-]{0,63}$'),
 model_version text NOT NULL CHECK(model_version ~ '^[A-Za-z0-9._-]{1,64}$'),
 prompt_sha256 text NOT NULL CHECK(prompt_sha256 ~ '^[a-f0-9]{64}$'),
 rubric_sha256 text NOT NULL CHECK(rubric_sha256 ~ '^[a-f0-9]{64}$'),
 submission_sha256 text NOT NULL CHECK(submission_sha256 ~ '^[a-f0-9]{64}$'),
 status text NOT NULL CHECK(status IN ('SUCCEEDED','FAILED')),
 proposed_value_milli bigint,
 rationale text CHECK(rationale IS NULL OR length(rationale) BETWEEN 1 AND 2000),
 failure_code text CHECK(failure_code IS NULL OR failure_code ~ '^[A-Z_]{1,64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(grade_item_id,class_id) REFERENCES v3.grade_items(id,class_id),
 FOREIGN KEY(aggregate_id,class_id) REFERENCES v3.submission_aggregates(id,class_id),
 FOREIGN KEY(submission_revision_id,class_id) REFERENCES v3.submission_revisions(id,class_id),
 FOREIGN KEY(definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 CHECK((status='SUCCEEDED' AND proposed_value_milli IS NOT NULL AND failure_code IS NULL AND rationale IS NOT NULL) OR
       (status='FAILED' AND proposed_value_milli IS NULL AND rationale IS NULL AND failure_code IS NOT NULL)));
CREATE INDEX ai_suggestions_context_idx ON v3.ai_suggestions(class_id,grade_item_id,aggregate_id,created_at,id);
CREATE INDEX ai_suggestions_revision_idx ON v3.ai_suggestions(submission_revision_id,class_id);
CREATE INDEX ai_suggestions_definition_idx ON v3.ai_suggestions(definition_version_id,grade_item_id,class_id);
CREATE INDEX ai_suggestions_actor_idx ON v3.ai_suggestions(requested_by,created_at,id);
CREATE TABLE v3.ai_suggestion_commands(
 actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 suggestion_id text NOT NULL REFERENCES v3.ai_suggestions(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE INDEX ai_suggestion_commands_suggestion_idx ON v3.ai_suggestion_commands(suggestion_id);
CREATE FUNCTION v3.guard_ai_suggestion_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='AI_SUGGESTION_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_ai_suggestion_immutable() FROM PUBLIC;
CREATE TRIGGER ai_suggestion_immutable BEFORE UPDATE ON v3.ai_suggestions FOR EACH ROW EXECUTE FUNCTION v3.guard_ai_suggestion_immutable();
CREATE TRIGGER ai_suggestion_command_immutable BEFORE UPDATE ON v3.ai_suggestion_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_ai_suggestion_immutable();
CREATE FUNCTION v3.guard_ai_suggestion_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE valid boolean; BEGIN
 SELECT EXISTS(SELECT 1 FROM v3.grade_items gi
  JOIN v3.classes c ON c.id=gi.class_id AND c.state='PUBLISHED'
  JOIN v3.submission_aggregates a ON a.class_id=gi.class_id AND a.activity_id=gi.activity_id
  JOIN v3.submission_revisions r ON r.aggregate_id=a.id AND r.class_id=a.class_id
  JOIN v3.submission_receipts receipt ON receipt.revision_id=r.id AND receipt.aggregate_id=a.id AND receipt.class_id=a.class_id
  JOIN v3.grade_definition_publications p ON p.grade_item_id=gi.id AND p.definition_version_id=NEW.definition_version_id
   AND p.revision=(SELECT max(x.revision) FROM v3.grade_definition_publications x WHERE x.grade_item_id=gi.id)
  JOIN v3.grade_definition_versions d ON d.id=p.definition_version_id AND d.grade_item_id=gi.id AND d.class_id=gi.class_id
  JOIN v3.grade_scales sc ON sc.id=d.scale_id AND sc.class_id=d.class_id
  LEFT JOIN v3identity.links l ON l.user_id=NEW.requested_by AND l.active AND l.valid_until>clock_timestamp()
  LEFT JOIN v3identity.site_roles sr ON sr.user_id=l.user_id
  WHERE gi.id=NEW.grade_item_id AND gi.class_id=NEW.class_id AND a.id=NEW.aggregate_id
   AND r.id=NEW.submission_revision_id AND r.body_sha256=NEW.submission_sha256
   AND a.owner_user_id<>NEW.requested_by
   AND (NEW.status='FAILED' OR NEW.proposed_value_milli BETWEEN sc.minimum_milli AND sc.maximum_milli)
   AND (coalesce(sr.admin,false) OR EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles role ON role.enrollment_id=e.id AND role.class_id=e.class_id
      WHERE e.class_id=gi.class_id AND e.user_id=NEW.requested_by AND e.status='ACTIVE' AND role.role='teacher' AND role.revoked_at IS NULL
      AND NOT EXISTS(SELECT 1 FROM v3.class_access x WHERE x.class_id=e.class_id AND x.closed_at IS NOT NULL)
      AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)))) INTO valid;
 IF NOT valid THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='AI_SUGGESTION_CONTEXT_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_ai_suggestion_insert() FROM PUBLIC;
CREATE TRIGGER ai_suggestion_insert_guard BEFORE INSERT ON v3.ai_suggestions FOR EACH ROW EXECUTE FUNCTION v3.guard_ai_suggestion_insert();
ALTER TABLE v3.grade_revisions ADD COLUMN ai_suggestion_id text REFERENCES v3.ai_suggestions(id);
CREATE INDEX grade_revisions_ai_suggestion_idx ON v3.grade_revisions(ai_suggestion_id) WHERE ai_suggestion_id IS NOT NULL;
CREATE FUNCTION v3.guard_ai_grade_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE valid boolean; BEGIN
 IF NEW.ai_suggestion_id IS NULL THEN RETURN NEW; END IF;
 SELECT EXISTS(SELECT 1 FROM v3.ai_suggestions s WHERE s.id=NEW.ai_suggestion_id AND s.status='SUCCEEDED'
  AND s.class_id=NEW.class_id AND s.grade_item_id=NEW.grade_item_id AND s.aggregate_id=NEW.aggregate_id
  AND s.submission_revision_id=NEW.submission_revision_id AND s.definition_version_id=NEW.definition_version_id) INTO valid;
 IF NOT valid THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='AI_GRADE_SOURCE_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_ai_grade_source() FROM PUBLIC;
CREATE TRIGGER ai_grade_source_guard BEFORE INSERT ON v3.grade_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_ai_grade_source();`

func AISuggestionChecksum() string { return Digest([]byte(aiSuggestionSchema)) }

func InstallAISuggestions(ctx context.Context, pool *pgxpool.Pool) error {
	return installM13(ctx, pool, "m13-cp03", "pre_m13_cp03.dump", "m13-cp03-1", aiSuggestionSchema, "m10-cp04-1", LegacyCompletionObservationChecksum())
}
