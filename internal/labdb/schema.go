// Package labdb is an operator/test harness boundary, not production migration.
// It accepts only the fresh marker-bound loopback cluster from qa.postgres_lab.
package labdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"strconv"
	"time"
)

const schema = `CREATE SCHEMA v3;
CREATE SCHEMA v3ops;
CREATE TABLE v3ops.migrations(version text PRIMARY KEY, checksum text NOT NULL, backup_hash text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE v3.users(id text PRIMARY KEY, active boolean NOT NULL DEFAULT true, site_admin boolean NOT NULL DEFAULT false);
CREATE TABLE v3.sessions(token_hash text PRIMARY KEY, user_id text NOT NULL REFERENCES v3.users(id), expires_at timestamptz NOT NULL, revoked_at timestamptz);
CREATE INDEX sessions_user_idx ON v3.sessions(user_id);
CREATE TABLE v3.classes(id text PRIMARY KEY, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200), program_label text NOT NULL DEFAULT '', cohort_label text NOT NULL DEFAULT '', state text NOT NULL DEFAULT 'DRAFT' CHECK(state IN ('DRAFT','PUBLISHED','ARCHIVED')), version bigint NOT NULL DEFAULT 1);
CREATE TABLE v3.enrollments(id text PRIMARY KEY, class_id text NOT NULL REFERENCES v3.classes(id), user_id text NOT NULL REFERENCES v3.users(id), status text NOT NULL CHECK(status IN ('ACTIVE','ENDED')), UNIQUE(class_id,user_id), UNIQUE(id,class_id));
CREATE INDEX enrollments_user_idx ON v3.enrollments(user_id);
CREATE TABLE v3.episodes(id text PRIMARY KEY, enrollment_id text NOT NULL REFERENCES v3.enrollments(id), opened_at timestamptz NOT NULL DEFAULT now(), ended_at timestamptz);
CREATE UNIQUE INDEX episodes_open_idx ON v3.episodes(enrollment_id) WHERE ended_at IS NULL;
CREATE TABLE v3.roles(id text PRIMARY KEY, enrollment_id text NOT NULL, class_id text NOT NULL, role text NOT NULL CHECK(role IN ('teacher','student')), revoked_at timestamptz, FOREIGN KEY(enrollment_id,class_id) REFERENCES v3.enrollments(id,class_id));
CREATE UNIQUE INDEX roles_active_idx ON v3.roles(enrollment_id,role) WHERE revoked_at IS NULL;
CREATE INDEX roles_class_idx ON v3.roles(class_id);
CREATE TABLE v3.audit(id text PRIMARY KEY, actor_id text NOT NULL REFERENCES v3.users(id), class_id text REFERENCES v3.classes(id), action text NOT NULL, request_id text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX audit_actor_idx ON v3.audit(actor_id);
CREATE INDEX audit_class_idx ON v3.audit(class_id);
CREATE TABLE v3.outbox(event_id text PRIMARY KEY REFERENCES v3.audit(id), status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','LEASED','SENT')), lease_id text, lease_until timestamptz, attempts integer NOT NULL DEFAULT 0, next_attempt timestamptz NOT NULL DEFAULT now());
CREATE INDEX outbox_pending_idx ON v3.outbox(next_attempt,event_id) WHERE status <> 'SENT';
CREATE TABLE v3.commands(actor_id text NOT NULL REFERENCES v3.users(id), key text NOT NULL, request_hash text NOT NULL, response jsonb NOT NULL, PRIMARY KEY(actor_id,key));`

func Digest(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func Checksum() string          { return Digest([]byte(schema)) }
func Target(port, marker string) string {
	return Digest([]byte("127.0.0.1\x00" + port + "\x00qa_cp05\x00" + marker))
}

type Proof struct {
	RunID       string    `json:"run_id"`
	Target      string    `json:"target"`
	CreatedAt   time.Time `json:"created_at"`
	ArchiveHash string    `json:"archive_hash"`
	Restored    bool      `json:"restored"`
}

func ValidateProof(p Proof, root, target string, now time.Time) error {
	return validateArtifact(p, root, target, now, "pre_v3.dump")
}

// MigrationGate re-verifies the retained backup bytes before the reusable
// runner sees a receipt. It is intentionally restricted to the synthetic lab.
func MigrationGate(now time.Time) (runner.Gate, error) {
	root := os.Getenv("QA_LAB_DIR")
	info, err := os.Lstat(filepath.Join(root, "pre-v3-proof.json"))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return runner.Gate{}, errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(filepath.Join(root, "pre-v3-proof.json"))
	if err != nil {
		return runner.Gate{}, errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return runner.Gate{}, errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	target := Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER"))
	if err = ValidateProof(proof, root, target, now.UTC()); err != nil {
		return runner.Gate{}, err
	}
	return runner.Gate{RunID: proof.RunID, TargetFingerprint: target, ArchiveSHA256: proof.ArchiveHash, RestoreVerified: true, VerifiedAt: proof.CreatedAt}, nil
}
func validateArtifact(p Proof, root, target string, now time.Time, filename string) error {
	if !p.Restored || p.Target != target || p.RunID != filepath.Base(root) || now.Before(p.CreatedAt) || now.Sub(p.CreatedAt) > time.Hour || !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	info, err := os.Lstat(filepath.Join(root, filename))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return errors.New("LAB_BACKUP_FILE_REJECTED")
	}
	data, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil || Digest(data) != p.ArchiveHash {
		return errors.New("LAB_BACKUP_HASH_REJECTED")
	}
	return nil
}
func Config(runtime bool) (*pgx.ConnConfig, error) {
	port := os.Getenv("QA_PG_PORT")
	n, err := strconv.Atoi(port)
	marker := os.Getenv("QA_PG_MARKER")
	if err != nil || n < 1024 || n > 65535 || os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" || len(marker) != 64 {
		return nil, errors.New("LAB_ACK_REQUIRED")
	}
	user := "qa_fixture"
	password := os.Getenv("QA_PG_PASSWORD")
	if runtime {
		user = "qa_runtime"
		password = os.Getenv("QA_RUNTIME_PASSWORD")
	}
	if len(password) < 32 {
		return nil, errors.New("LAB_PASSWORD_REQUIRED")
	}
	cfg, err := pgx.ParseConfig("postgres://" + user + "@127.0.0.1:" + port + "/qa_cp05?sslmode=disable")
	if err != nil {
		return nil, errors.New("LAB_CONFIG_REJECTED")
	}
	cfg.Password = password
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{"search_path": "pg_catalog", "statement_timeout": "5000", "lock_timeout": "1000"}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.ConnectTimeout = 3 * time.Second
	return cfg, nil
}
func Owner(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := Config(false)
	if err != nil {
		return nil, err
	}
	pc, err := pgxpool.ParseConfig("postgres://qa_fixture@127.0.0.1/qa_cp05?sslmode=disable")
	if err != nil {
		return nil, err
	}
	pc.ConnConfig = cfg
	pc.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if err = Identity(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
func Runtime(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := Config(true)
	if err != nil {
		return nil, err
	}
	pool, err := database.OpenSynthetic(ctx, cfg, os.Getenv("QA_PG_ACK"))
	if err != nil {
		return nil, err
	}
	if err = Identity(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
func Identity(ctx context.Context, pool *pgxpool.Pool) error {
	var marker string
	if pool.QueryRow(ctx, "SELECT value FROM qa_cp05.marker").Scan(&marker) != nil || marker != os.Getenv("QA_PG_MARKER") {
		return errors.New("LAB_IDENTITY_REJECTED")
	}
	return nil
}

// Install is a closed, fixed migration: no caller SQL, dotenv, arbitrary DSN or
// shared target. The trusted test harness supplies an actual restore receipt.
func Install(ctx context.Context, pool *pgxpool.Pool, expected string) error {
	if expected != Checksum() {
		return errors.New("MIGRATION_CHECKSUM_CONFLICT")
	}
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	info, statErr := os.Lstat(filepath.Join(root, "pre-v3-proof.json"))
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(filepath.Join(root, "pre-v3-proof.json"))
	if err != nil {
		return errors.New("LAB_BACKUP_RECEIPT_REQUIRED")
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	if err = ValidateProof(proof, root, Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), time.Now().UTC()); err != nil {
		return err
	}
	cfg := pool.Config().ConnConfig
	if cfg.Host != "127.0.0.1" || cfg.Database != "qa_cp05" || cfg.User != "qa_fixture" {
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
		var installed bool
		if err := tx.QueryRow(ctx, "SELECT to_regclass('v3ops.migrations') IS NOT NULL").Scan(&installed); err != nil {
			return err
		}
		if installed {
			var checksum string
			if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='class-slice-1'").Scan(&checksum); err != nil {
				return err
			}
			if checksum != expected {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		// Simple protocol supports the fixed multi-statement DDL only here.
		if _, err := tx.Exec(ctx, schema, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES('class-slice-1',$1,$2)", expected, proof.ArchiveHash)
		return err
	})
}
