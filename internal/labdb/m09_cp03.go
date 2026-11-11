package labdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Additive, synthetic-only installment. Grade history remains immutable; preview
// and release are separate append-only records, never derived from enrollment.
const gradeImportReleaseSchema = `CREATE TABLE v3.grade_import_previews(
 id text PRIMARY KEY,actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 class_id text NOT NULL,grade_item_id text NOT NULL,definition_version_id text NOT NULL,
 publication_revision bigint NOT NULL CHECK(publication_revision>0),input_hash text NOT NULL CHECK(input_hash ~ '^[a-f0-9]{64}$'),
 entries jsonb NOT NULL CHECK(jsonb_typeof(entries)='array' AND jsonb_array_length(entries) BETWEEN 1 AND 50),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL DEFAULT (clock_timestamp()+interval '10 minutes'),
 FOREIGN KEY(grade_item_id,class_id) REFERENCES v3.grade_items(id,class_id),
 FOREIGN KEY(definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 UNIQUE(actor_id,key));
CREATE INDEX grade_import_previews_item_idx ON v3.grade_import_previews(grade_item_id,class_id,expires_at);
CREATE INDEX grade_import_previews_definition_idx ON v3.grade_import_previews(definition_version_id,grade_item_id,class_id);
CREATE TABLE v3.grade_import_confirms(
 preview_id text PRIMARY KEY REFERENCES v3.grade_import_previews(id),actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,
 confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(actor_id,key));
CREATE INDEX grade_import_confirms_actor_idx ON v3.grade_import_confirms(actor_id,confirmed_at);
ALTER TABLE v3.grade_revisions ADD COLUMN source_preview_id text REFERENCES v3.grade_import_previews(id);
CREATE INDEX grade_revisions_preview_idx ON v3.grade_revisions(source_preview_id) WHERE source_preview_id IS NOT NULL;
CREATE TABLE v3.grade_releases(
 id text PRIMARY KEY,class_id text NOT NULL,grade_item_id text NOT NULL,aggregate_id text NOT NULL,
 revision_id text NOT NULL,grade_version bigint NOT NULL CHECK(grade_version>0),learner_user_id text NOT NULL REFERENCES v3.users(id),
 actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 released_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(revision_id,class_id,grade_item_id,aggregate_id) REFERENCES v3.grade_revisions(id,class_id,grade_item_id,aggregate_id),
 UNIQUE(actor_id,key),UNIQUE(grade_item_id,aggregate_id,grade_version));
CREATE INDEX grade_releases_learner_idx ON v3.grade_releases(learner_user_id,class_id,grade_item_id,released_at);
CREATE INDEX grade_releases_revision_idx ON v3.grade_releases(revision_id,class_id,grade_item_id,aggregate_id);
CREATE FUNCTION v3.guard_grade_release_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE actual_version bigint; actual_revision text; actual_learner text; BEGIN
 SELECT c.version,c.revision_id,r.learner_user_id INTO actual_version,actual_revision,actual_learner
 FROM v3.grade_current c JOIN v3.grade_revisions r ON r.id=c.revision_id
 WHERE c.class_id=NEW.class_id AND c.grade_item_id=NEW.grade_item_id AND c.aggregate_id=NEW.aggregate_id;
 IF actual_version IS DISTINCT FROM NEW.grade_version OR actual_revision IS DISTINCT FROM NEW.revision_id OR actual_learner IS DISTINCT FROM NEW.learner_user_id OR NEW.actor_id=NEW.learner_user_id THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_RELEASE_CONTEXT_REJECTED'; END IF;
 RETURN NEW; END$$;
REVOKE ALL ON FUNCTION v3.guard_grade_release_insert() FROM PUBLIC;
CREATE TRIGGER grade_release_insert_guard BEFORE INSERT ON v3.grade_releases FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_release_insert();
CREATE TRIGGER grade_import_previews_immutable BEFORE UPDATE ON v3.grade_import_previews FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();
CREATE TRIGGER grade_import_confirms_immutable BEFORE UPDATE ON v3.grade_import_confirms FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();
CREATE TRIGGER grade_releases_immutable BEFORE UPDATE ON v3.grade_releases FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();`

func GradeImportReleaseChecksum() string { return Digest([]byte(gradeImportReleaseSchema)) }

func InstallGradeImportRelease(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m09-cp03", "pre_m09_cp03.dump", "m09-cp03-1", gradeImportReleaseSchema, "m09-cp02-1", ScopedGradeChecksum())
}
