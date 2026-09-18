package foundation

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
)

var ErrReconciliation = errors.New("M15_RECONCILIATION_REJECTED")

type Reconciliation struct {
	RunID          string
	SnapshotID     string
	TableScopes    int
	FileScopes     int
	LandingRows    int
	DecisionRows   int
	MappedClasses  int
	Quarantined    int
	Archived       int
	ShadowClasses  int
	OriginMappings int
	Receipts       int
	CheckpointHash string
	EvidenceHash   string
}

// Reconcile validates one complete unpublished synthetic import under a stable
// read-only snapshot. A FILE scope proves manifest accounting, not object bytes.
func Reconcile(ctx context.Context, pool *pgxpool.Pool, runID string) (Reconciliation, error) {
	var result Reconciliation
	if !validToken(runID) || syntheticOwnerPool(ctx, pool) != nil {
		return result, ErrReconciliation
	}
	proof, err := labdb.ValidatedInstallmentProof("m15-cp05", "pre_m15_cp05.dump")
	if err != nil {
		return result, ErrReconciliation
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var version, reviewer, mappingHash, sourceHash, manifestHash, environment, instance, namespace string
	err = tx.QueryRow(ctx, `SELECT r.snapshot_id,r.mapping_version,r.reviewer,r.plan_sha256,s.plan_sha256,s.manifest_sha256,s.source_environment,s.source_instance_id,s.target_namespace FROM v3migration.mapping_runs r JOIN v3migration.snapshots s ON s.snapshot_id=r.snapshot_id WHERE r.run_id=$1`, runID).Scan(&result.SnapshotID, &version, &reviewer, &mappingHash, &sourceHash, &manifestHash, &environment, &instance, &namespace)
	if err != nil || environment != "synthetic" {
		return Reconciliation{}, ErrReconciliation
	}
	result.RunID = runID
	var committedAt time.Time
	var planned, applied int
	err = tx.QueryRow(ctx, `SELECT planned_count,applied_count,mapping_sha256,committed_at FROM v3migration.import_checkpoints WHERE run_id=$1 AND batch_no=1`, runID).Scan(&planned, &applied, &result.CheckpointHash, &committedAt)
	if err != nil || proof.CreatedAt.Before(committedAt) || planned < 1 || planned > 100 || applied != planned || result.CheckpointHash != mappingHash {
		return Reconciliation{}, ErrReconciliation
	}
	var backupHash string
	if tx.QueryRow(ctx, `SELECT backup_sha256 FROM v3migration.import_checkpoints WHERE run_id=$1 AND batch_no=1`, runID).Scan(&backupHash) != nil || !validHash(backupHash) {
		return Reconciliation{}, ErrReconciliation
	}
	objects, err := tx.Query(ctx, `SELECT kind,name,expected_count,object_sha256 FROM v3migration.scope_objects WHERE snapshot_id=$1 ORDER BY kind,name`, result.SnapshotID)
	if err != nil {
		return Reconciliation{}, err
	}
	var scopes []Object
	for objects.Next() {
		var o Object
		if objects.Scan(&o.Kind, &o.Name, &o.Count, &o.SHA256) != nil {
			objects.Close()
			return Reconciliation{}, ErrReconciliation
		}
		scopes = append(scopes, o)
		if o.Kind == "FILE" {
			result.FileScopes++
		} else if o.Kind == "TABLE" {
			result.TableScopes++
		}
	}
	err = objects.Err()
	objects.Close()
	if err != nil {
		return Reconciliation{}, err
	}
	computedManifest, scopeMap, err := inventory(scopes)
	if err != nil || computedManifest != manifestHash || result.TableScopes < len(BaselineTables) || result.FileScopes == 0 {
		return Reconciliation{}, ErrReconciliation
	}
	for _, table := range BaselineTables {
		if _, ok := scopeMap["TABLE:"+table]; !ok {
			return Reconciliation{}, ErrReconciliation
		}
	}
	landed, err := tx.Query(ctx, `SELECT source_table,source_pk,source_version,raw_sha256,sealed_ref FROM v3migration.landing_rows WHERE snapshot_id=$1 ORDER BY source_table,source_pk,source_version`, result.SnapshotID)
	if err != nil {
		return Reconciliation{}, err
	}
	parts := map[string][]string{}
	landingKeys := map[string]string{}
	for landed.Next() {
		var table, pk, ver, hash, sealed string
		if landed.Scan(&table, &pk, &ver, &hash, &sealed) != nil || !validHash(hash) || !validToken(sealed) {
			landed.Close()
			return Reconciliation{}, ErrReconciliation
		}
		if _, ok := scopeMap["TABLE:"+table]; !ok {
			landed.Close()
			return Reconciliation{}, ErrReconciliation
		}
		parts[table] = append(parts[table], pk+"\x00"+ver+"\x00"+hash)
		landingKeys[table+"\x00"+pk+"\x00"+ver] = hash
		result.LandingRows++
	}
	err = landed.Err()
	landed.Close()
	if err != nil {
		return Reconciliation{}, err
	}
	for _, scope := range scopes {
		if scope.Kind != "TABLE" {
			continue
		}
		rows := parts[scope.Name]
		sort.Strings(rows)
		content := "empty:" + scope.Name
		if len(rows) != 0 {
			content = strings.Join(rows, "\n")
		}
		if scope.Count != int64(len(rows)) || scope.SHA256 != digest(content) {
			return Reconciliation{}, ErrReconciliation
		}
	}
	rows, err := tx.Query(ctx, `SELECT source_table,source_pk,source_version,disposition,coalesce(target_id,''),reason_code,raw_sha256 FROM v3migration.mapping_decisions WHERE run_id=$1 ORDER BY source_table,source_pk,source_version`, runID)
	if err != nil {
		return Reconciliation{}, err
	}
	var decisions []Decision
	for rows.Next() {
		var d Decision
		if rows.Scan(&d.Table, &d.PK, &d.Version, &d.Disposition, &d.TargetID, &d.ReasonCode, &d.RawSHA256) != nil || landingKeys[d.Table+"\x00"+d.PK+"\x00"+d.Version] != d.RawSHA256 {
			rows.Close()
			return Reconciliation{}, ErrReconciliation
		}
		decisions = append(decisions, d)
		result.DecisionRows++
		switch d.Disposition {
		case "MAP_CLASS":
			result.MappedClasses++
		case "QUARANTINE":
			result.Quarantined++
		case "ARCHIVE_ONLY":
			result.Archived++
		default:
			rows.Close()
			return Reconciliation{}, ErrReconciliation
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Reconciliation{}, err
	}
	if result.DecisionRows != result.LandingRows || result.MappedClasses != planned || decisionDigest(result.SnapshotID, version, reviewer, sourceHash, decisions) != mappingHash {
		return Reconciliation{}, ErrReconciliation
	}
	for _, d := range decisions {
		if d.Disposition != "MAP_CLASS" {
			continue
		}
		if d.Table != "classes" || d.TargetID != classTargetID(environment, instance, d.PK, namespace) {
			return Reconciliation{}, ErrReconciliation
		}
		var shadowHash, mapHash, receiptHash, owner string
		err = tx.QueryRow(ctx, `SELECT s.raw_sha256,m.raw_sha256,r.raw_sha256 FROM v3migration.shadow_classes s JOIN v3migration.origin_mappings m ON m.target_id=s.target_id AND m.run_id=s.mapped_run_id JOIN v3migration.import_receipts r ON r.run_id=m.run_id AND r.source_table=m.source_table AND r.source_pk=m.source_pk AND r.source_version=m.source_version AND r.target_id=m.target_id WHERE s.mapped_run_id=$1 AND s.target_id=$2 AND s.source_environment=$3 AND s.source_instance_id=$4 AND s.source_table='classes' AND s.source_pk=$5 AND s.source_version=$6 AND s.target_namespace=$7 AND s.publish_state='UNPUBLISHED'`, runID, d.TargetID, environment, instance, d.PK, d.Version, namespace).Scan(&shadowHash, &mapHash, &receiptHash)
		if err != nil || shadowHash != d.RawSHA256 || mapHash != d.RawSHA256 || receiptHash != d.RawSHA256 {
			return Reconciliation{}, ErrReconciliation
		}
		err = tx.QueryRow(ctx, `SELECT owner FROM v3migration.owner_epochs WHERE source_environment=$1 AND source_instance_id=$2 AND source_table='classes' AND source_pk=$3 AND target_namespace=$4 ORDER BY epoch DESC LIMIT 1`, environment, instance, d.PK, namespace).Scan(&owner)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Reconciliation{}, err
		}
		if owner == "V3" || owner == "HOLD_READ_ONLY" {
			return Reconciliation{}, ErrReconciliation
		}
		var publicCount int
		if tx.QueryRow(ctx, `SELECT count(*) FROM v3.classes WHERE id=$1`, d.TargetID).Scan(&publicCount) != nil || publicCount != 0 {
			return Reconciliation{}, ErrReconciliation
		}
	}
	for query, target := range map[string]*int{
		`SELECT count(*) FROM v3migration.shadow_classes WHERE mapped_run_id=$1`: &result.ShadowClasses,
		`SELECT count(*) FROM v3migration.origin_mappings WHERE run_id=$1`:       &result.OriginMappings,
		`SELECT count(*) FROM v3migration.import_receipts WHERE run_id=$1`:       &result.Receipts,
	} {
		if tx.QueryRow(ctx, query, runID).Scan(target) != nil || *target != planned {
			return Reconciliation{}, ErrReconciliation
		}
	}
	var outbound int
	if tx.QueryRow(ctx, `SELECT count(*) FROM v3.outbox WHERE topic LIKE 'migration.%'`).Scan(&outbound) != nil || outbound != 0 {
		return Reconciliation{}, ErrReconciliation
	}
	result.EvidenceHash = digest(runID + "\x00" + manifestHash + "\x00" + mappingHash + "\x00" + backupHash + "\x00" + proof.ArchiveHash)
	if err = tx.Commit(ctx); err != nil {
		return Reconciliation{}, err
	}
	return result, nil
}
