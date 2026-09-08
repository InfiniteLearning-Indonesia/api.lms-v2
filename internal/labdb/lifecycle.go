package labdb

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/migration/backup"
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
	data, err := os.ReadFile(filepath.Join(root, checkpoint+"-proof.json"))
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
