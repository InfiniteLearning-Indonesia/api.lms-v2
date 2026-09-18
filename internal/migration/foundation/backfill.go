package foundation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
	"lmsv3/internal/platform/transaction"
)

var ErrBackfill = errors.New("M15_BACKFILL_REJECTED")
var ErrOwnerFence = errors.New("M15_OWNER_FENCE_REJECTED")

type classDecision struct{ pk, version, targetID, rawHash string }

func syntheticOwnerPool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrBackfill
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || labdb.Identity(ctx, pool) != nil {
		return ErrBackfill
	}
	return nil
}

// Backfill writes only an unpublished private build. Actual Class publication,
// academic mapping, and production ownership changes are outside this slice.
func Backfill(ctx context.Context, pool *pgxpool.Pool, runID string) error {
	return backfill(ctx, pool, runID, false)
}

func backfill(ctx context.Context, pool *pgxpool.Pool, runID string, failBeforeCommit bool) error {
	if !validToken(runID) || syntheticOwnerPool(ctx, pool) != nil {
		return ErrBackfill
	}
	proof, err := labdb.ValidatedInstallmentProof("m15-cp04", "pre_m15_cp04.dump")
	if err != nil {
		return ErrBackfill
	}
	var checksum string
	if pool.QueryRow(ctx, "SELECT checksum FROM v3migration.installations WHERE version='m15-cp04-1'").Scan(&checksum) != nil || checksum != labdb.M15StageChecksum("m15-cp04") {
		return ErrBackfill
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if e := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810917)").Scan(&locked); e != nil || !locked {
			return ErrBackfill
		}
		var snapshotID, version, reviewer, mappingHash, environment, instance, namespace, sourceHash string
		var createdAt time.Time
		if e := tx.QueryRow(ctx, `SELECT r.snapshot_id,r.mapping_version,r.reviewer,r.plan_sha256,r.created_at,s.source_environment,s.source_instance_id,s.target_namespace,s.plan_sha256 FROM v3migration.mapping_runs r JOIN v3migration.snapshots s ON s.snapshot_id=r.snapshot_id WHERE r.run_id=$1`, runID).Scan(&snapshotID, &version, &reviewer, &mappingHash, &createdAt, &environment, &instance, &namespace, &sourceHash); e != nil || proof.CreatedAt.Before(createdAt) {
			return ErrBackfill
		}
		rows, e := tx.Query(ctx, "SELECT source_table,source_pk,source_version,disposition,coalesce(target_id,''),reason_code,raw_sha256 FROM v3migration.mapping_decisions WHERE run_id=$1 ORDER BY source_table,source_pk,source_version", runID)
		if e != nil {
			return e
		}
		var decisions []Decision
		var classes []classDecision
		for rows.Next() {
			var d Decision
			if rows.Scan(&d.Table, &d.PK, &d.Version, &d.Disposition, &d.TargetID, &d.ReasonCode, &d.RawSHA256) != nil {
				rows.Close()
				return ErrBackfill
			}
			decisions = append(decisions, d)
			if d.Disposition == "MAP_CLASS" {
				classes = append(classes, classDecision{pk: d.PK, version: d.Version, targetID: d.TargetID, rawHash: d.RawSHA256})
			}
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		if len(classes) < 1 || len(classes) > 100 || decisionDigest(snapshotID, version, reviewer, sourceHash, decisions) != mappingHash {
			return ErrBackfill
		}
		for _, d := range classes {
			if d.targetID != classTargetID(environment, instance, d.pk, namespace) {
				return ErrBackfill
			}
			var owner string
			e := tx.QueryRow(ctx, `SELECT owner FROM v3migration.owner_epochs WHERE source_environment=$1 AND source_instance_id=$2 AND source_table='classes' AND source_pk=$3 AND target_namespace=$4 ORDER BY epoch DESC LIMIT 1`, environment, instance, d.pk, namespace).Scan(&owner)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if owner == "V3" || owner == "HOLD_READ_ONLY" {
				return ErrOwnerFence
			}
		}
		var oldHash, oldBackup string
		e = tx.QueryRow(ctx, "SELECT mapping_sha256,backup_sha256 FROM v3migration.import_checkpoints WHERE run_id=$1 AND batch_no=1", runID).Scan(&oldHash, &oldBackup)
		if e == nil {
			if oldHash != mappingHash || oldBackup != proof.ArchiveHash {
				return ErrConflict
			}
			var receiptCount, mapCount, shadowCount int
			if tx.QueryRow(ctx, "SELECT count(*) FROM v3migration.import_receipts WHERE run_id=$1", runID).Scan(&receiptCount) != nil || receiptCount != len(classes) {
				return ErrConflict
			}
			if tx.QueryRow(ctx, "SELECT count(*) FROM v3migration.origin_mappings WHERE run_id=$1", runID).Scan(&mapCount) != nil || mapCount != len(classes) {
				return ErrConflict
			}
			if tx.QueryRow(ctx, "SELECT count(*) FROM v3migration.shadow_classes WHERE mapped_run_id=$1", runID).Scan(&shadowCount) != nil || shadowCount != len(classes) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		for _, d := range classes {
			if _, e = tx.Exec(ctx, `INSERT INTO v3migration.shadow_classes(target_id,target_namespace,source_environment,source_instance_id,source_table,source_pk,source_version,raw_sha256,mapped_run_id) VALUES($1,$2,$3,$4,'classes',$5,$6,$7,$8)`, d.targetID, namespace, environment, instance, d.pk, d.version, d.rawHash, runID); e != nil {
				return ErrConflict
			}
			if _, e = tx.Exec(ctx, `INSERT INTO v3migration.origin_mappings(run_id,source_table,source_pk,source_version,target_id,raw_sha256) VALUES($1,'classes',$2,$3,$4,$5)`, runID, d.pk, d.version, d.targetID, d.rawHash); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO v3migration.import_receipts(run_id,source_table,source_pk,source_version,target_id,raw_sha256) VALUES($1,'classes',$2,$3,$4,$5)`, runID, d.pk, d.version, d.targetID, d.rawHash); e != nil {
				return e
			}
		}
		if failBeforeCommit {
			return ErrBackfill
		}
		_, e = tx.Exec(ctx, "INSERT INTO v3migration.import_checkpoints(run_id,batch_no,planned_count,applied_count,mapping_sha256,backup_sha256) VALUES($1,1,$2,$2,$3,$4)", runID, len(classes), mappingHash, proof.ArchiveHash)
		return e
	})
}

// AdvanceOwner records synthetic ownership as an append-only epoch under the
// same lock as import. It is not a production cutover API.
func AdvanceOwner(ctx context.Context, pool *pgxpool.Pool, environment, instance, pk, namespace string, expectedEpoch int64, next string) error {
	if syntheticOwnerPool(ctx, pool) != nil || !validToken(environment) || !validToken(instance) || !validToken(pk) || !validToken(namespace) || expectedEpoch < 0 {
		return ErrOwnerFence
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if e := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810917)").Scan(&locked); e != nil || !locked {
			return ErrOwnerFence
		}
		var epoch int64
		var current string
		e := tx.QueryRow(ctx, `SELECT epoch,owner FROM v3migration.owner_epochs WHERE source_environment=$1 AND source_instance_id=$2 AND source_table='classes' AND source_pk=$3 AND target_namespace=$4 ORDER BY epoch DESC LIMIT 1`, environment, instance, pk, namespace).Scan(&epoch, &current)
		if !errors.Is(e, pgx.ErrNoRows) && e != nil {
			return e
		}
		if epoch != expectedEpoch {
			return ErrOwnerFence
		}
		allowed := (epoch == 0 && next == "SHADOW_ONLY") || (current == "SHADOW_ONLY" && (next == "V3" || next == "HOLD_READ_ONLY"))
		if !allowed {
			return ErrOwnerFence
		}
		_, e = tx.Exec(ctx, "INSERT INTO v3migration.owner_epochs(source_environment,source_instance_id,source_table,source_pk,target_namespace,epoch,owner) VALUES($1,$2,'classes',$3,$4,$5,$6)", environment, instance, pk, namespace, epoch+1, next)
		return e
	})
}
