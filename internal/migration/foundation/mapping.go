package foundation

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/labdb"
	"lmsv3/internal/platform/transaction"
)

var ErrMapping = errors.New("M15_MAPPING_REJECTED")

type Decision struct {
	Table       string
	PK          string
	Version     string
	Disposition string
	TargetID    string
	ReasonCode  string
	RawSHA256   string
}

func classTargetID(environment, instance, pk, namespace string) string {
	return digest(environment + "\x00" + instance + "\x00classes\x00" + pk + "\x00" + namespace)[:32]
}

func classify(table, pk, version, rawHash, environment, instance, namespace string, versions int) Decision {
	d := Decision{Table: table, PK: pk, Version: version, RawSHA256: rawHash}
	switch {
	case table == "classes" && versions == 1:
		d.Disposition, d.ReasonCode, d.TargetID = "MAP_CLASS", "CLASS_1_TO_1", classTargetID(environment, instance, pk, namespace)
	case table == "classes":
		d.Disposition, d.ReasonCode = "QUARANTINE", "CLASS_VERSION_AMBIGUOUS"
	case table == "competency_scores" || table == "rubrik_assessment_scores":
		d.Disposition, d.ReasonCode = "QUARANTINE", "SCORE_CLASS_AMBIGUOUS"
	case table == "batches":
		d.Disposition, d.ReasonCode = "QUARANTINE", "BATCH_CHRONOLOGY_AMBIGUOUS"
	case table == "session":
		d.Disposition, d.ReasonCode = "ARCHIVE_ONLY", "AUTH_SESSION_RESTRICTED"
	default:
		known := false
		for _, baseline := range BaselineTables {
			if table == baseline {
				known = true
				break
			}
		}
		if known {
			d.Disposition, d.ReasonCode = "ARCHIVE_ONLY", "DEFERRED_DOMAIN_SCOPE"
		} else {
			d.Disposition, d.ReasonCode = "QUARANTINE", "UNREVIEWED_SOURCE_TABLE"
		}
	}
	return d
}

func decisionDigest(snapshotID, mappingVersion, reviewer, sourceHash string, decisions []Decision) string {
	parts := []string{snapshotID, mappingVersion, reviewer, sourceHash}
	for _, d := range decisions {
		parts = append(parts, strings.Join([]string{d.Table, d.PK, d.Version, d.Disposition, d.TargetID, d.ReasonCode, d.RawSHA256}, "\x00"))
	}
	return digest(strings.Join(parts, "\n"))
}

// Decide writes a complete, immutable synthetic decision set. The reviewer
// label is a fixture, never a substitute for real academic sign-off.
func Decide(ctx context.Context, pool *pgxpool.Pool, plan Plan, runID, mappingVersion, reviewer string) ([]Decision, error) {
	if !validToken(runID) || !validToken(mappingVersion) || reviewer != "qa-domain-owner" {
		return nil, ErrMapping
	}
	manifest, err := Preflight(ctx, pool, plan)
	if err != nil {
		return nil, err
	}
	if labdb.VerifyInstallmentProof("m15-cp03", "pre_m15_cp03.dump") != nil {
		return nil, ErrMapping
	}
	var checksum string
	if pool.QueryRow(ctx, "SELECT checksum FROM v3migration.installations WHERE version='m15-cp03-1'").Scan(&checksum) != nil || checksum != labdb.M15StageChecksum("m15-cp03") {
		return nil, ErrMapping
	}
	var environment, instance, namespace, sourceHash string
	if pool.QueryRow(ctx, "SELECT source_environment,source_instance_id,target_namespace,plan_sha256 FROM v3migration.snapshots WHERE snapshot_id=$1", plan.SnapshotID).Scan(&environment, &instance, &namespace, &sourceHash) != nil || environment != plan.SourceEnvironment || instance != plan.SourceInstanceID || namespace != plan.TargetNamespace || sourceHash != planDigest(plan, manifest) {
		return nil, ErrMapping
	}
	rows, err := pool.Query(ctx, "SELECT source_table,source_pk,source_version,raw_sha256 FROM v3migration.landing_rows WHERE snapshot_id=$1 ORDER BY source_table,source_pk,source_version", plan.SnapshotID)
	if err != nil {
		return nil, err
	}
	var source []Decision
	versions := map[string]int{}
	for rows.Next() {
		var d Decision
		if rows.Scan(&d.Table, &d.PK, &d.Version, &d.RawSHA256) != nil {
			rows.Close()
			return nil, ErrMapping
		}
		source = append(source, d)
		if d.Table == "classes" {
			versions[d.PK]++
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(source) == 0 || len(source) != len(plan.Rows) {
		return nil, ErrMapping
	}
	decisions := make([]Decision, 0, len(source))
	for _, r := range source {
		decisions = append(decisions, classify(r.Table, r.PK, r.Version, r.RawSHA256, environment, instance, namespace, versions[r.PK]))
	}
	sort.Slice(decisions, func(i, j int) bool {
		a, b := decisions[i], decisions[j]
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		if a.PK != b.PK {
			return a.PK < b.PK
		}
		return a.Version < b.Version
	})
	planHash := decisionDigest(plan.SnapshotID, mappingVersion, reviewer, sourceHash, decisions)
	err = transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if e := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810916)").Scan(&locked); e != nil || !locked {
			return ErrMapping
		}
		var existingRun, existingHash string
		e := tx.QueryRow(ctx, "SELECT run_id,plan_sha256 FROM v3migration.mapping_runs WHERE snapshot_id=$1 AND mapping_version=$2", plan.SnapshotID, mappingVersion).Scan(&existingRun, &existingHash)
		if e == nil {
			if existingRun != runID || existingHash != planHash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO v3migration.mapping_runs(run_id,snapshot_id,mapping_version,reviewer,plan_sha256) VALUES($1,$2,$3,$4,$5)", runID, plan.SnapshotID, mappingVersion, reviewer, planHash); e != nil {
			return e
		}
		for _, d := range decisions {
			if _, e = tx.Exec(ctx, "INSERT INTO v3migration.mapping_decisions(run_id,snapshot_id,source_table,source_pk,source_version,disposition,target_id,reason_code,raw_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", runID, plan.SnapshotID, d.Table, d.PK, d.Version, d.Disposition, nullableTarget(d.TargetID), d.ReasonCode, d.RawSHA256); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return decisions, nil
}

func nullableTarget(value string) any {
	if value == "" {
		return nil
	}
	return value
}
