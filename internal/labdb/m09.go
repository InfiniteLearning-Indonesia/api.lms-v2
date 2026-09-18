package labdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
)

const gradeDefinitionSchema = `CREATE TABLE v3.grade_scales(
 id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 minimum_milli bigint NOT NULL,maximum_milli bigint NOT NULL,display_decimals smallint NOT NULL CHECK(display_decimals BETWEEN 0 AND 3),
 created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(id,class_id),CHECK(minimum_milli<maximum_milli),CHECK(minimum_milli BETWEEN -1000000000 AND 1000000000),CHECK(maximum_milli BETWEEN -1000000000 AND 1000000000));
CREATE INDEX grade_scales_class_idx ON v3.grade_scales(class_id,created_at,id);
CREATE INDEX grade_scales_actor_idx ON v3.grade_scales(created_by,created_at);
CREATE TABLE v3.grade_items(
 id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),
 created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(class_id,activity_id) REFERENCES v3.content_activities(class_id,id),UNIQUE(id,class_id),UNIQUE(class_id,activity_id));
CREATE INDEX grade_items_actor_idx ON v3.grade_items(created_by,created_at);
CREATE TABLE v3.grade_definition_versions(
 id text PRIMARY KEY,grade_item_id text NOT NULL,class_id text NOT NULL,version integer NOT NULL CHECK(version>0),scale_id text NOT NULL,
 weight_bps integer NOT NULL CHECK(weight_bps BETWEEN 1 AND 10000),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(grade_item_id,class_id) REFERENCES v3.grade_items(id,class_id),FOREIGN KEY(scale_id,class_id) REFERENCES v3.grade_scales(id,class_id),
 UNIQUE(id,grade_item_id,class_id),UNIQUE(grade_item_id,version));
CREATE INDEX grade_definition_versions_scale_idx ON v3.grade_definition_versions(scale_id,class_id);
CREATE INDEX grade_definition_versions_actor_idx ON v3.grade_definition_versions(created_by,created_at);
CREATE TABLE v3.rubric_criteria(
 id text PRIMARY KEY,definition_version_id text NOT NULL,grade_item_id text NOT NULL,class_id text NOT NULL,criterion_key text NOT NULL CHECK(criterion_key ~ '^[a-z][a-z0-9_-]{0,63}$'),
 title text NOT NULL CHECK(length(title) BETWEEN 1 AND 240),parent_criterion_id text,weight_bps integer NOT NULL CHECK(weight_bps BETWEEN 1 AND 10000),
 maximum_milli bigint NOT NULL CHECK(maximum_milli>0 AND maximum_milli<=1000000000),position integer NOT NULL CHECK(position BETWEEN 1 AND 1000),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 UNIQUE(id,definition_version_id),UNIQUE(definition_version_id,criterion_key),UNIQUE(definition_version_id,position),
 FOREIGN KEY(parent_criterion_id,definition_version_id) REFERENCES v3.rubric_criteria(id,definition_version_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(parent_criterion_id IS NULL OR parent_criterion_id<>id));
CREATE INDEX rubric_criteria_definition_idx ON v3.rubric_criteria(definition_version_id,position,id);
CREATE INDEX rubric_criteria_parent_idx ON v3.rubric_criteria(parent_criterion_id,definition_version_id) WHERE parent_criterion_id IS NOT NULL;
CREATE TABLE v3.grade_definition_publications(
 id text PRIMARY KEY,grade_item_id text NOT NULL,class_id text NOT NULL,definition_version_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 previous_definition_version_id text,actor_id text NOT NULL REFERENCES v3.users(id),published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(grade_item_id,class_id) REFERENCES v3.grade_items(id,class_id),
 FOREIGN KEY(definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 FOREIGN KEY(previous_definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 UNIQUE(grade_item_id,revision),UNIQUE(definition_version_id));
CREATE INDEX grade_definition_publications_current_idx ON v3.grade_definition_publications(grade_item_id,revision DESC);
CREATE INDEX grade_definition_publications_class_idx ON v3.grade_definition_publications(class_id,published_at,grade_item_id);
CREATE INDEX grade_definition_publications_actor_idx ON v3.grade_definition_publications(actor_id,published_at);
CREATE INDEX grade_definition_publications_previous_idx ON v3.grade_definition_publications(previous_definition_version_id,grade_item_id) WHERE previous_definition_version_id IS NOT NULL;
CREATE TABLE v3.grade_definition_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_grade_definition_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_DEFINITION_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_grade_definition_immutable() FROM PUBLIC;
CREATE TRIGGER grade_scales_immutable BEFORE UPDATE ON v3.grade_scales FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE TRIGGER grade_items_immutable BEFORE UPDATE ON v3.grade_items FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE TRIGGER grade_definition_versions_immutable BEFORE UPDATE ON v3.grade_definition_versions FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE TRIGGER rubric_criteria_immutable BEFORE UPDATE ON v3.rubric_criteria FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE TRIGGER grade_definition_publications_immutable BEFORE UPDATE ON v3.grade_definition_publications FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE TRIGGER grade_definition_commands_immutable BEFORE UPDATE ON v3.grade_definition_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_definition_immutable();
CREATE FUNCTION v3.guard_grade_item_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3,v3identity AS $$ DECLARE allowed boolean; BEGIN
 SELECT (c.state='PUBLISHED' AND a.activity_type='ASSIGNMENT' AND (coalesce(sr.admin,false) OR EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id WHERE e.class_id=NEW.class_id AND e.user_id=NEW.created_by AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)))) INTO allowed
 FROM v3.classes c JOIN v3.content_activities a ON a.class_id=c.id AND a.id=NEW.activity_id LEFT JOIN v3identity.links l ON l.user_id=NEW.created_by AND l.active AND l.valid_until>clock_timestamp() LEFT JOIN v3identity.site_roles sr ON sr.user_id=l.user_id WHERE c.id=NEW.class_id;
 IF coalesce(allowed,false)=false THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_ITEM_CONTEXT_REJECTED'; END IF; RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_grade_item_insert() FROM PUBLIC;
CREATE TRIGGER grade_item_insert_guard BEFORE INSERT ON v3.grade_items FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_item_insert();
CREATE FUNCTION v3.guard_rubric_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM v3.grade_definition_publications WHERE definition_version_id=NEW.definition_version_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PUBLISHED_RUBRIC_IMMUTABLE'; END IF; RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_rubric_insert() FROM PUBLIC;
CREATE TRIGGER rubric_insert_guard BEFORE INSERT ON v3.rubric_criteria FOR EACH ROW EXECUTE FUNCTION v3.guard_rubric_insert();
CREATE FUNCTION v3.guard_grade_publication_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE last_revision bigint; last_definition text; criterion_count integer; leaf_weight bigint; cycle_found boolean; BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.grade_item_id,0));
 PERFORM 1 FROM v3.grade_items WHERE id=NEW.grade_item_id AND class_id=NEW.class_id;
 SELECT revision,definition_version_id INTO last_revision,last_definition FROM v3.grade_definition_publications WHERE grade_item_id=NEW.grade_item_id ORDER BY revision DESC LIMIT 1;
 SELECT count(*) INTO criterion_count FROM v3.rubric_criteria WHERE definition_version_id=NEW.definition_version_id;
 SELECT coalesce(sum(c.weight_bps),0) INTO leaf_weight FROM v3.rubric_criteria c WHERE c.definition_version_id=NEW.definition_version_id AND NOT EXISTS(SELECT 1 FROM v3.rubric_criteria child WHERE child.definition_version_id=c.definition_version_id AND child.parent_criterion_id=c.id);
 WITH RECURSIVE walk(id,parent_criterion_id,path,cycle) AS (
  SELECT id,parent_criterion_id,ARRAY[id],false FROM v3.rubric_criteria WHERE definition_version_id=NEW.definition_version_id
  UNION ALL SELECT p.id,p.parent_criterion_id,w.path||p.id,p.id=ANY(w.path) FROM walk w JOIN v3.rubric_criteria p ON p.id=w.parent_criterion_id AND p.definition_version_id=NEW.definition_version_id WHERE NOT w.cycle
 ) SELECT coalesce(bool_or(cycle),false) INTO cycle_found FROM walk;
 IF NEW.revision IS DISTINCT FROM coalesce(last_revision,0)+1 OR NEW.previous_definition_version_id IS DISTINCT FROM last_definition OR criterion_count<1 OR leaf_weight<>10000 OR cycle_found THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_PUBLICATION_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_grade_publication_insert() FROM PUBLIC;
CREATE TRIGGER grade_publication_insert_guard BEFORE INSERT ON v3.grade_definition_publications FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_publication_insert();`

const scopedGradeSchema = `CREATE TABLE v3.grade_revisions(
 id text PRIMARY KEY,class_id text NOT NULL,grade_item_id text NOT NULL,definition_version_id text NOT NULL,aggregate_id text NOT NULL,attempt_id text NOT NULL,submission_revision_id text NOT NULL,
 learner_user_id text NOT NULL REFERENCES v3.users(id),grader_user_id text NOT NULL REFERENCES v3.users(id),version bigint NOT NULL CHECK(version>0),value_milli bigint NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(grade_item_id,class_id) REFERENCES v3.grade_items(id,class_id),FOREIGN KEY(definition_version_id,grade_item_id,class_id) REFERENCES v3.grade_definition_versions(id,grade_item_id,class_id),
 FOREIGN KEY(attempt_id,class_id,aggregate_id) REFERENCES v3.submission_attempts(id,class_id,aggregate_id),FOREIGN KEY(submission_revision_id,class_id,attempt_id) REFERENCES v3.submission_revisions(id,class_id,attempt_id),
 UNIQUE(id,class_id,grade_item_id,aggregate_id),UNIQUE(grade_item_id,aggregate_id,version));
CREATE INDEX grade_revisions_learner_idx ON v3.grade_revisions(learner_user_id,class_id,created_at,id);
CREATE INDEX grade_revisions_grader_idx ON v3.grade_revisions(grader_user_id,created_at,id);
CREATE INDEX grade_revisions_submission_idx ON v3.grade_revisions(submission_revision_id,class_id,attempt_id);
CREATE INDEX grade_revisions_definition_idx ON v3.grade_revisions(definition_version_id,grade_item_id,class_id);
CREATE TABLE v3.grade_current(
 grade_item_id text NOT NULL,aggregate_id text NOT NULL,class_id text NOT NULL,revision_id text NOT NULL,version bigint NOT NULL CHECK(version>0),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(grade_item_id,aggregate_id),FOREIGN KEY(revision_id,class_id,grade_item_id,aggregate_id) REFERENCES v3.grade_revisions(id,class_id,grade_item_id,aggregate_id));
CREATE INDEX grade_current_class_idx ON v3.grade_current(class_id,grade_item_id,aggregate_id);
CREATE INDEX grade_current_revision_idx ON v3.grade_current(revision_id,class_id,grade_item_id,aggregate_id);
CREATE TABLE v3.grade_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_grade_revision_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_HISTORY_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_grade_revision_immutable() FROM PUBLIC;
CREATE TRIGGER grade_revisions_immutable BEFORE UPDATE ON v3.grade_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();
CREATE TRIGGER grade_commands_immutable BEFORE UPDATE ON v3.grade_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_immutable();
CREATE FUNCTION v3.guard_grade_revision_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE activity text; aggregate_activity text; owner_id text; receipt_revision text; active_definition text; minimum_value bigint; maximum_value bigint; BEGIN
 SELECT gi.activity_id INTO activity FROM v3.grade_items gi WHERE gi.id=NEW.grade_item_id AND gi.class_id=NEW.class_id;
 SELECT s.activity_id,s.owner_user_id INTO aggregate_activity,owner_id FROM v3.submission_aggregates s WHERE s.id=NEW.aggregate_id AND s.class_id=NEW.class_id;
 SELECT r.revision_id INTO receipt_revision FROM v3.submission_receipts r WHERE r.aggregate_id=NEW.aggregate_id AND r.attempt_id=NEW.attempt_id;
 SELECT p.definition_version_id,sc.minimum_milli,sc.maximum_milli INTO active_definition,minimum_value,maximum_value FROM v3.grade_definition_publications p JOIN v3.grade_definition_versions d ON d.id=p.definition_version_id AND d.grade_item_id=p.grade_item_id AND d.class_id=p.class_id JOIN v3.grade_scales sc ON sc.id=d.scale_id AND sc.class_id=d.class_id WHERE p.grade_item_id=NEW.grade_item_id ORDER BY p.revision DESC LIMIT 1;
 IF activity IS DISTINCT FROM aggregate_activity OR owner_id IS DISTINCT FROM NEW.learner_user_id OR receipt_revision IS DISTINCT FROM NEW.submission_revision_id OR active_definition IS DISTINCT FROM NEW.definition_version_id OR NEW.grader_user_id IS NOT DISTINCT FROM NEW.learner_user_id OR NEW.value_milli<minimum_value OR NEW.value_milli>maximum_value THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_CONTEXT_REJECTED'; END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_grade_revision_insert() FROM PUBLIC;
CREATE TRIGGER grade_revision_insert_guard BEFORE INSERT ON v3.grade_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_revision_insert();
CREATE FUNCTION v3.guard_grade_current_write() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE history_version bigint; BEGIN
 SELECT version INTO history_version FROM v3.grade_revisions WHERE id=NEW.revision_id AND class_id=NEW.class_id AND grade_item_id=NEW.grade_item_id AND aggregate_id=NEW.aggregate_id;
 IF history_version IS DISTINCT FROM NEW.version OR (TG_OP='UPDATE' AND (NEW.grade_item_id IS DISTINCT FROM OLD.grade_item_id OR NEW.aggregate_id IS DISTINCT FROM OLD.aggregate_id OR NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='GRADE_CURRENT_REJECTED'; END IF; RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_grade_current_write() FROM PUBLIC;
CREATE TRIGGER grade_current_write_guard BEFORE INSERT OR UPDATE ON v3.grade_current FOR EACH ROW EXECUTE FUNCTION v3.guard_grade_current_write();`

func GradeDefinitionChecksum() string { return Digest([]byte(gradeDefinitionSchema)) }
func ScopedGradeChecksum() string     { return Digest([]byte(scopedGradeSchema)) }

func installGradeSlice(ctx context.Context, pool *pgxpool.Pool, checkpoint, archive, version, ddl, dependency, dependencyChecksum string) error {
	if pool == nil || VerifyInstallmentProof(checkpoint, archive) != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810909)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var checksum string
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", dependency).Scan(&checksum) != nil || checksum != dependencyChecksum {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", version).Scan(&checksum)
		if err == nil {
			if checksum != Digest([]byte(ddl)) {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, ddl, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", version, Digest([]byte(ddl)), proofArchiveHash(checkpoint))
		return err
	})
}

func InstallGradeDefinitions(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m09-cp01", "pre_m09_cp01.dump", "m09-cp01-1", gradeDefinitionSchema, "m13-cp02-1", NotificationSafetyChecksum())
}

func InstallScopedGrades(ctx context.Context, pool *pgxpool.Pool) error {
	return installGradeSlice(ctx, pool, "m09-cp02", "pre_m09_cp02.dump", "m09-cp02-1", scopedGradeSchema, "m09-cp01-1", GradeDefinitionChecksum())
}
