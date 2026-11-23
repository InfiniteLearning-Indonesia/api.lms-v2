package legacycore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/labdb"
)

const TargetDB = "legacy_core_v3"
const TargetUser = "lms_core_import"

var ErrTarget = errors.New("CORE_LOCAL_TARGET_REJECTED")
var ErrBackup = errors.New("CORE_TARGET_BACKUP_REJECTED")
var ErrWrite = errors.New("CORE_TRANSACTION_FAILED")
var ErrReconcile = errors.New("CORE_TARGET_RECONCILIATION_FAILED")
var hex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)

type targetProof struct {
	Port       int       `json:"port"`
	Marker     string    `json:"marker"`
	BackupHash string    `json:"backup_sha256"`
	VerifiedAt time.Time `json:"verified_at"`
	Restored   bool      `json:"restored"`
}

func readBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, ErrBackup
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrBackup
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrBackup
	}
	return data, nil
}

func proof(path string) (targetProof, string, error) {
	var p targetProof
	if !filepath.IsAbs(path) || filepath.Base(path) != "target-proof.json" ||
		!strings.Contains(filepath.Base(filepath.Dir(path)), "-go-core-rehearsal-") {
		return p, "", ErrTarget
	}
	root := filepath.Dir(path)
	verifiedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !samePath(verifiedRoot, root) {
		return p, "", ErrTarget
	}
	raw, err := readBounded(path, 65536)
	if err != nil || json.Unmarshal(raw, &p) != nil || !p.Restored || !hex64.MatchString(p.Marker) ||
		!hex64.MatchString(p.BackupHash) || p.Port < 1024 || p.Port > 65535 ||
		time.Since(p.VerifiedAt) < 0 || time.Since(p.VerifiedAt) > time.Hour {
		return p, "", ErrBackup
	}
	data, err := readBounded(filepath.Join(root, "prestate", "empty.dump"), 8<<20)
	if err != nil || !strings.HasPrefix(string(data), "PGDMP") || digest(data) != p.BackupHash {
		return p, "", ErrBackup
	}
	return p, filepath.Join(root, "postgres"), nil
}

func samePath(a, b string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

const journalDDL = `CREATE SCHEMA core_rehearsal;
CREATE TABLE core_rehearsal.run(snapshot text PRIMARY KEY, mapping text NOT NULL, plan_hash text NOT NULL, backup_hash text NOT NULL);
CREATE TABLE core_rehearsal.origin(source_table text NOT NULL, legacy_id text NOT NULL, disposition text NOT NULL,
 reason text NOT NULL, target_id text NOT NULL, original_time text NOT NULL, PRIMARY KEY(source_table,legacy_id));`

// Apply checks a marker-bound loopback target and verified backup, then imports
// the whole core and its provenance in one transaction.
func Apply(ctx context.Context, plan Plan, proofPath, password string, rollback bool) (string, error) {
	p, directory, err := proof(proofPath)
	if err != nil {
		return "", err
	}
	if !hex64.MatchString(password) {
		return "", ErrTarget
	}
	cfg, err := pgx.ParseConfig("postgres://" + TargetUser + "@127.0.0.1:" + strconv.Itoa(p.Port) + "/" + TargetDB + "?sslmode=disable")
	if err != nil {
		return "", ErrTarget
	}
	// Keep inherited libpq connection and runtime overrides outside the target.
	cfg.Host, cfg.Port, cfg.User, cfg.Database = "127.0.0.1", uint16(p.Port), TargetUser, TargetDB
	cfg.Password, cfg.TLSConfig, cfg.Fallbacks = password, nil, nil
	cfg.ConnectTimeout = 5 * time.Second
	cfg.RuntimeParams = map[string]string{"search_path": "pg_catalog", "lock_timeout": "2s", "statement_timeout": "10s"}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return "", ErrTarget
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		return "", ErrTarget
	}
	defer tx.Rollback(context.Background())
	var host, database, user, marker, actualDirectory string
	var port, version int
	var privileged bool
	err = tx.QueryRow(ctx, `SELECT pg_catalog.host(inet_server_addr()), inet_server_port(), current_database(), current_user,
 current_setting('server_version_num')::int, g.marker, g.data_directory,
 r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolbypassrls
 FROM rehearsal_guard.target g JOIN pg_catalog.pg_roles r ON r.rolname=current_user`).Scan(
		&host, &port, &database, &user, &version, &marker, &actualDirectory, &privileged)
	if err != nil || host != "127.0.0.1" || port != p.Port || database != TargetDB || user != TargetUser ||
		version < 180000 || version >= 190000 || marker != p.Marker || !samePath(directory, actualDirectory) || privileged {
		return "", ErrTarget
	}
	var locked, existing bool
	if tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(781415)").Scan(&locked) != nil || !locked ||
		tx.QueryRow(ctx, "SELECT to_regclass('core_rehearsal.run') IS NOT NULL").Scan(&existing) != nil {
		return "", ErrTarget
	}
	status := "PASS_LOCAL_CORE_IMPORTED"
	if existing {
		if rollback {
			return "", ErrTarget
		}
		var hash, backup, mapping string
		if tx.QueryRow(ctx, "SELECT plan_hash,backup_hash,mapping FROM core_rehearsal.run WHERE snapshot=$1", Snapshot).
			Scan(&hash, &backup, &mapping) != nil || hash != plan.Digest() || backup != p.BackupHash || mapping != Mapping {
			return "", ErrReconcile
		}
		status = "PASS_LOCAL_CORE_REPLAY"
	} else {
		var other bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname NOT IN ('pg_catalog','information_schema','public','rehearsal_guard') AND nspname NOT LIKE 'pg_%')
   OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public')`).Scan(&other) != nil || other {
			return "", ErrTarget
		}
		if _, err = tx.Exec(ctx, labdb.CoreDDL()+"\n"+journalDDL, pgx.QueryExecModeSimpleProtocol); err != nil {
			return "", ErrWrite
		}
		if err = writePlan(ctx, tx, plan, p.BackupHash); err != nil {
			return "", ErrWrite
		}
	}
	if err = reconcile(ctx, tx, plan); err != nil {
		return "", err
	}
	if rollback {
		var empty bool
		if tx.Rollback(ctx) != nil || conn.QueryRow(ctx, `SELECT to_regnamespace('v3') IS NULL
			AND to_regnamespace('v3ops') IS NULL AND to_regnamespace('core_rehearsal') IS NULL`).Scan(&empty) != nil || !empty {
			return "", ErrReconcile
		}
		return "PASS_LOCAL_CORE_ROLLBACK", nil
	}
	if err = tx.Commit(ctx); err != nil {
		return "", ErrWrite
	}
	return status, nil
}

func writePlan(ctx context.Context, tx pgx.Tx, plan Plan, backup string) error {
	for _, user := range plan.Users {
		if _, err := tx.Exec(ctx, "INSERT INTO v3.users(id,active,site_admin) VALUES($1,$2,false)", user.ID, user.Status == "active"); err != nil {
			return err
		}
	}
	for _, class := range plan.Classes {
		if _, err := tx.Exec(ctx, "INSERT INTO v3.classes(id,name,program_label,cohort_label) VALUES($1,$2,$3,$4)", class.ID, class.Name, class.Program, class.Cohort); err != nil {
			return err
		}
	}
	observed, err := time.Parse(time.RFC3339Nano, ObservedAt)
	if err != nil {
		return err
	}
	for _, item := range plan.Participations {
		if _, err = tx.Exec(ctx, "INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($1,$2,$3,'ACTIVE')", item.ID, item.ClassID, item.UserID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.episodes(id,enrollment_id,opened_at) VALUES($1,$2,$3)", item.EpisodeID, item.ID, observed); err != nil {
			return err
		}
		for _, role := range item.Roles {
			if _, err = tx.Exec(ctx, "INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($1,$2,$3,$4)", id("role", item.ID, role), item.ID, item.ClassID, role); err != nil {
				return err
			}
		}
	}
	for _, decision := range plan.Decisions {
		if _, err = tx.Exec(ctx, "INSERT INTO core_rehearsal.origin VALUES($1,$2,$3,$4,$5,$6)",
			decision.Table, decision.SourceID, decision.Disposition, decision.Reason, decision.TargetID, decision.SourceTime); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "INSERT INTO core_rehearsal.run VALUES($1,$2,$3,$4)", Snapshot, Mapping, plan.Digest(), backup)
	return err
}

func reconcile(ctx context.Context, tx pgx.Tx, plan Plan) error {
	check := func(query string, args ...any) bool {
		var ok bool
		return tx.QueryRow(ctx, query, args...).Scan(&ok) == nil && ok
	}
	if !check(`SELECT (SELECT count(*) FROM v3.users)=$1 AND (SELECT count(*) FROM v3.classes)=$2
  AND (SELECT count(*) FROM v3.enrollments)=$3 AND (SELECT count(*) FROM v3.episodes)=$3
  AND (SELECT count(*) FROM v3.roles)=$4 AND (SELECT count(*) FROM core_rehearsal.origin)=$5
  AND (SELECT count(*) FROM core_rehearsal.run)=1 AND NOT EXISTS (SELECT 1 FROM v3.sessions)
  AND NOT EXISTS (SELECT 1 FROM v3.outbox)`,
		plan.Counts.Users, plan.Counts.Classes, plan.Counts.Participations,
		plan.Counts.TeacherRoles+plan.Counts.StudentRoles, len(plan.Decisions)) {
		return ErrReconcile
	}
	for _, user := range plan.Users {
		if !check("SELECT EXISTS(SELECT 1 FROM v3.users WHERE id=$1 AND active=$2 AND NOT site_admin)", user.ID, user.Status == "active") {
			return ErrReconcile
		}
	}
	for _, class := range plan.Classes {
		if !check("SELECT EXISTS(SELECT 1 FROM v3.classes WHERE id=$1 AND name=$2 AND program_label=$3 AND cohort_label=$4 AND state='DRAFT' AND version=1)", class.ID, class.Name, class.Program, class.Cohort) {
			return ErrReconcile
		}
	}
	observed, _ := time.Parse(time.RFC3339Nano, ObservedAt)
	for _, item := range plan.Participations {
		if !check(`SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.episodes p ON p.enrollment_id=e.id
    WHERE e.id=$1 AND e.class_id=$2 AND e.user_id=$3 AND e.status='ACTIVE' AND p.id=$4 AND p.ended_at IS NULL AND p.opened_at=$5)`,
			item.ID, item.ClassID, item.UserID, item.EpisodeID, observed) {
			return ErrReconcile
		}
		for _, role := range item.Roles {
			if !check("SELECT EXISTS(SELECT 1 FROM v3.roles WHERE id=$1 AND enrollment_id=$2 AND class_id=$3 AND role=$4 AND revoked_at IS NULL)", id("role", item.ID, role), item.ID, item.ClassID, role) {
				return ErrReconcile
			}
		}
	}
	for _, decision := range plan.Decisions {
		if !check("SELECT EXISTS(SELECT 1 FROM core_rehearsal.origin WHERE source_table=$1 AND legacy_id=$2 AND disposition=$3 AND reason=$4 AND target_id=$5 AND original_time=$6)",
			decision.Table, decision.SourceID, decision.Disposition, decision.Reason, decision.TargetID, decision.SourceTime) {
			return ErrReconcile
		}
	}
	return nil
}
