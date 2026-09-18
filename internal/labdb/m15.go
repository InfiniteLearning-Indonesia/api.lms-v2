package labdb

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/migration/backup"
	"lmsv3/internal/platform/transaction"
)

const migrationFoundationSchema = `CREATE SCHEMA v3migration;
REVOKE ALL ON SCHEMA v3migration FROM PUBLIC;
CREATE TABLE v3migration.installations(version text PRIMARY KEY,checksum text NOT NULL CHECK(checksum ~ '^[a-f0-9]{64}$'),backup_hash text NOT NULL CHECK(backup_hash ~ '^[a-f0-9]{64}$'),applied_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE v3migration.snapshots(snapshot_id text PRIMARY KEY,source_environment text NOT NULL CHECK(source_environment='synthetic'),source_instance_id text NOT NULL,source_schema_sha256 text NOT NULL CHECK(source_schema_sha256 ~ '^[a-f0-9]{64}$'),source_snapshot_sha256 text NOT NULL CHECK(source_snapshot_sha256 ~ '^[a-f0-9]{64}$'),target_namespace text NOT NULL,manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[a-f0-9]{64}$'),plan_sha256 text NOT NULL CHECK(plan_sha256 ~ '^[a-f0-9]{64}$'),landed_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(snapshot_id,source_environment,source_instance_id,target_namespace));
CREATE TABLE v3migration.scope_objects(snapshot_id text NOT NULL REFERENCES v3migration.snapshots(snapshot_id),kind text NOT NULL CHECK(kind IN ('TABLE','FILE')),name text NOT NULL,expected_count bigint NOT NULL CHECK(expected_count>=0),object_sha256 text NOT NULL CHECK(object_sha256 ~ '^[a-f0-9]{64}$'),PRIMARY KEY(snapshot_id,kind,name),UNIQUE(snapshot_id,name));
CREATE TABLE v3migration.origin_keys(source_environment text NOT NULL,source_instance_id text NOT NULL,source_table text NOT NULL,source_pk text NOT NULL,target_namespace text NOT NULL,first_snapshot_id text NOT NULL REFERENCES v3migration.snapshots(snapshot_id),PRIMARY KEY(source_environment,source_instance_id,source_table,source_pk,target_namespace));
CREATE INDEX origin_keys_first_snapshot_idx ON v3migration.origin_keys(first_snapshot_id);
CREATE TABLE v3migration.landing_rows(snapshot_id text NOT NULL,source_environment text NOT NULL,source_instance_id text NOT NULL,source_table text NOT NULL,source_pk text NOT NULL,source_version text NOT NULL,target_namespace text NOT NULL,raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),sealed_ref text NOT NULL,landed_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(snapshot_id,source_table,source_pk,source_version),FOREIGN KEY(snapshot_id,source_environment,source_instance_id,target_namespace) REFERENCES v3migration.snapshots(snapshot_id,source_environment,source_instance_id,target_namespace),FOREIGN KEY(snapshot_id,source_table) REFERENCES v3migration.scope_objects(snapshot_id,name),FOREIGN KEY(source_environment,source_instance_id,source_table,source_pk,target_namespace) REFERENCES v3migration.origin_keys(source_environment,source_instance_id,source_table,source_pk,target_namespace));
CREATE INDEX landing_rows_origin_idx ON v3migration.landing_rows(source_environment,source_instance_id,source_table,source_pk,target_namespace);
CREATE FUNCTION v3migration.guard_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='M15_PROVENANCE_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3migration.guard_immutable() FROM PUBLIC;
CREATE TRIGGER installations_immutable BEFORE UPDATE OR DELETE ON v3migration.installations FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER snapshots_immutable BEFORE UPDATE OR DELETE ON v3migration.snapshots FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER scope_objects_immutable BEFORE UPDATE OR DELETE ON v3migration.scope_objects FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER origin_keys_immutable BEFORE UPDATE OR DELETE ON v3migration.origin_keys FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER landing_rows_immutable BEFORE UPDATE OR DELETE ON v3migration.landing_rows FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();`

const migrationMappingSchema = `CREATE TABLE v3migration.mapping_runs(run_id text PRIMARY KEY,snapshot_id text NOT NULL REFERENCES v3migration.snapshots(snapshot_id),mapping_version text NOT NULL,reviewer text NOT NULL,plan_sha256 text NOT NULL CHECK(plan_sha256 ~ '^[a-f0-9]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(snapshot_id,mapping_version),UNIQUE(run_id,snapshot_id));
CREATE INDEX mapping_runs_snapshot_idx ON v3migration.mapping_runs(snapshot_id,created_at);
CREATE TABLE v3migration.mapping_decisions(run_id text NOT NULL,snapshot_id text NOT NULL,source_table text NOT NULL,source_pk text NOT NULL,source_version text NOT NULL,disposition text NOT NULL CHECK(disposition IN ('MAP_CLASS','QUARANTINE','ARCHIVE_ONLY')),target_id text,reason_code text NOT NULL,raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),PRIMARY KEY(run_id,source_table,source_pk,source_version),FOREIGN KEY(run_id,snapshot_id) REFERENCES v3migration.mapping_runs(run_id,snapshot_id),FOREIGN KEY(snapshot_id,source_table,source_pk,source_version) REFERENCES v3migration.landing_rows(snapshot_id,source_table,source_pk,source_version),CHECK((disposition='MAP_CLASS' AND source_table='classes' AND target_id IS NOT NULL AND reason_code='CLASS_1_TO_1') OR (disposition<>'MAP_CLASS' AND target_id IS NULL)));
CREATE INDEX mapping_decisions_snapshot_idx ON v3migration.mapping_decisions(snapshot_id,source_table,source_pk);
CREATE TRIGGER mapping_runs_immutable BEFORE UPDATE OR DELETE ON v3migration.mapping_runs FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER mapping_decisions_immutable BEFORE UPDATE OR DELETE ON v3migration.mapping_decisions FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();`

const migrationBackfillSchema = `CREATE TABLE v3migration.owner_epochs(source_environment text NOT NULL,source_instance_id text NOT NULL,source_table text NOT NULL CHECK(source_table='classes'),source_pk text NOT NULL,target_namespace text NOT NULL,epoch bigint NOT NULL CHECK(epoch>0),owner text NOT NULL CHECK(owner IN ('LEGACY','SHADOW_ONLY','V3','HOLD_READ_ONLY')),recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(source_environment,source_instance_id,source_table,source_pk,target_namespace,epoch),FOREIGN KEY(source_environment,source_instance_id,source_table,source_pk,target_namespace) REFERENCES v3migration.origin_keys(source_environment,source_instance_id,source_table,source_pk,target_namespace));
CREATE INDEX owner_epochs_latest_idx ON v3migration.owner_epochs(source_environment,source_instance_id,source_table,source_pk,target_namespace,epoch DESC);
CREATE TABLE v3migration.shadow_classes(target_id text PRIMARY KEY,target_namespace text NOT NULL,source_environment text NOT NULL,source_instance_id text NOT NULL,source_table text NOT NULL CHECK(source_table='classes'),source_pk text NOT NULL,source_version text NOT NULL,raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),mapped_run_id text NOT NULL REFERENCES v3migration.mapping_runs(run_id),publish_state text NOT NULL DEFAULT 'UNPUBLISHED' CHECK(publish_state='UNPUBLISHED'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(source_environment,source_instance_id,source_table,source_pk,target_namespace),FOREIGN KEY(source_environment,source_instance_id,source_table,source_pk,target_namespace) REFERENCES v3migration.origin_keys(source_environment,source_instance_id,source_table,source_pk,target_namespace));
CREATE INDEX shadow_classes_run_idx ON v3migration.shadow_classes(mapped_run_id,target_id);
CREATE TABLE v3migration.origin_mappings(run_id text NOT NULL,source_table text NOT NULL,source_pk text NOT NULL,source_version text NOT NULL,target_id text NOT NULL REFERENCES v3migration.shadow_classes(target_id),raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),PRIMARY KEY(run_id,source_table,source_pk,source_version),FOREIGN KEY(run_id,source_table,source_pk,source_version) REFERENCES v3migration.mapping_decisions(run_id,source_table,source_pk,source_version));
CREATE INDEX origin_mappings_target_idx ON v3migration.origin_mappings(target_id);
CREATE TABLE v3migration.import_receipts(run_id text NOT NULL,source_table text NOT NULL,source_pk text NOT NULL,source_version text NOT NULL,target_id text NOT NULL REFERENCES v3migration.shadow_classes(target_id),raw_sha256 text NOT NULL CHECK(raw_sha256 ~ '^[a-f0-9]{64}$'),recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(run_id,source_table,source_pk,source_version),FOREIGN KEY(run_id,source_table,source_pk,source_version) REFERENCES v3migration.origin_mappings(run_id,source_table,source_pk,source_version));
CREATE INDEX import_receipts_target_idx ON v3migration.import_receipts(target_id);
CREATE TABLE v3migration.import_checkpoints(run_id text NOT NULL REFERENCES v3migration.mapping_runs(run_id),batch_no integer NOT NULL CHECK(batch_no=1),planned_count integer NOT NULL CHECK(planned_count BETWEEN 1 AND 100),applied_count integer NOT NULL CHECK(applied_count=planned_count),mapping_sha256 text NOT NULL CHECK(mapping_sha256 ~ '^[a-f0-9]{64}$'),backup_sha256 text NOT NULL CHECK(backup_sha256 ~ '^[a-f0-9]{64}$'),committed_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(run_id,batch_no));
CREATE TRIGGER owner_epochs_immutable BEFORE UPDATE OR DELETE ON v3migration.owner_epochs FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER shadow_classes_immutable BEFORE UPDATE OR DELETE ON v3migration.shadow_classes FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER origin_mappings_immutable BEFORE UPDATE OR DELETE ON v3migration.origin_mappings FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER import_receipts_immutable BEFORE UPDATE OR DELETE ON v3migration.import_receipts FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();
CREATE TRIGGER import_checkpoints_immutable BEFORE UPDATE OR DELETE ON v3migration.import_checkpoints FOR EACH ROW EXECUTE FUNCTION v3migration.guard_immutable();`

func M15StageChecksum(checkpoint string) string {
	switch checkpoint {
	case "m15-cp02":
		return Digest([]byte(migrationFoundationSchema))
	case "m15-cp03":
		return Digest([]byte(migrationMappingSchema))
	case "m15-cp04":
		return Digest([]byte(migrationBackfillSchema))
	default:
		return ""
	}
}

// InstallMigrationStage is a closed, additive local-only migration. CP03 and
// CP04 each require a new dump plus actual isolated restore proof.
func InstallMigrationStage(ctx context.Context, pool *pgxpool.Pool, checkpoint string) error {
	var ddl, dependency string
	switch checkpoint {
	case "m15-cp03":
		ddl, dependency = migrationMappingSchema, "m15-cp02"
	case "m15-cp04":
		ddl, dependency = migrationBackfillSchema, "m15-cp03"
	default:
		return errors.New("UNKNOWN_M15_STAGE")
	}
	if pool == nil || VerifyInstallmentProof(checkpoint, "pre_"+checkpoint[:3]+"_"+checkpoint[4:]+".dump") != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.Database != "qa_cp05" || config.User != "qa_fixture" || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var old string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3migration.installations WHERE version=$1", dependency+"-1").Scan(&old); err != nil || old != M15StageChecksum(dependency) {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3migration.installations WHERE version=$1", checkpoint+"-1").Scan(&old)
		if err == nil {
			if old != M15StageChecksum(checkpoint) {
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
		_, err = tx.Exec(ctx, "INSERT INTO v3migration.installations(version,checksum,backup_hash) VALUES($1,$2,$3)", checkpoint+"-1", M15StageChecksum(checkpoint), proofArchiveHash(checkpoint))
		return err
	})
}

func TargetFromEnvironment() string {
	return Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER"))
}
func M08CP04Checksum() string { return Digest([]byte(submissionChronologySchema)) }

func VerifyInstallmentProof(checkpoint, filename string) error {
	_, err := ValidatedInstallmentProof(checkpoint, filename)
	return err
}

func ValidatedInstallmentProof(checkpoint, filename string) (Proof, error) {
	root := os.Getenv("QA_LAB_DIR")
	if !backup.SafeRoot(root) {
		return Proof{}, errors.New("LAB_BACKUP_ROOT_REJECTED")
	}
	path := filepath.Join(root, checkpoint+"-proof.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return Proof{}, errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Proof{}, err
	}
	var proof Proof
	if json.Unmarshal(data, &proof) != nil {
		return Proof{}, errors.New("LAB_BACKUP_RECEIPT_INVALID")
	}
	if err = validateArtifact(proof, root, TargetFromEnvironment(), time.Now().UTC(), filename); err != nil {
		return Proof{}, err
	}
	return proof, nil
}

// InstallMigrationFoundation can install only the fixed private schema into
// the fresh local cluster after a checkpoint-specific verified restore.
func InstallMigrationFoundation(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil || VerifyInstallmentProof("m15-cp02", "pre_m15_cp02.dump") != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	config := pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.Database != "qa_cp05" || config.User != "qa_fixture" || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var old string
		if err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m08-cp04-1'").Scan(&old); err != nil || old != M08CP04Checksum() {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		var installed bool
		if err := tx.QueryRow(ctx, "SELECT to_regclass('v3migration.installations') IS NOT NULL").Scan(&installed); err != nil {
			return err
		}
		if installed {
			if err := tx.QueryRow(ctx, "SELECT checksum FROM v3migration.installations WHERE version='m15-cp02-1'").Scan(&old); err != nil || old != Digest([]byte(migrationFoundationSchema)) {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		var occupied bool
		if err := tx.QueryRow(ctx, "SELECT to_regnamespace('v3migration') IS NOT NULL").Scan(&occupied); err != nil || occupied {
			return errors.New("MIGRATION_PRIVATE_SCHEMA_DRIFT")
		}
		_, err := tx.Exec(ctx, migrationFoundationSchema, pgx.QueryExecModeSimpleProtocol)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3migration.installations(version,checksum,backup_hash) VALUES('m15-cp02-1',$1,$2)", Digest([]byte(migrationFoundationSchema)), proofArchiveHash("m15-cp02"))
		return err
	})
}

func proofArchiveHash(checkpoint string) string {
	data, err := os.ReadFile(filepath.Join(os.Getenv("QA_LAB_DIR"), checkpoint+"-proof.json"))
	if err != nil {
		return ""
	}
	var p Proof
	if json.Unmarshal(data, &p) != nil {
		return ""
	}
	return p.ArchiveHash
}
