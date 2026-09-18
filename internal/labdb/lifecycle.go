package labdb

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/migration/backup"
	"lmsv3/internal/migration/runner"
	"lmsv3/internal/platform/database"
	"lmsv3/internal/platform/transaction"
	"os"
	"path/filepath"
	"time"
)

const identitySchema = `CREATE SCHEMA v3identity;
CREATE TABLE v3identity.links(user_id text PRIMARY KEY REFERENCES v3.users(id),issuer text NOT NULL,subject text NOT NULL,version bigint NOT NULL CHECK(version>0),active boolean NOT NULL,valid_until timestamptz NOT NULL,UNIQUE(issuer,subject));
CREATE TABLE v3identity.sessions(token_hash text PRIMARY KEY,user_id text NOT NULL REFERENCES v3identity.links(user_id),authority_version bigint NOT NULL,csrf_hash text NOT NULL,expires_at timestamptz NOT NULL,absolute_until timestamptz NOT NULL,revoked_at timestamptz,created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE INDEX identity_sessions_user_idx ON v3identity.sessions(user_id);
CREATE TABLE v3identity.challenges(nonce_hash text PRIMARY KEY,binding_hash text NOT NULL,expires_at timestamptz NOT NULL,consumed_at timestamptz);
CREATE TABLE v3identity.consumed(issuer text NOT NULL,jti text NOT NULL,consumed_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(issuer,jti));
CREATE TABLE v3identity.site_roles(user_id text PRIMARY KEY REFERENCES v3.users(id),admin boolean NOT NULL DEFAULT false,version bigint NOT NULL DEFAULT 1 CHECK(version>0));
CREATE TABLE v3identity.role_events(user_id text NOT NULL REFERENCES v3.users(id),version bigint NOT NULL,previous_admin boolean NOT NULL,admin boolean NOT NULL,actor_id text NOT NULL REFERENCES v3.users(id),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(user_id,version));
CREATE INDEX role_events_actor_idx ON v3identity.role_events(actor_id);
CREATE TABLE v3identity.account_events(user_id text NOT NULL REFERENCES v3identity.links(user_id),version bigint NOT NULL,previous_version bigint NOT NULL,previous_active boolean NOT NULL,active boolean NOT NULL,occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(user_id,version));`

const accessSchema = `CREATE TABLE v3.class_access(class_id text PRIMARY KEY REFERENCES v3.classes(id),closed_at timestamptz);
CREATE TABLE v3.enrollment_access(enrollment_id text PRIMARY KEY REFERENCES v3.enrollments(id),suspended_at timestamptz);
CREATE TABLE v3.transition_events(id text PRIMARY KEY,actor_id text NOT NULL REFERENCES v3.users(id),class_id text NOT NULL REFERENCES v3.classes(id),enrollment_id text REFERENCES v3.enrollments(id),action text NOT NULL,previous_state text NOT NULL,next_state text NOT NULL,occurred_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE INDEX transition_actor_idx ON v3.transition_events(actor_id);
CREATE INDEX transition_class_idx ON v3.transition_events(class_id);
CREATE INDEX transition_enrollment_idx ON v3.transition_events(enrollment_id);`

const outboxWorkerSchema = `ALTER TABLE v3.outbox ADD COLUMN topic text NOT NULL DEFAULT 'audit.v1' CHECK(topic ~ '^[a-z][a-z0-9_.-]{0,63}$');
ALTER TABLE v3.outbox ADD COLUMN sent_at timestamptz;
ALTER TABLE v3.outbox ADD COLUMN last_error_code text CHECK(last_error_code IS NULL OR last_error_code ~ '^[A-Z][A-Z0-9_]{0,63}$');
CREATE INDEX outbox_topic_pending_idx ON v3.outbox(topic,next_attempt,event_id) WHERE status <> 'SENT';
CREATE TABLE v3ops.outbox_attempts(event_id text NOT NULL REFERENCES v3.outbox(event_id),lease_id text NOT NULL,attempt_number integer NOT NULL CHECK(attempt_number>0),outcome text NOT NULL CHECK(outcome IN ('CLAIMED','DELIVERED','RETRY_SCHEDULED','FAILED')),error_code text CHECK(error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(event_id,lease_id,outcome),CHECK((outcome IN ('RETRY_SCHEDULED','FAILED'))=(error_code IS NOT NULL)));
CREATE INDEX outbox_attempts_occurred_idx ON v3ops.outbox_attempts(occurred_at,event_id);
CREATE TABLE v3ops.outbox_failures(event_id text PRIMARY KEY REFERENCES v3.outbox(event_id),final_attempt integer NOT NULL CHECK(final_attempt>0),error_code text NOT NULL CHECK(error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),failed_at timestamptz NOT NULL DEFAULT clock_timestamp());`

const classGroupingSchema = `CREATE TABLE v3.class_groups(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(id,class_id));
CREATE UNIQUE INDEX class_groups_name_idx ON v3.class_groups(class_id,lower(name));
CREATE INDEX class_groups_actor_idx ON v3.class_groups(created_by);
CREATE TABLE v3.group_memberships(id text PRIMARY KEY,class_id text NOT NULL,group_id text NOT NULL,enrollment_id text NOT NULL,assigned_by text NOT NULL REFERENCES v3.users(id),assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),ended_at timestamptz,supersedes_id text REFERENCES v3.group_memberships(id),FOREIGN KEY(group_id,class_id) REFERENCES v3.class_groups(id,class_id),FOREIGN KEY(enrollment_id,class_id) REFERENCES v3.enrollments(id,class_id),CHECK(ended_at IS NULL OR ended_at>=assigned_at));
CREATE UNIQUE INDEX group_memberships_active_enrollment_idx ON v3.group_memberships(enrollment_id) WHERE ended_at IS NULL;
CREATE INDEX group_memberships_group_idx ON v3.group_memberships(group_id,class_id,assigned_at);
CREATE INDEX group_memberships_enrollment_idx ON v3.group_memberships(enrollment_id,class_id,assigned_at);
CREATE INDEX group_memberships_class_idx ON v3.group_memberships(class_id,assigned_at);
CREATE INDEX group_memberships_actor_idx ON v3.group_memberships(assigned_by);
CREATE INDEX group_memberships_supersedes_idx ON v3.group_memberships(supersedes_id) WHERE supersedes_id IS NOT NULL;
CREATE TABLE v3.mentor_assignments(id text PRIMARY KEY,class_id text NOT NULL,student_enrollment_id text NOT NULL,mentor_enrollment_id text NOT NULL,assigned_by text NOT NULL REFERENCES v3.users(id),assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),ended_at timestamptz,supersedes_id text REFERENCES v3.mentor_assignments(id),FOREIGN KEY(student_enrollment_id,class_id) REFERENCES v3.enrollments(id,class_id),FOREIGN KEY(mentor_enrollment_id,class_id) REFERENCES v3.enrollments(id,class_id),CHECK(student_enrollment_id<>mentor_enrollment_id),CHECK(ended_at IS NULL OR ended_at>=assigned_at));
CREATE UNIQUE INDEX mentor_assignments_active_student_idx ON v3.mentor_assignments(student_enrollment_id) WHERE ended_at IS NULL;
CREATE INDEX mentor_assignments_student_idx ON v3.mentor_assignments(student_enrollment_id,class_id,assigned_at);
CREATE INDEX mentor_assignments_mentor_idx ON v3.mentor_assignments(mentor_enrollment_id,class_id,assigned_at);
CREATE INDEX mentor_assignments_class_idx ON v3.mentor_assignments(class_id,assigned_at);
CREATE INDEX mentor_assignments_actor_idx ON v3.mentor_assignments(assigned_by);
CREATE INDEX mentor_assignments_supersedes_idx ON v3.mentor_assignments(supersedes_id) WHERE supersedes_id IS NOT NULL;`

const participationProvenanceSchema = `ALTER TABLE v3.episodes ADD COLUMN kind text NOT NULL DEFAULT 'INITIAL' CHECK(kind IN ('INITIAL','REJOIN','RETAKE','TRANSFER','EQUIVALENCE'));
ALTER TABLE v3.episodes ADD COLUMN origin_episode_id text REFERENCES v3.episodes(id);
ALTER TABLE v3.episodes ADD COLUMN recognition_policy text NOT NULL DEFAULT 'SEPARATE_HISTORY' CHECK(recognition_policy IN ('SEPARATE_HISTORY','REVIEW_REQUIRED','REFERENCE_ONLY'));
ALTER TABLE v3.episodes ADD COLUMN created_by text REFERENCES v3.users(id);
ALTER TABLE v3.episodes ADD CONSTRAINT episodes_origin_policy_ck CHECK((kind='INITIAL' AND origin_episode_id IS NULL AND recognition_policy='SEPARATE_HISTORY') OR (kind IN ('REJOIN','RETAKE') AND origin_episode_id IS NOT NULL AND recognition_policy='SEPARATE_HISTORY') OR (kind='TRANSFER' AND origin_episode_id IS NOT NULL AND recognition_policy='REVIEW_REQUIRED') OR (kind='EQUIVALENCE' AND origin_episode_id IS NOT NULL AND recognition_policy='REFERENCE_ONLY'));
CREATE INDEX episodes_enrollment_history_idx ON v3.episodes(enrollment_id,opened_at,id);
CREATE INDEX episodes_origin_idx ON v3.episodes(origin_episode_id) WHERE origin_episode_id IS NOT NULL;
CREATE INDEX episodes_actor_idx ON v3.episodes(created_by) WHERE created_by IS NOT NULL;
CREATE TABLE v3.invitations(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),user_id text NOT NULL REFERENCES v3.users(id),role text NOT NULL CHECK(role IN ('teacher','student')),status text NOT NULL DEFAULT 'INVITED' CHECK(status IN ('INVITED','ACCEPTED')),invited_by text NOT NULL REFERENCES v3.users(id),invited_at timestamptz NOT NULL DEFAULT clock_timestamp(),accepted_at timestamptz,accepted_episode_id text REFERENCES v3.episodes(id),CHECK((status='INVITED' AND accepted_at IS NULL AND accepted_episode_id IS NULL) OR (status='ACCEPTED' AND accepted_at IS NOT NULL)));
CREATE UNIQUE INDEX invitations_active_idx ON v3.invitations(class_id,user_id,role) WHERE status='INVITED';
CREATE INDEX invitations_user_idx ON v3.invitations(user_id,status,invited_at);
CREATE INDEX invitations_class_idx ON v3.invitations(class_id,status,invited_at);
CREATE INDEX invitations_actor_idx ON v3.invitations(invited_by);
CREATE INDEX invitations_episode_idx ON v3.invitations(accepted_episode_id) WHERE accepted_episode_id IS NOT NULL;`

const privateFileAssetSchema = `CREATE TABLE v3.file_assets(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),owner_user_id text NOT NULL REFERENCES v3.users(id),purpose text NOT NULL CHECK(purpose IN ('MATERIAL','EVIDENCE')),object_key text NOT NULL CHECK(object_key ~ '^[a-f0-9]{64}$'),original_name text NOT NULL CHECK(length(original_name) BETWEEN 1 AND 180),declared_media_type text NOT NULL CHECK(length(declared_media_type) BETWEEN 3 AND 100),detected_media_type text NOT NULL CHECK(length(detected_media_type) BETWEEN 3 AND 100),byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 2097152),sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),state text NOT NULL CHECK(state IN ('QUARANTINED','AVAILABLE','REJECTED')),scan_status text NOT NULL CHECK(scan_status IN ('UNAVAILABLE','CLEAN','THREAT')),scan_code text NOT NULL CHECK(scan_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),available_at timestamptz,CHECK(object_key<>sha256),CHECK((state='AVAILABLE' AND scan_status='CLEAN' AND available_at IS NOT NULL) OR (state<>'AVAILABLE' AND available_at IS NULL)),UNIQUE(id,class_id));
CREATE INDEX file_assets_class_state_idx ON v3.file_assets(class_id,state,created_at,id);
CREATE INDEX file_assets_owner_idx ON v3.file_assets(owner_user_id,class_id,created_at);
CREATE INDEX file_assets_actor_idx ON v3.file_assets(created_by,created_at);
CREATE UNIQUE INDEX file_assets_object_idx ON v3.file_assets(object_key);
CREATE INDEX file_assets_quarantine_idx ON v3.file_assets(created_at,id) WHERE state='QUARANTINED';
CREATE TABLE v3.file_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));`

const privateFileDeliverySchema = `CREATE TABLE v3.file_download_grants(token_hash text PRIMARY KEY CHECK(token_hash ~ '^[a-f0-9]{64}$'),asset_id text NOT NULL REFERENCES v3.file_assets(id),user_id text NOT NULL REFERENCES v3.users(id),expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),CHECK(expires_at>created_at));
CREATE INDEX file_download_grants_asset_idx ON v3.file_download_grants(asset_id,expires_at);
CREATE INDEX file_download_grants_user_idx ON v3.file_download_grants(user_id,expires_at);
CREATE INDEX file_download_grants_expiry_idx ON v3.file_download_grants(expires_at);
CREATE TABLE v3.file_access_events(id text PRIMARY KEY,asset_id text NOT NULL REFERENCES v3.file_assets(id),user_id text NOT NULL REFERENCES v3.users(id),grant_hash text NOT NULL REFERENCES v3.file_download_grants(token_hash),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),outcome text NOT NULL CHECK(outcome='DELIVERED'));
CREATE INDEX file_access_events_asset_idx ON v3.file_access_events(asset_id,occurred_at,id);
CREATE INDEX file_access_events_user_idx ON v3.file_access_events(user_id,occurred_at,id);
CREATE INDEX file_access_events_grant_idx ON v3.file_access_events(grant_hash,occurred_at,id);`

const contentStructureSchema = `CREATE TABLE v3.content_sections(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),position integer NOT NULL CHECK(position BETWEEN 1 AND 10000),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(class_id,id));
CREATE UNIQUE INDEX content_sections_position_idx ON v3.content_sections(class_id,position);
CREATE INDEX content_sections_class_idx ON v3.content_sections(class_id,id);
CREATE INDEX content_sections_actor_idx ON v3.content_sections(created_by,created_at);
CREATE TABLE v3.content_activities(id text PRIMARY KEY,class_id text NOT NULL,section_id text NOT NULL,activity_type text NOT NULL CHECK(activity_type IN ('RESOURCE','ASSIGNMENT','QUIZ')),title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),position integer NOT NULL CHECK(position BETWEEN 1 AND 10000),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(class_id,section_id) REFERENCES v3.content_sections(class_id,id),UNIQUE(class_id,id));
CREATE UNIQUE INDEX content_activities_position_idx ON v3.content_activities(section_id,position);
CREATE INDEX content_activities_class_idx ON v3.content_activities(class_id,section_id,id);
CREATE INDEX content_activities_section_idx ON v3.content_activities(section_id,id);
CREATE INDEX content_activities_actor_idx ON v3.content_activities(created_by,created_at);
CREATE TABLE v3.content_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));`

const contentSafetySchema = `CREATE TABLE v3.content_revisions(id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,revision_no integer NOT NULL CHECK(revision_no>0),state text NOT NULL DEFAULT 'DRAFT' CHECK(state IN ('DRAFT','PUBLISHED','WITHDRAWN')),canonical_body text NOT NULL CHECK(length(canonical_body) BETWEEN 1 AND 524288),content_format text NOT NULL CHECK(content_format='SAFE_HTML'),timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 64),available_from timestamptz,due_at timestamptz,cutoff_at timestamptz,version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(class_id,activity_id) REFERENCES v3.content_activities(class_id,id),UNIQUE(class_id,id),UNIQUE(activity_id,revision_no),CHECK(due_at IS NULL OR available_from IS NULL OR due_at>=available_from),CHECK(cutoff_at IS NULL OR due_at IS NULL OR cutoff_at>=due_at),CHECK(cutoff_at IS NULL OR available_from IS NULL OR cutoff_at>=available_from));
CREATE INDEX content_revisions_class_idx ON v3.content_revisions(class_id,activity_id,id);
CREATE INDEX content_revisions_activity_idx ON v3.content_revisions(activity_id,revision_no);
CREATE INDEX content_revisions_actor_idx ON v3.content_revisions(created_by,created_at);
CREATE INDEX content_revisions_published_available_idx ON v3.content_revisions(class_id,available_from,activity_id) WHERE state='PUBLISHED';
CREATE TABLE v3.content_revision_sources(revision_id text PRIMARY KEY REFERENCES v3.content_revisions(id),raw_body text NOT NULL CHECK(length(raw_body) BETWEEN 1 AND 65536),raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp());`

const contentRevisionContractSchema = `ALTER TABLE v3.content_revisions ADD CONSTRAINT content_revisions_contract_identity_uniq UNIQUE(class_id,activity_id,id);
CREATE TABLE v3.assignment_revision_rules(revision_id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,contract_version smallint NOT NULL DEFAULT 1 CHECK(contract_version=1),submission_mode text NOT NULL CHECK(submission_mode IN ('TEXT','FILES','TEXT_AND_FILES')),max_attempts integer NOT NULL CHECK(max_attempts BETWEEN 1 AND 100),max_files integer NOT NULL CHECK(max_files BETWEEN 0 AND 20),max_file_bytes bigint NOT NULL CHECK(max_file_bytes BETWEEN 0 AND 2097152),late_policy text NOT NULL CHECK(late_policy IN ('REJECT_AFTER_CUTOFF','ALLOW_MARKED_LATE')),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(class_id,activity_id,revision_id) REFERENCES v3.content_revisions(class_id,activity_id,id),CHECK((submission_mode='TEXT' AND max_files=0 AND max_file_bytes=0) OR (submission_mode IN ('FILES','TEXT_AND_FILES') AND max_files BETWEEN 1 AND 20 AND max_file_bytes BETWEEN 1 AND 2097152)));
CREATE INDEX assignment_revision_rules_class_idx ON v3.assignment_revision_rules(class_id,activity_id,revision_id);
CREATE INDEX assignment_revision_rules_actor_idx ON v3.assignment_revision_rules(created_by,created_at);
CREATE FUNCTION v3.guard_content_revision_update() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.activity_id IS DISTINCT FROM OLD.activity_id OR NEW.revision_no IS DISTINCT FROM OLD.revision_no OR NEW.content_format IS DISTINCT FROM OLD.content_format OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.version IS DISTINCT FROM OLD.version+1 OR NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='CONTENT_REVISION_IDENTITY_IMMUTABLE';
 END IF;
 IF OLD.state='WITHDRAWN' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='WITHDRAWN_REVISION_IMMUTABLE';
 END IF;
 IF OLD.state='PUBLISHED' THEN
  IF NEW.state<>'WITHDRAWN' OR NEW.canonical_body IS DISTINCT FROM OLD.canonical_body OR NEW.timezone IS DISTINCT FROM OLD.timezone OR NEW.available_from IS DISTINCT FROM OLD.available_from OR NEW.due_at IS DISTINCT FROM OLD.due_at OR NEW.cutoff_at IS DISTINCT FROM OLD.cutoff_at THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PUBLISHED_REVISION_IMMUTABLE';
  END IF;
 ELSIF OLD.state='DRAFT' THEN
  IF NEW.state NOT IN ('DRAFT','PUBLISHED') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='INVALID_REVISION_TRANSITION';
  END IF;
  IF NEW.state='PUBLISHED' AND (NEW.canonical_body IS DISTINCT FROM OLD.canonical_body OR NEW.timezone IS DISTINCT FROM OLD.timezone OR NEW.available_from IS DISTINCT FROM OLD.available_from OR NEW.due_at IS DISTINCT FROM OLD.due_at OR NEW.cutoff_at IS DISTINCT FROM OLD.cutoff_at) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PUBLISH_MUST_NOT_MUTATE_REVISION';
  END IF;
  IF NEW.state='PUBLISHED' AND EXISTS(SELECT 1 FROM v3.content_activities a WHERE a.id=OLD.activity_id AND a.class_id=OLD.class_id AND a.activity_type='ASSIGNMENT') AND NOT EXISTS(SELECT 1 FROM v3.assignment_revision_rules r WHERE r.revision_id=OLD.id AND r.class_id=OLD.class_id AND r.activity_id=OLD.activity_id) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='ASSIGNMENT_RULE_REQUIRED';
  END IF;
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_content_revision_update() FROM PUBLIC;
CREATE TRIGGER content_revision_update_guard BEFORE UPDATE ON v3.content_revisions FOR EACH ROW EXECUTE FUNCTION v3.guard_content_revision_update();
CREATE FUNCTION v3.guard_content_revision_source_update() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE parent_state text;
BEGIN
 IF NEW.revision_id IS DISTINCT FROM OLD.revision_id OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='CONTENT_SOURCE_IDENTITY_IMMUTABLE';
 END IF;
 SELECT state INTO parent_state FROM v3.content_revisions WHERE id=OLD.revision_id;
 IF parent_state IS DISTINCT FROM 'DRAFT' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PUBLISHED_SOURCE_IMMUTABLE';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_content_revision_source_update() FROM PUBLIC;
CREATE TRIGGER content_revision_source_update_guard BEFORE UPDATE ON v3.content_revision_sources FOR EACH ROW EXECUTE FUNCTION v3.guard_content_revision_source_update();
CREATE FUNCTION v3.guard_assignment_revision_rule() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE parent_state text; parent_type text;
BEGIN
 SELECT r.state,a.activity_type INTO parent_state,parent_type FROM v3.content_revisions r JOIN v3.content_activities a ON a.id=r.activity_id AND a.class_id=r.class_id WHERE r.id=NEW.revision_id AND r.class_id=NEW.class_id AND r.activity_id=NEW.activity_id;
 IF parent_state IS DISTINCT FROM 'DRAFT' OR parent_type IS DISTINCT FROM 'ASSIGNMENT' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='ASSIGNMENT_RULE_PARENT_REJECTED';
 END IF;
 IF TG_OP='UPDATE' AND (NEW.revision_id IS DISTINCT FROM OLD.revision_id OR NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.activity_id IS DISTINCT FROM OLD.activity_id OR NEW.contract_version IS DISTINCT FROM OLD.contract_version OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.version IS DISTINCT FROM OLD.version+1 OR NEW.updated_at<OLD.updated_at) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='ASSIGNMENT_RULE_IDENTITY_IMMUTABLE';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_assignment_revision_rule() FROM PUBLIC;
CREATE TRIGGER assignment_revision_rule_guard BEFORE INSERT OR UPDATE ON v3.assignment_revision_rules FOR EACH ROW EXECUTE FUNCTION v3.guard_assignment_revision_rule();`

const submissionFoundationSchema = `ALTER TABLE v3.enrollments ADD CONSTRAINT enrollments_submission_identity_uniq UNIQUE(id,class_id,user_id);
ALTER TABLE v3.episodes ADD CONSTRAINT episodes_submission_identity_uniq UNIQUE(id,enrollment_id);
CREATE TABLE v3.submission_aggregates(id text PRIMARY KEY,class_id text NOT NULL,activity_id text NOT NULL,assignment_revision_id text NOT NULL,enrollment_id text NOT NULL,episode_id text NOT NULL,owner_user_id text NOT NULL,current_attempt_no integer NOT NULL DEFAULT 1 CHECK(current_attempt_no>0),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(class_id,activity_id,assignment_revision_id) REFERENCES v3.content_revisions(class_id,activity_id,id),FOREIGN KEY(enrollment_id,class_id,owner_user_id) REFERENCES v3.enrollments(id,class_id,user_id),FOREIGN KEY(episode_id,enrollment_id) REFERENCES v3.episodes(id,enrollment_id),UNIQUE(id,class_id),UNIQUE(id,class_id,activity_id,assignment_revision_id),UNIQUE(activity_id,episode_id));
CREATE INDEX submission_aggregates_owner_idx ON v3.submission_aggregates(owner_user_id,class_id,created_at,id);
CREATE INDEX submission_aggregates_assignment_idx ON v3.submission_aggregates(class_id,activity_id,assignment_revision_id,episode_id);
CREATE TABLE v3.submission_attempts(id text PRIMARY KEY,aggregate_id text NOT NULL,class_id text NOT NULL,activity_id text NOT NULL,assignment_revision_id text NOT NULL,attempt_no integer NOT NULL CHECK(attempt_no>0),state text NOT NULL DEFAULT 'DRAFT' CHECK(state IN ('DRAFT','LOCKED','RETURNED')),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(aggregate_id,class_id,activity_id,assignment_revision_id) REFERENCES v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id),UNIQUE(aggregate_id,attempt_no),UNIQUE(id,class_id,aggregate_id));
CREATE INDEX submission_attempts_assignment_idx ON v3.submission_attempts(class_id,activity_id,assignment_revision_id,aggregate_id,attempt_no);
CREATE TABLE v3.submission_revisions(id text PRIMARY KEY,class_id text NOT NULL,aggregate_id text NOT NULL,attempt_id text NOT NULL,revision_no integer NOT NULL CHECK(revision_no>0),body text NOT NULL CHECK(length(body)<=65536),body_sha256 text NOT NULL CHECK(body_sha256 ~ '^[a-f0-9]{64}$'),created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(attempt_id,class_id,aggregate_id) REFERENCES v3.submission_attempts(id,class_id,aggregate_id),UNIQUE(id,class_id),UNIQUE(attempt_id,revision_no));
CREATE INDEX submission_revisions_aggregate_idx ON v3.submission_revisions(aggregate_id,attempt_id,revision_no,id);
CREATE TABLE v3.submission_revision_files(revision_id text NOT NULL,class_id text NOT NULL,file_asset_id text NOT NULL,position integer NOT NULL CHECK(position BETWEEN 1 AND 20),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(revision_id,class_id) REFERENCES v3.submission_revisions(id,class_id),FOREIGN KEY(file_asset_id,class_id) REFERENCES v3.file_assets(id,class_id),PRIMARY KEY(revision_id,file_asset_id),UNIQUE(revision_id,position));
CREATE INDEX submission_revision_files_asset_idx ON v3.submission_revision_files(file_asset_id,class_id,revision_id);
CREATE TABLE v3.submission_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_submission_foundation_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE revision_state text; activity_kind text; enrollment_state text; episode_closed timestamptz;
BEGIN
 SELECT r.state,a.activity_type INTO revision_state,activity_kind FROM v3.content_revisions r JOIN v3.content_activities a ON a.id=r.activity_id AND a.class_id=r.class_id JOIN v3.assignment_revision_rules ar ON ar.revision_id=r.id AND ar.class_id=r.class_id AND ar.activity_id=r.activity_id WHERE r.id=NEW.assignment_revision_id AND r.class_id=NEW.class_id AND r.activity_id=NEW.activity_id;
 SELECT status INTO enrollment_state FROM v3.enrollments WHERE id=NEW.enrollment_id AND class_id=NEW.class_id AND user_id=NEW.owner_user_id;
 SELECT ended_at INTO episode_closed FROM v3.episodes WHERE id=NEW.episode_id AND enrollment_id=NEW.enrollment_id;
 IF revision_state IS DISTINCT FROM 'PUBLISHED' OR activity_kind IS DISTINCT FROM 'ASSIGNMENT' OR enrollment_state IS DISTINCT FROM 'ACTIVE' OR episode_closed IS NOT NULL OR NEW.created_by IS DISTINCT FROM NEW.owner_user_id THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_CONTEXT_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_submission_foundation_insert() FROM PUBLIC;
CREATE TRIGGER submission_foundation_insert_guard BEFORE INSERT ON v3.submission_aggregates FOR EACH ROW EXECUTE FUNCTION v3.guard_submission_foundation_insert();`

const submissionRevisionLifecycleSchema = `ALTER TABLE v3.submission_attempts ADD COLUMN locked_at timestamptz;
ALTER TABLE v3.submission_attempts ADD COLUMN returned_at timestamptz;
ALTER TABLE v3.submission_attempts ADD COLUMN returned_by text REFERENCES v3.users(id);
ALTER TABLE v3.submission_attempts ADD CONSTRAINT submission_attempt_state_time_ck CHECK((state='DRAFT' AND locked_at IS NULL AND returned_at IS NULL AND returned_by IS NULL) OR (state='LOCKED' AND locked_at IS NOT NULL AND returned_at IS NULL AND returned_by IS NULL) OR (state='RETURNED' AND locked_at IS NOT NULL AND returned_at IS NOT NULL AND returned_by IS NOT NULL));
CREATE FUNCTION v3.guard_submission_aggregate_update() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.activity_id IS DISTINCT FROM OLD.activity_id OR NEW.assignment_revision_id IS DISTINCT FROM OLD.assignment_revision_id OR NEW.enrollment_id IS DISTINCT FROM OLD.enrollment_id OR NEW.episode_id IS DISTINCT FROM OLD.episode_id OR NEW.owner_user_id IS DISTINCT FROM OLD.owner_user_id OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.current_attempt_no<>OLD.current_attempt_no+1 OR NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_AGGREGATE_UPDATE_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_submission_aggregate_update() FROM PUBLIC;
CREATE TRIGGER submission_aggregate_update_guard BEFORE UPDATE ON v3.submission_aggregates FOR EACH ROW EXECUTE FUNCTION v3.guard_submission_aggregate_update();
CREATE FUNCTION v3.guard_submission_attempt_update() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.aggregate_id IS DISTINCT FROM OLD.aggregate_id OR NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.activity_id IS DISTINCT FROM OLD.activity_id OR NEW.assignment_revision_id IS DISTINCT FROM OLD.assignment_revision_id OR NEW.attempt_no IS DISTINCT FROM OLD.attempt_no OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_ATTEMPT_IDENTITY_IMMUTABLE';
 END IF;
 IF NOT ((OLD.state='DRAFT' AND NEW.state='LOCKED' AND NEW.locked_at IS NOT NULL AND NEW.returned_at IS NULL AND NEW.returned_by IS NULL) OR (OLD.state='LOCKED' AND NEW.state='RETURNED' AND NEW.locked_at IS NOT NULL AND NEW.returned_at IS NOT NULL AND NEW.returned_by IS NOT NULL)) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_ATTEMPT_TRANSITION_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_submission_attempt_update() FROM PUBLIC;
CREATE TRIGGER submission_attempt_update_guard BEFORE UPDATE ON v3.submission_attempts FOR EACH ROW EXECUTE FUNCTION v3.guard_submission_attempt_update();`

const submissionHandinSchema = `ALTER TABLE v3.submission_revisions ADD CONSTRAINT submission_revision_receipt_identity_uniq UNIQUE(id,class_id,attempt_id);
CREATE TABLE v3.submission_receipts(id text PRIMARY KEY,class_id text NOT NULL,aggregate_id text NOT NULL,attempt_id text NOT NULL,revision_id text NOT NULL,submitted_by text NOT NULL REFERENCES v3.users(id),submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),due_at timestamptz,cutoff_at timestamptz,timezone text NOT NULL DEFAULT 'UTC',late boolean NOT NULL DEFAULT false,late_policy text NOT NULL DEFAULT 'REJECT_AFTER_CUTOFF' CHECK(late_policy IN ('REJECT_AFTER_CUTOFF','ALLOW_MARKED_LATE')),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(attempt_id,class_id,aggregate_id) REFERENCES v3.submission_attempts(id,class_id,aggregate_id),FOREIGN KEY(revision_id,class_id,attempt_id) REFERENCES v3.submission_revisions(id,class_id,attempt_id),UNIQUE(attempt_id),UNIQUE(id,class_id));
CREATE INDEX submission_receipts_revision_idx ON v3.submission_receipts(revision_id,class_id,attempt_id);
CREATE INDEX submission_receipts_aggregate_idx ON v3.submission_receipts(aggregate_id,submitted_at,id);
CREATE FUNCTION v3.guard_submission_receipt() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
DECLARE current_state text; current_revision text; assignment_revision text; owner_id text; content_state text; available_time timestamptz; latest_revision text; policy text; zone text; due_time timestamptz; cutoff_time timestamptz;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_RECEIPT_IMMUTABLE'; END IF;
 SELECT a.state,a.assignment_revision_id,s.owner_user_id INTO current_state,assignment_revision,owner_id FROM v3.submission_attempts a JOIN v3.submission_aggregates s ON s.id=a.aggregate_id AND s.class_id=a.class_id WHERE a.id=NEW.attempt_id AND a.class_id=NEW.class_id AND a.aggregate_id=NEW.aggregate_id FOR SHARE OF a,s;
 SELECT r.id INTO latest_revision FROM v3.submission_revisions r WHERE r.attempt_id=NEW.attempt_id ORDER BY r.revision_no DESC LIMIT 1;
 SELECT c.state,c.available_from,c.timezone,c.due_at,c.cutoff_at,ar.late_policy INTO content_state,available_time,zone,due_time,cutoff_time,policy FROM v3.content_revisions c JOIN v3.assignment_revision_rules ar ON ar.revision_id=c.id AND ar.class_id=c.class_id AND ar.activity_id=c.activity_id WHERE c.id=assignment_revision;
 NEW.submitted_at:=clock_timestamp();
 IF current_state IS DISTINCT FROM 'LOCKED' OR latest_revision IS DISTINCT FROM NEW.revision_id OR owner_id IS DISTINCT FROM NEW.submitted_by OR content_state IS DISTINCT FROM 'PUBLISHED' OR (available_time IS NOT NULL AND NEW.submitted_at<available_time) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_HANDIN_CONTEXT_REJECTED';
 END IF;
 IF cutoff_time IS NOT NULL AND NEW.submitted_at>cutoff_time THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='SUBMISSION_CUTOFF_PASSED'; END IF;
 NEW.timezone:=zone; NEW.due_at:=due_time; NEW.cutoff_at:=cutoff_time; NEW.late_policy:=policy; NEW.late:=(due_time IS NOT NULL AND NEW.submitted_at>due_time);
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_submission_receipt() FROM PUBLIC;
CREATE TRIGGER submission_receipt_insert_guard BEFORE INSERT ON v3.submission_receipts FOR EACH ROW EXECUTE FUNCTION v3.guard_submission_receipt();
CREATE TRIGGER submission_receipt_immutable_guard BEFORE UPDATE ON v3.submission_receipts FOR EACH ROW EXECUTE FUNCTION v3.guard_submission_receipt();`

const submissionChronologySchema = `CREATE TABLE v3.legacy_submission_evidence(source_instance_id text NOT NULL CHECK(length(source_instance_id) BETWEEN 1 AND 128),source_table text NOT NULL CHECK(source_table='submissions'),source_pk text NOT NULL CHECK(length(source_pk) BETWEEN 1 AND 128),source_version text NOT NULL CHECK(length(source_version) BETWEEN 1 AND 128),raw_payload jsonb NOT NULL,raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),raw_time_text text,chronology_status text NOT NULL DEFAULT 'UNKNOWN' CHECK(chronology_status='UNKNOWN'),landed_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(source_instance_id,source_table,source_pk,source_version));
CREATE INDEX legacy_submission_evidence_landed_idx ON v3.legacy_submission_evidence(landed_at,source_instance_id,source_pk);
CREATE FUNCTION v3.guard_legacy_submission_evidence() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='LEGACY_SUBMISSION_EVIDENCE_IMMUTABLE';
END$$;
REVOKE ALL ON FUNCTION v3.guard_legacy_submission_evidence() FROM PUBLIC;
CREATE TRIGGER legacy_submission_evidence_immutable_guard BEFORE UPDATE ON v3.legacy_submission_evidence FOR EACH ROW EXECUTE FUNCTION v3.guard_legacy_submission_evidence();`

// Catalogue is the single immutable source for the currently implemented v3
// installments. The reusable runner consumes a copy; callers cannot add SQL.
func Catalogue() []runner.Migration {
	return []runner.Migration{
		{Version: "class-slice-1", SQL: schema},
		{Version: "cp11-1", SQL: identitySchema, Requires: []string{"class-slice-1"}},
		{Version: "cp12-1", SQL: accessSchema, Requires: []string{"cp11-1"}},
		{Version: "m03-cp05-1", SQL: outboxWorkerSchema, Requires: []string{"class-slice-1"}},
		{Version: "m05-cp03-1", SQL: classGroupingSchema, Requires: []string{"cp12-1"}},
		{Version: "m05-cp04-1", SQL: participationProvenanceSchema, Requires: []string{"m05-cp03-1"}},
		{Version: "m06-cp01-1", SQL: privateFileAssetSchema, Requires: []string{"m05-cp04-1"}},
		{Version: "m06-cp02-1", SQL: privateFileDeliverySchema, Requires: []string{"m06-cp01-1"}},
		{Version: "m07-cp01-1", SQL: contentStructureSchema, Requires: []string{"m06-cp02-1"}},
		{Version: "m07-cp02-1", SQL: contentSafetySchema, Requires: []string{"m07-cp01-1"}},
		{Version: "m07-cp03-1", SQL: contentRevisionContractSchema, Requires: []string{"m07-cp02-1"}},
		{Version: "m08-cp01-1", SQL: submissionFoundationSchema, Requires: []string{"m07-cp03-1"}},
		{Version: "m08-cp02-1", SQL: submissionRevisionLifecycleSchema, Requires: []string{"m08-cp01-1"}},
		{Version: "m08-cp03-1", SQL: submissionHandinSchema, Requires: []string{"m08-cp02-1"}},
		{Version: "m08-cp04-1", SQL: submissionChronologySchema, Requires: []string{"m08-cp03-1"}},
		{Version: "m11-cp01-1", SQL: meetingCalendarSchema, Requires: []string{"m05-cp04-1", "m06-cp02-1"}},
		{Version: "m12-cp01-1", SQL: logbookPeriodSchema, Requires: []string{"m05-cp04-1", "m06-cp02-1"}},
		{Version: "m12-cp02-1", SQL: logbookEntrySchema, Requires: []string{"m12-cp01-1"}},
		{Version: "m12-cp03-1", SQL: logbookReviewSchema, Requires: []string{"m12-cp02-1"}},
		{Version: "m13-cp01-1", SQL: durableDeliverySchema, Requires: []string{"m12-cp03-1"}},
		{Version: "m13-cp02-1", SQL: notificationSafetySchema, Requires: []string{"m13-cp01-1"}},
		// M09 is appended in implementation chronology. Its business dependency
		// is M08; M13 is the immutable migration-history predecessor only.
		{Version: "m09-cp01-1", SQL: gradeDefinitionSchema, Requires: []string{"m08-cp04-1", "m13-cp02-1"}},
		{Version: "m09-cp02-1", SQL: scopedGradeSchema, Requires: []string{"m09-cp01-1"}},
	}
}

// InstallContent applies only the closed M07-CP01 structure installment to a
// marker-bound synthetic database after a fresh backup and isolated restore.
func InstallContent(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	ddl, dependencyVersion, dependencyChecksum, snapshot := "", "", "", ""
	switch checkpoint {
	case "m07-cp01":
		ddl, dependencyVersion, dependencyChecksum, snapshot = contentStructureSchema, "m06-cp02-1", Digest([]byte(privateFileDeliverySchema)), "pre_m07_cp01.dump"
	case "m07-cp02":
		ddl, dependencyVersion, dependencyChecksum, snapshot = contentSafetySchema, "m07-cp01-1", Digest([]byte(contentStructureSchema)), "pre_m07_cp02.dump"
	case "m07-cp03":
		ddl, dependencyVersion, dependencyChecksum, snapshot = contentRevisionContractSchema, "m07-cp02-1", Digest([]byte(contentSafetySchema)), "pre_m07_cp03.dump"
	default:
		return errors.New("UNKNOWN_INSTALLMENT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	proofPath := filepath.Join(root, checkpoint+"-proof.json")
	info, err := os.Lstat(proofPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return errors.New("INVALID_INSTALLMENT_PROOF")
	}
	if err = validateArtifact(proof, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now().UTC(), snapshot); err != nil {
		return err
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.User != "qa_fixture" || config.Database != "qa_cp05" {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	if err = Identity(ctx, pool); err != nil {
		return err
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var dependency string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", dependencyVersion).Scan(&dependency); err != nil {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		if dependency != dependencyChecksum {
			return errors.New("MIGRATION_CHECKSUM_CONFLICT")
		}
		var existing string
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", checkpoint+"-1").Scan(&existing)
		if err == nil {
			if existing != Digest([]byte(ddl)) {
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
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", Digest([]byte(ddl)), proof.ArchiveHash)
		return err
	})
}

// InstallSubmissions applies one closed M08 installment only after a fresh,
// checksum-bound dump has also been restored into an isolated fixture.
func InstallSubmissions(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	ddl, dependencyVersion, dependencyChecksum, snapshot := "", "", "", ""
	switch checkpoint {
	case "m08-cp01":
		ddl, dependencyVersion, dependencyChecksum, snapshot = submissionFoundationSchema, "m07-cp03-1", Digest([]byte(contentRevisionContractSchema)), "pre_m08_cp01.dump"
	case "m08-cp02":
		ddl, dependencyVersion, dependencyChecksum, snapshot = submissionRevisionLifecycleSchema, "m08-cp01-1", Digest([]byte(submissionFoundationSchema)), "pre_m08_cp02.dump"
	case "m08-cp03":
		ddl, dependencyVersion, dependencyChecksum, snapshot = submissionHandinSchema, "m08-cp02-1", Digest([]byte(submissionRevisionLifecycleSchema)), "pre_m08_cp03.dump"
	case "m08-cp04":
		ddl, dependencyVersion, dependencyChecksum, snapshot = submissionChronologySchema, "m08-cp03-1", Digest([]byte(submissionHandinSchema)), "pre_m08_cp04.dump"
	default:
		return errors.New("UNKNOWN_INSTALLMENT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	proofPath := filepath.Join(root, checkpoint+"-proof.json")
	info, err := os.Lstat(proofPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return errors.New("INVALID_INSTALLMENT_PROOF")
	}
	if err = validateArtifact(proof, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now().UTC(), snapshot); err != nil {
		return err
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.User != "qa_fixture" || config.Database != "qa_cp05" {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	if err = Identity(ctx, pool); err != nil {
		return err
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var dependency string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", dependencyVersion).Scan(&dependency); err != nil || dependency != dependencyChecksum {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		var existing string
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", checkpoint+"-1").Scan(&existing)
		if err == nil {
			if existing != Digest([]byte(ddl)) {
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
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", Digest([]byte(ddl)), proof.ArchiveHash)
		return err
	})
}

// Additional closed operator installments; original class-slice-1 is untouched.
// Receipt is separately bound to a fresh snapshot made after prior checkpoints.
func InstallLifecycle(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	ddl := ""
	switch checkpoint {
	case "cp11":
		ddl = identitySchema
	case "cp12":
		ddl = accessSchema
	default:
		return errors.New("UNKNOWN_INSTALLMENT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	info, statErr := os.Lstat(filepath.Join(root, checkpoint+"-proof.json"))
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	proofPath := filepath.Join(root, checkpoint+"-proof.json")
	info, err := os.Lstat(proofPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return err
	}
	var p Proof
	if json.Unmarshal(data, &p) != nil {
		return errors.New("INVALID_INSTALLMENT_PROOF")
	}
	if err = validateArtifact(p, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now(), "pre_"+checkpoint+".dump"); err != nil {
		return err
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.User != "qa_fixture" || c.Database != "qa_cp05" {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	if err = Identity(ctx, pool); err != nil {
		return err
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var old string
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", checkpoint+"-1").Scan(&old)
		if err == nil {
			if old != Digest([]byte(ddl)) {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if checkpoint == "cp12" {
			if err = tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='cp11-1'").Scan(&old); err != nil || old != Digest([]byte(identitySchema)) {
				return errors.New("CP11_REQUIRED")
			}
		}
		if _, err = tx.Exec(ctx, ddl, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", Digest([]byte(ddl)), p.ArchiveHash)
		return err
	})
}

// InstallParticipation replays only closed M05 installments against the exact
// marker-bound synthetic database. Its fresh snapshot receipt is required even
// when the catalogue runner already applied the installment earlier in a full
// forward batch.
func InstallParticipation(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	ddl := ""
	requires := ""
	requiredChecksum := ""
	snapshot := ""
	switch checkpoint {
	case "m05-cp03":
		ddl = classGroupingSchema
		requires = "cp12-1"
		requiredChecksum = Digest([]byte(accessSchema))
		snapshot = "pre_m05_cp03"
	case "m05-cp04":
		ddl = participationProvenanceSchema
		requires = "m05-cp03-1"
		requiredChecksum = Digest([]byte(classGroupingSchema))
		snapshot = "pre_m05_cp04"
	default:
		return errors.New("UNKNOWN_INSTALLMENT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	data, err := os.ReadFile(filepath.Join(root, checkpoint+"-proof.json"))
	if err != nil {
		return errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return errors.New("INVALID_INSTALLMENT_PROOF")
	}
	if err = validateArtifact(proof, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now().UTC(), snapshot+".dump"); err != nil {
		return err
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.User != "qa_fixture" || config.Database != "qa_cp05" {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	if err = Identity(ctx, pool); err != nil {
		return err
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var dependency string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", requires).Scan(&dependency); err != nil {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		if dependency != requiredChecksum {
			return errors.New("MIGRATION_CHECKSUM_CONFLICT")
		}
		var existing string
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", checkpoint+"-1").Scan(&existing)
		if err == nil {
			if existing != Digest([]byte(ddl)) {
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
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", Digest([]byte(ddl)), proof.ArchiveHash)
		return err
	})
}

// InstallFiles is an additive, marker-bound synthetic installer. Each M06
// installment requires its own fresh dump plus actual isolated restore proof.
func InstallFiles(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	ddl, requires, requiredChecksum, snapshot := "", "", "", ""
	switch checkpoint {
	case "m06-cp01":
		ddl, requires, requiredChecksum, snapshot = privateFileAssetSchema, "m05-cp04-1", Digest([]byte(participationProvenanceSchema)), "pre_m06_cp01"
	case "m06-cp02":
		ddl, requires, requiredChecksum, snapshot = privateFileDeliverySchema, "m06-cp01-1", Digest([]byte(privateFileAssetSchema)), "pre_m06_cp02"
	default:
		return errors.New("UNKNOWN_INSTALLMENT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	proofPath := filepath.Join(root, checkpoint+"-proof.json")
	info, err := os.Lstat(proofPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return errors.New("INVALID_INSTALLMENT_PROOF")
	}
	if err = validateArtifact(proof, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now().UTC(), snapshot+".dump"); err != nil {
		return err
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.User != "qa_fixture" || config.Database != "qa_cp05" {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	if err = Identity(ctx, pool); err != nil {
		return err
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var dependency string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", requires).Scan(&dependency); err != nil {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		if dependency != requiredChecksum {
			return errors.New("MIGRATION_CHECKSUM_CONFLICT")
		}
		var existing string
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", checkpoint+"-1").Scan(&existing)
		if err == nil {
			if existing != Digest([]byte(ddl)) {
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
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", Digest([]byte(ddl)), proof.ArchiveHash)
		return err
	})
}

// Identity writer is separate from Class runtime, marker-bound and synthetic.
// The role is provisioned by operator tests, never by API startup.
func IdentityRuntime(ctx context.Context) (*pgxpool.Pool, error) {
	c, err := Config(true)
	if err != nil {
		return nil, err
	}
	c.User = "qa_identity"
	c.Password = os.Getenv("QA_IDENTITY_PASSWORD")
	if len(c.Password) < 32 {
		return nil, errors.New("IDENTITY_PASSWORD_REQUIRED")
	}
	pc, err := pgxpool.ParseConfig("postgres://qa_identity@127.0.0.1/qa_cp05?sslmode=disable")
	if err != nil {
		return nil, err
	}
	pc.ConnConfig = c
	pc.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "5000"
	pc.ConnConfig.RuntimeParams["application_name"] = "lms-v3-identity-lab"
	pc.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	var unsafe bool
	err = pool.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreatedb OR r.rolcreaterole OR has_schema_privilege(current_user,'public','CREATE') OR has_schema_privilege(current_user,'v3identity','CREATE') OR has_table_privilege(current_user,'v3.users','INSERT,UPDATE,DELETE,TRUNCATE') OR has_table_privilege(current_user,'v3.classes','INSERT,UPDATE,DELETE,TRUNCATE') OR has_table_privilege(current_user,'v3.audit','UPDATE,DELETE,TRUNCATE') OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN ('v3','v3identity') AND c.relkind='r' AND (c.relowner=r.oid OR has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE'))) FROM pg_roles r WHERE rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		pool.Close()
		return nil, errors.New("IDENTITY_PRIVILEGES_REJECTED")
	}
	if err = Identity(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// WorkerRuntime is an exact synthetic boundary for the queue consumer. It may
// update leases and append attempt history, but cannot mutate domain/audit data,
// delete retained rows, own objects, or execute DDL.
func WorkerRuntime(ctx context.Context) (*pgxpool.Pool, error) {
	c, err := Config(true)
	if err != nil {
		return nil, err
	}
	c.User = "qa_worker"
	c.Password = os.Getenv("QA_WORKER_PASSWORD")
	if len(c.Password) < 32 {
		return nil, errors.New("WORKER_PASSWORD_REQUIRED")
	}
	pool, err := database.OpenSyntheticWorker(ctx, c, os.Getenv("QA_PG_ACK"))
	if err != nil {
		return nil, err
	}
	var unsafe bool
	err = pool.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreatedb OR r.rolcreaterole
 OR has_schema_privilege(current_user,'public','CREATE')
 OR has_schema_privilege(current_user,'v3','CREATE')
 OR has_schema_privilege(current_user,'v3ops','CREATE')
 OR has_table_privilege(current_user,'v3.users','INSERT,UPDATE,DELETE,TRUNCATE')
 OR has_table_privilege(current_user,'v3.classes','INSERT,UPDATE,DELETE,TRUNCATE')
 OR has_table_privilege(current_user,'v3.audit','INSERT,UPDATE,DELETE,TRUNCATE')
	OR has_table_privilege(current_user,'v3.outbox','INSERT,DELETE,TRUNCATE')
	OR has_table_privilege(current_user,'v3ops.outbox_attempts','UPDATE,DELETE,TRUNCATE')
	OR has_table_privilege(current_user,'v3ops.outbox_failures','UPDATE,DELETE,TRUNCATE')
	OR coalesce(has_table_privilege(current_user,to_regclass('v3ops.outbox_provider_receipts'),'UPDATE,DELETE,TRUNCATE'),false)
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname IN ('v3','v3ops','v3identity') AND c.relkind='r' AND c.relowner=r.oid)
 FROM pg_roles r WHERE rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		pool.Close()
		return nil, errors.New("WORKER_PRIVILEGES_REJECTED")
	}
	if err = Identity(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
