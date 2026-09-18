package labdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
)

const logbookPeriodSchema = `CREATE TABLE v3.logbook_period_sets(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 64),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(id,class_id));
CREATE INDEX logbook_period_sets_class_idx ON v3.logbook_period_sets(class_id,created_at,id);
CREATE INDEX logbook_period_sets_actor_idx ON v3.logbook_period_sets(created_by,created_at);
CREATE TABLE v3.logbook_periods(id text PRIMARY KEY,set_id text NOT NULL,class_id text NOT NULL,ordinal integer NOT NULL CHECK(ordinal BETWEEN 1 AND 24),name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),starts_at timestamptz NOT NULL,ends_at timestamptz NOT NULL,due_at timestamptz NOT NULL,FOREIGN KEY(set_id,class_id) REFERENCES v3.logbook_period_sets(id,class_id),UNIQUE(set_id,ordinal),CHECK(starts_at<ends_at AND ends_at<=due_at));
CREATE INDEX logbook_periods_class_idx ON v3.logbook_periods(class_id,set_id,ordinal);
CREATE INDEX logbook_periods_set_fk_idx ON v3.logbook_periods(set_id,class_id,ordinal);
CREATE TABLE v3.logbook_period_publications(id text PRIMARY KEY,class_id text NOT NULL,set_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),previous_set_id text,actor_id text NOT NULL REFERENCES v3.users(id),published_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(set_id,class_id) REFERENCES v3.logbook_period_sets(id,class_id),FOREIGN KEY(previous_set_id,class_id) REFERENCES v3.logbook_period_sets(id,class_id),UNIQUE(class_id,revision),UNIQUE(set_id));
CREATE INDEX logbook_period_publications_class_idx ON v3.logbook_period_publications(class_id,revision DESC);
CREATE INDEX logbook_period_publications_actor_idx ON v3.logbook_period_publications(actor_id,published_at);
CREATE INDEX logbook_period_publications_previous_fk_idx ON v3.logbook_period_publications(previous_set_id,class_id) WHERE previous_set_id IS NOT NULL;
CREATE TABLE v3.logbook_period_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_logbook_period_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_PERIOD_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_logbook_period_immutable() FROM PUBLIC;
CREATE TRIGGER logbook_period_sets_immutable BEFORE UPDATE ON v3.logbook_period_sets FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_period_immutable();
CREATE TRIGGER logbook_periods_immutable BEFORE UPDATE ON v3.logbook_periods FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_period_immutable();
CREATE TRIGGER logbook_period_publications_immutable BEFORE UPDATE ON v3.logbook_period_publications FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_period_immutable();
CREATE TRIGGER logbook_period_commands_immutable BEFORE UPDATE ON v3.logbook_period_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_period_immutable();
CREATE FUNCTION v3.guard_logbook_period_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ BEGIN
 PERFORM 1 FROM v3.logbook_period_sets WHERE id=NEW.set_id AND class_id=NEW.class_id FOR UPDATE;
 IF EXISTS(SELECT 1 FROM v3.logbook_period_publications WHERE set_id=NEW.set_id) OR (SELECT count(*) FROM v3.logbook_periods WHERE set_id=NEW.set_id)>=24 OR EXISTS(SELECT 1 FROM v3.logbook_periods p WHERE p.set_id=NEW.set_id AND p.starts_at<NEW.ends_at AND p.ends_at>NEW.starts_at) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_PERIOD_INSERT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_period_insert() FROM PUBLIC;
CREATE TRIGGER logbook_period_insert_guard BEFORE INSERT ON v3.logbook_periods FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_period_insert();
CREATE FUNCTION v3.guard_logbook_publication_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE last_revision bigint; last_set text; period_count integer; last_ordinal integer; BEGIN
 PERFORM 1 FROM v3.classes WHERE id=NEW.class_id FOR UPDATE;
 SELECT revision,set_id INTO last_revision,last_set FROM v3.logbook_period_publications WHERE class_id=NEW.class_id ORDER BY revision DESC LIMIT 1;
 SELECT count(*),max(ordinal) INTO period_count,last_ordinal FROM v3.logbook_periods WHERE set_id=NEW.set_id AND class_id=NEW.class_id;
 IF NEW.revision IS DISTINCT FROM COALESCE(last_revision,0)+1 OR NEW.previous_set_id IS DISTINCT FROM last_set OR period_count<1 OR period_count IS DISTINCT FROM last_ordinal THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_PUBLICATION_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_publication_insert() FROM PUBLIC;
CREATE TRIGGER logbook_publication_insert_guard BEFORE INSERT ON v3.logbook_period_publications FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_publication_insert();`

const logbookEntrySchema = `ALTER TABLE v3.logbook_periods ADD CONSTRAINT logbook_periods_class_identity_uniq UNIQUE(id,class_id);
CREATE TABLE v3.logbook_entries(id text PRIMARY KEY,class_id text NOT NULL,period_id text NOT NULL,enrollment_id text NOT NULL,episode_id text NOT NULL,owner_user_id text NOT NULL,created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(period_id,class_id) REFERENCES v3.logbook_periods(id,class_id),FOREIGN KEY(enrollment_id,class_id,owner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),UNIQUE(id,class_id),UNIQUE(period_id,episode_id));
CREATE INDEX logbook_entries_owner_idx ON v3.logbook_entries(owner_user_id,class_id,created_at,id);
CREATE INDEX logbook_entries_period_idx ON v3.logbook_entries(period_id,class_id,episode_id);
CREATE INDEX logbook_entries_enrollment_idx ON v3.logbook_entries(enrollment_id,class_id,episode_id);
CREATE TABLE v3.logbook_entry_revisions(id text PRIMARY KEY,entry_id text NOT NULL,class_id text NOT NULL,revision_no integer NOT NULL CHECK(revision_no>0),body text NOT NULL CHECK(length(body)<=65536),body_sha256 text NOT NULL CHECK(body_sha256 ~ '^[a-f0-9]{64}$'),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(entry_id,class_id) REFERENCES v3.logbook_entries(id,class_id),UNIQUE(id,entry_id,class_id),UNIQUE(entry_id,revision_no));
CREATE INDEX logbook_entry_revisions_entry_idx ON v3.logbook_entry_revisions(entry_id,revision_no,id);
CREATE INDEX logbook_entry_revisions_actor_idx ON v3.logbook_entry_revisions(created_by,created_at);
CREATE TABLE v3.logbook_revision_files(revision_id text NOT NULL,entry_id text NOT NULL,class_id text NOT NULL,file_asset_id text NOT NULL,position integer NOT NULL CHECK(position BETWEEN 1 AND 10),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(revision_id,entry_id,class_id) REFERENCES v3.logbook_entry_revisions(id,entry_id,class_id),FOREIGN KEY(file_asset_id,class_id) REFERENCES v3.file_assets(id,class_id),PRIMARY KEY(revision_id,file_asset_id),UNIQUE(revision_id,position));
CREATE INDEX logbook_revision_files_asset_idx ON v3.logbook_revision_files(file_asset_id,class_id,revision_id);
CREATE INDEX logbook_revision_files_entry_idx ON v3.logbook_revision_files(entry_id,class_id,revision_id);
CREATE TABLE v3.logbook_entry_events(id text PRIMARY KEY,entry_id text NOT NULL,class_id text NOT NULL,revision_id text NOT NULL,version bigint NOT NULL CHECK(version>0),state text NOT NULL CHECK(state IN ('DRAFT','SUBMITTED','REVISION_REQUIRED','ACCEPTED')),actor_id text NOT NULL REFERENCES v3.users(id),mentor_assignment_id text REFERENCES v3.mentor_assignments(id),feedback text NOT NULL DEFAULT '' CHECK(length(feedback)<=8000),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(revision_id,entry_id,class_id) REFERENCES v3.logbook_entry_revisions(id,entry_id,class_id),UNIQUE(entry_id,version));
CREATE INDEX logbook_entry_events_current_idx ON v3.logbook_entry_events(entry_id,version DESC);
CREATE INDEX logbook_entry_events_class_idx ON v3.logbook_entry_events(class_id,occurred_at,entry_id);
CREATE INDEX logbook_entry_events_actor_idx ON v3.logbook_entry_events(actor_id,occurred_at);
CREATE INDEX logbook_entry_events_mentor_idx ON v3.logbook_entry_events(mentor_assignment_id,occurred_at) WHERE mentor_assignment_id IS NOT NULL;
CREATE TABLE v3.logbook_entry_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_logbook_entry_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_ENTRY_HISTORY_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_logbook_entry_immutable() FROM PUBLIC;
CREATE TRIGGER logbook_entries_immutable BEFORE UPDATE ON v3.logbook_entries FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();
CREATE TRIGGER logbook_entry_revisions_immutable BEFORE UPDATE ON v3.logbook_entry_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();
CREATE TRIGGER logbook_revision_files_immutable BEFORE UPDATE ON v3.logbook_revision_files FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();
CREATE TRIGGER logbook_entry_events_immutable BEFORE UPDATE ON v3.logbook_entry_events FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();
CREATE TRIGGER logbook_entry_commands_immutable BEFORE UPDATE ON v3.logbook_entry_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();
CREATE FUNCTION v3.guard_logbook_entry_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE period_open timestamptz; period_due timestamptz; published_set text; enrollment_state text; episode_closed timestamptz; BEGIN
 SELECT p.starts_at,p.due_at,p.set_id INTO period_open,period_due,published_set FROM v3.logbook_periods p JOIN v3.logbook_period_publications pub ON pub.set_id=p.set_id AND pub.class_id=p.class_id WHERE p.id=NEW.period_id AND p.class_id=NEW.class_id AND pub.revision=(SELECT max(x.revision) FROM v3.logbook_period_publications x WHERE x.class_id=NEW.class_id);
 SELECT status INTO enrollment_state FROM v3.enrollments WHERE id=NEW.enrollment_id AND class_id=NEW.class_id AND user_id=NEW.owner_user_id;
 SELECT ended_at INTO episode_closed FROM v3.episodes WHERE id=NEW.episode_id AND enrollment_id=NEW.enrollment_id;
 IF published_set IS NULL OR clock_timestamp()<period_open OR clock_timestamp()>period_due OR enrollment_state IS DISTINCT FROM 'ACTIVE' OR episode_closed IS NOT NULL OR NEW.created_by IS DISTINCT FROM NEW.owner_user_id
  OR NOT EXISTS(SELECT 1 FROM v3.roles r WHERE r.enrollment_id=NEW.enrollment_id AND r.class_id=NEW.class_id AND r.role='student' AND r.revoked_at IS NULL)
  OR EXISTS(SELECT 1 FROM v3.enrollment_access a WHERE a.enrollment_id=NEW.enrollment_id AND a.suspended_at IS NOT NULL) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_ENTRY_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_entry_insert() FROM PUBLIC;
CREATE TRIGGER logbook_entry_insert_guard BEFORE INSERT ON v3.logbook_entries FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_insert();
CREATE FUNCTION v3.guard_logbook_revision_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE owner_id text; last_state text; next_revision integer; BEGIN
 PERFORM 1 FROM v3.logbook_entries WHERE id=NEW.entry_id AND class_id=NEW.class_id FOR UPDATE;
 SELECT owner_user_id INTO owner_id FROM v3.logbook_entries WHERE id=NEW.entry_id AND class_id=NEW.class_id;
 SELECT state INTO last_state FROM v3.logbook_entry_events WHERE entry_id=NEW.entry_id ORDER BY version DESC LIMIT 1;
 SELECT coalesce(max(revision_no),0)+1 INTO next_revision FROM v3.logbook_entry_revisions WHERE entry_id=NEW.entry_id;
 IF NEW.created_by IS DISTINCT FROM owner_id OR NEW.revision_no IS DISTINCT FROM next_revision OR last_state IN ('SUBMITTED','ACCEPTED') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_REVISION_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_revision_insert() FROM PUBLIC;
CREATE TRIGGER logbook_revision_insert_guard BEFORE INSERT ON v3.logbook_entry_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_revision_insert();
CREATE FUNCTION v3.guard_logbook_revision_file_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE owner_id text; asset_owner text; asset_state text; asset_purpose text; BEGIN
 SELECT e.owner_user_id INTO owner_id FROM v3.logbook_entry_revisions r JOIN v3.logbook_entries e ON e.id=r.entry_id AND e.class_id=r.class_id WHERE r.id=NEW.revision_id AND r.entry_id=NEW.entry_id AND r.class_id=NEW.class_id;
 SELECT owner_user_id,state,purpose INTO asset_owner,asset_state,asset_purpose FROM v3.file_assets WHERE id=NEW.file_asset_id AND class_id=NEW.class_id;
 IF owner_id IS NULL OR asset_owner IS DISTINCT FROM owner_id OR asset_state IS DISTINCT FROM 'AVAILABLE' OR asset_purpose IS DISTINCT FROM 'EVIDENCE' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_FILE_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_revision_file_insert() FROM PUBLIC;
CREATE TRIGGER logbook_revision_file_insert_guard BEFORE INSERT ON v3.logbook_revision_files FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_revision_file_insert();
CREATE FUNCTION v3.guard_logbook_entry_event_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3,v3identity AS $$ DECLARE owner_id text; prior_state text; prior_revision text; prior_version bigint; latest_revision text; mentor_enrollment text; mentor_user text; site_admin boolean; BEGIN
 PERFORM 1 FROM v3.logbook_entries WHERE id=NEW.entry_id AND class_id=NEW.class_id FOR UPDATE;
 SELECT owner_user_id INTO owner_id FROM v3.logbook_entries WHERE id=NEW.entry_id AND class_id=NEW.class_id;
 SELECT state,revision_id,version INTO prior_state,prior_revision,prior_version FROM v3.logbook_entry_events WHERE entry_id=NEW.entry_id ORDER BY version DESC LIMIT 1;
 SELECT id INTO latest_revision FROM v3.logbook_entry_revisions WHERE entry_id=NEW.entry_id ORDER BY revision_no DESC LIMIT 1;
 IF NEW.version IS DISTINCT FROM COALESCE(prior_version,0)+1 OR NEW.revision_id IS DISTINCT FROM latest_revision THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_EVENT_VERSION_REJECTED';
 END IF;
 IF NEW.state='DRAFT' THEN
  IF NEW.actor_id IS DISTINCT FROM owner_id OR NEW.mentor_assignment_id IS NOT NULL OR NEW.feedback<>'' OR NOT ((prior_state IS NULL AND NEW.version=1) OR prior_state IN ('DRAFT','REVISION_REQUIRED')) OR (prior_state='DRAFT' AND NEW.revision_id IS NOT DISTINCT FROM prior_revision) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_DRAFT_TRANSITION_REJECTED';
  END IF;
 ELSIF NEW.state='SUBMITTED' THEN
  IF prior_state IS DISTINCT FROM 'DRAFT' OR NEW.actor_id IS DISTINCT FROM owner_id OR NEW.revision_id IS DISTINCT FROM prior_revision OR NEW.mentor_assignment_id IS NOT NULL OR NEW.feedback<>'' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_SUBMIT_TRANSITION_REJECTED';
  END IF;
 ELSE
  SELECT coalesce(r.admin,false) INTO site_admin FROM v3identity.site_roles r JOIN v3identity.links l ON l.user_id=r.user_id WHERE r.user_id=NEW.actor_id AND l.active AND l.valid_until>clock_timestamp();
  IF NEW.mentor_assignment_id IS NOT NULL THEN
   SELECT mentor_enrollment_id INTO mentor_enrollment FROM v3.mentor_assignments WHERE id=NEW.mentor_assignment_id AND class_id=NEW.class_id AND ended_at IS NULL AND student_enrollment_id=(SELECT enrollment_id FROM v3.logbook_entries WHERE id=NEW.entry_id);
   SELECT e.user_id INTO mentor_user FROM v3.enrollments e WHERE e.id=mentor_enrollment AND e.class_id=NEW.class_id AND e.status='ACTIVE'
    AND EXISTS(SELECT 1 FROM v3.roles r WHERE r.enrollment_id=e.id AND r.class_id=e.class_id AND r.role='teacher' AND r.revoked_at IS NULL)
    AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access a WHERE a.enrollment_id=e.id AND a.suspended_at IS NOT NULL);
  END IF;
  IF prior_state IS DISTINCT FROM 'SUBMITTED' OR NEW.revision_id IS DISTINCT FROM prior_revision OR length(NEW.feedback)<1 OR NEW.actor_id IS NOT DISTINCT FROM owner_id OR NOT (coalesce(site_admin,false) OR (mentor_user IS NOT DISTINCT FROM NEW.actor_id AND NEW.mentor_assignment_id IS NOT NULL)) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LOGBOOK_REVIEW_TRANSITION_REJECTED';
  END IF;
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_logbook_entry_event_insert() FROM PUBLIC;
CREATE TRIGGER logbook_entry_event_insert_guard BEFORE INSERT ON v3.logbook_entry_events FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_event_insert();`

const logbookReviewSchema = `CREATE TABLE v3.logbook_review_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE TRIGGER logbook_review_commands_immutable BEFORE UPDATE ON v3.logbook_review_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_logbook_entry_immutable();`

func LogbookPeriodChecksum() string { return Digest([]byte(logbookPeriodSchema)) }
func LogbookEntryChecksum() string  { return Digest([]byte(logbookEntrySchema)) }
func LogbookReviewChecksum() string { return Digest([]byte(logbookReviewSchema)) }

// InstallLogbookPeriods is restricted to the marker-bound, fresh synthetic QA
// target and demands a new SQL/custom backup plus an actual isolated restore.
func InstallLogbookPeriods(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil || VerifyInstallmentProof("m12-cp01", "pre_m12_cp01.dump") != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810912)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var previous string
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m05-cp04-1'").Scan(&previous) != nil || previous != Digest([]byte(participationProvenanceSchema)) {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m06-cp02-1'").Scan(&previous) != nil || previous != Digest([]byte(privateFileDeliverySchema)) {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m12-cp01-1'").Scan(&previous)
		if err == nil {
			if previous != LogbookPeriodChecksum() {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, logbookPeriodSchema, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES('m12-cp01-1',$1,$2)", LogbookPeriodChecksum(), proofArchiveHash("m12-cp01"))
		return err
	})
}

func installLogbookSlice(ctx context.Context, pool *pgxpool.Pool, checkpoint, archive, version, ddl, dependency string, dependencyChecksum string) error {
	if pool == nil || VerifyInstallmentProof(checkpoint, archive) != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810912)").Scan(&locked); err != nil || !locked {
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

func InstallLogbookEntries(ctx context.Context, pool *pgxpool.Pool) error {
	return installLogbookSlice(ctx, pool, "m12-cp02", "pre_m12_cp02.dump", "m12-cp02-1", logbookEntrySchema, "m12-cp01-1", LogbookPeriodChecksum())
}

func InstallLogbookReviews(ctx context.Context, pool *pgxpool.Pool) error {
	return installLogbookSlice(ctx, pool, "m12-cp03", "pre_m12_cp03.dump", "m12-cp03-1", logbookReviewSchema, "m12-cp02-1", LogbookEntryChecksum())
}
