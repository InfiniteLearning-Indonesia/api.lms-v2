// Package foundation is an operator-only, synthetic M15 preflight and
// provenance slice. It never connects to a legacy source or writes v3 domain.
package foundation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
	"lmsv3/internal/platform/transaction"
)

var ErrPreflight = errors.New("M15_PREFLIGHT_REJECTED")
var ErrConflict = errors.New("M15_PROVENANCE_CONFLICT")

var BaselineTables = []string{"users", "programs", "batches", "classes", "enrollments", "program_competencies", "competencies", "materials", "assignments", "submissions", "competency_scores", "rubrik_assessments", "rubrik_assessment_scores", "attendances", "permission_requests", "holidays", "mentor_async_days", "logbooks", "audit_logs", "session"}

type Object struct {
	Kind   string // TABLE or FILE
	Name   string
	Count  int64
	SHA256 string
}

type Row struct {
	Table     string
	PK        string
	Version   string
	RawSHA256 string
	SealedRef string // opaque reference into restricted, separately verified archive
}

type Plan struct {
	SnapshotID           string
	SourceEnvironment    string
	SourceInstanceID     string
	SourceSchemaSHA256   string
	SourceSnapshotSHA256 string
	TargetNamespace      string
	ExpectedTarget       string
	Observed             []Object
	Declared             []Object
	Rows                 []Row
}

func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && s == strings.ToLower(s)
}

func syntheticSnapshotHash(inventoryHash string) string {
	return digest("synthetic-snapshot-v1\x00" + inventoryHash)
}
func validToken(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' || c == '/') {
			return false
		}
	}
	return true
}
func inventory(objects []Object) (string, map[string]Object, error) {
	seen := map[string]Object{}
	for _, o := range objects {
		if (o.Kind != "TABLE" && o.Kind != "FILE") || !validToken(o.Name) || o.Count < 0 || !validHash(o.SHA256) {
			return "", nil, ErrPreflight
		}
		key := o.Kind + ":" + o.Name
		if _, ok := seen[key]; ok {
			return "", nil, ErrPreflight
		}
		seen[key] = o
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		o := seen[k]
		fmt.Fprintf(&b, "%s\x00%d\x00%s\n", k, o.Count, o.SHA256)
	}
	return digest(b.String()), seen, nil
}

// Preflight is read-only. The local lab's TLS-off trust mode is deliberately
// distinct from a real CA-validated source adapter and cannot authorize one.
func Preflight(ctx context.Context, pool *pgxpool.Pool, plan Plan) (string, error) {
	if pool == nil || plan.SourceEnvironment != "synthetic" || !validToken(plan.SnapshotID) || !validToken(plan.SourceInstanceID) || !validToken(plan.TargetNamespace) || !validHash(plan.SourceSchemaSHA256) || !validHash(plan.SourceSnapshotSHA256) || plan.ExpectedTarget != labdb.TargetFromEnvironment() {
		return "", ErrPreflight
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.Database != "qa_cp05" || config.User != "qa_fixture" || config.TLSConfig != nil || labdb.Identity(ctx, pool) != nil {
		return "", ErrPreflight
	}
	if err := labdb.VerifyInstallmentProof("m15-cp01", "pre_m15_cp01.dump"); err != nil {
		return "", ErrPreflight
	}
	var role string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&role); err != nil || role != "qa_fixture" {
		return "", ErrPreflight
	}
	var checksum string
	if err := pool.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m08-cp04-1'").Scan(&checksum); err != nil || checksum != labdb.M08CP04Checksum() {
		return "", ErrPreflight
	}
	observedHash, observed, err := inventory(plan.Observed)
	if err != nil {
		return "", ErrPreflight
	}
	declaredHash, declared, err := inventory(plan.Declared)
	if err != nil || observedHash != declaredHash || len(observed) != len(declared) {
		return "", ErrPreflight
	}
	if plan.SourceSchemaSHA256 != digest("synthetic-schema-v1") || plan.SourceSnapshotSHA256 != syntheticSnapshotHash(observedHash) {
		return "", ErrPreflight
	}
	for _, name := range BaselineTables {
		if _, ok := observed["TABLE:"+name]; !ok {
			return "", ErrPreflight
		}
	}
	files := 0
	for _, object := range observed {
		if object.Kind == "FILE" {
			files++
		}
	}
	if files == 0 {
		return "", ErrPreflight
	} // file inventory mandatory
	for _, r := range plan.Rows {
		if !validToken(r.Table) || !validToken(r.PK) || !validToken(r.Version) || !validHash(r.RawSHA256) || !validToken(r.SealedRef) {
			return "", ErrPreflight
		}
		if _, ok := observed["TABLE:"+r.Table]; !ok {
			return "", ErrPreflight
		}
	}
	counts := map[string]int64{}
	rowKeys := map[string]bool{}
	for _, r := range plan.Rows {
		key := r.Table + "\x00" + r.PK + "\x00" + r.Version
		if rowKeys[key] {
			return "", ErrPreflight
		}
		rowKeys[key] = true
		counts[r.Table]++
	}
	for _, o := range observed {
		if o.Kind == "TABLE" && counts[o.Name] != o.Count {
			return "", ErrPreflight
		}
	}
	for _, o := range observed {
		if o.Kind != "TABLE" {
			continue
		}
		parts := []string{}
		for _, r := range plan.Rows {
			if r.Table == o.Name {
				parts = append(parts, r.PK+"\x00"+r.Version+"\x00"+r.RawSHA256)
			}
		}
		sort.Strings(parts)
		content := "empty:" + o.Name
		if len(parts) > 0 {
			content = strings.Join(parts, "\n")
		}
		if digest(content) != o.SHA256 {
			return "", ErrPreflight
		}
	}
	return observedHash, nil
}

func planDigest(plan Plan, manifest string) string {
	parts := []string{plan.SnapshotID, plan.SourceEnvironment, plan.SourceInstanceID, plan.SourceSchemaSHA256, plan.SourceSnapshotSHA256, plan.TargetNamespace, manifest}
	rows := make([]string, 0, len(plan.Rows))
	for _, r := range plan.Rows {
		rows = append(rows, r.Table+"\x00"+r.PK+"\x00"+r.Version+"\x00"+r.RawSHA256+"\x00"+r.SealedRef)
	}
	sort.Strings(rows)
	return digest(strings.Join(append(parts, rows...), "\n"))
}

// Capture commits only restricted provenance records. It is not a transform or
// importer, and its replay behavior does not mutate a prior snapshot.
func Capture(ctx context.Context, pool *pgxpool.Pool, plan Plan) error {
	manifest, err := Preflight(ctx, pool, plan)
	if err != nil {
		return err
	}
	planHash := planDigest(plan, manifest)
	if err = labdb.VerifyInstallmentProof("m15-cp02", "pre_m15_cp02.dump"); err != nil {
		return ErrPreflight
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810915)").Scan(&locked); err != nil || !locked {
			return ErrPreflight
		}
		var existing string
		err := tx.QueryRow(ctx, "SELECT plan_sha256 FROM v3migration.snapshots WHERE snapshot_id=$1", plan.SnapshotID).Scan(&existing)
		if err == nil {
			if existing != planHash {
				return ErrConflict
			}
			var objects, rows int
			if tx.QueryRow(ctx, "SELECT count(*) FROM v3migration.scope_objects WHERE snapshot_id=$1", plan.SnapshotID).Scan(&objects) != nil || objects != len(plan.Declared) {
				return ErrConflict
			}
			if tx.QueryRow(ctx, "SELECT count(*) FROM v3migration.landing_rows WHERE snapshot_id=$1", plan.SnapshotID).Scan(&rows) != nil || rows != len(plan.Rows) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO v3migration.snapshots(snapshot_id,source_environment,source_instance_id,source_schema_sha256,source_snapshot_sha256,target_namespace,manifest_sha256,plan_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, plan.SnapshotID, plan.SourceEnvironment, plan.SourceInstanceID, plan.SourceSchemaSHA256, plan.SourceSnapshotSHA256, plan.TargetNamespace, manifest, planHash)
		if err != nil {
			return err
		}
		for _, o := range plan.Declared {
			if _, err = tx.Exec(ctx, `INSERT INTO v3migration.scope_objects(snapshot_id,kind,name,expected_count,object_sha256) VALUES($1,$2,$3,$4,$5)`, plan.SnapshotID, o.Kind, o.Name, o.Count, o.SHA256); err != nil {
				return err
			}
		}
		for _, r := range plan.Rows {
			if _, err = tx.Exec(ctx, `INSERT INTO v3migration.origin_keys(source_environment,source_instance_id,source_table,source_pk,target_namespace,first_snapshot_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, plan.SourceEnvironment, plan.SourceInstanceID, r.Table, r.PK, plan.TargetNamespace, plan.SnapshotID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO v3migration.landing_rows(snapshot_id,source_environment,source_instance_id,source_table,source_pk,source_version,target_namespace,raw_sha256,sealed_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, plan.SnapshotID, plan.SourceEnvironment, plan.SourceInstanceID, r.Table, r.PK, r.Version, plan.TargetNamespace, r.RawSHA256, r.SealedRef); err != nil {
				return err
			}
		}
		return nil
	})
}

func NewSyntheticPlan(target, snapshot string) Plan {
	objects := make([]Object, 0, len(BaselineTables)+2)
	for _, table := range BaselineTables {
		objects = append(objects, Object{Kind: "TABLE", Name: table, Count: 0, SHA256: digest("empty:" + table)})
	}
	objects = append(objects, Object{Kind: "TABLE", Name: "custom_tracking", Count: 0, SHA256: digest("empty:custom_tracking")}, Object{Kind: "FILE", Name: "r2/fixture-object", Count: 1, SHA256: digest("fixture-object")})
	inventoryHash, _, _ := inventory(objects)
	return Plan{SnapshotID: snapshot, SourceEnvironment: "synthetic", SourceInstanceID: "synthetic-source-1", SourceSchemaSHA256: digest("synthetic-schema-v1"), SourceSnapshotSHA256: syntheticSnapshotHash(inventoryHash), TargetNamespace: "synthetic-rehearsal", ExpectedTarget: target, Observed: append([]Object(nil), objects...), Declared: append([]Object(nil), objects...)}
}
