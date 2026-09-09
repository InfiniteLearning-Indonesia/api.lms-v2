// Package runner applies a closed, forward-only migration catalogue.
// It never discovers SQL from a database or accepts SQL from an operator request.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
)

var (
	identifier  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$`)
	digest      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	destructive = regexp.MustCompile(`(?i)\b(DELETE|TRUNCATE|DROP)\b`)
)

var (
	ErrGate      = errors.New("MIGRATION_BACKUP_GATE_REJECTED")
	ErrCatalogue = errors.New("MIGRATION_CATALOGUE_REJECTED")
	ErrConflict  = errors.New("MIGRATION_CHECKSUM_CONFLICT")
	ErrBusy      = errors.New("MIGRATION_BUSY")
	ErrDrift     = errors.New("MIGRATION_LEDGER_DRIFT")
)

type Migration struct {
	Version  string
	SQL      string
	Requires []string
}

func (m Migration) Checksum() string {
	sum := sha256.Sum256([]byte(m.SQL))
	return hex.EncodeToString(sum[:])
}

// Gate is created only after a fresh backup has been restored and reconciled.
// It is data, not authority: the operator command must validate the target too.
type Gate struct {
	RunID             string
	TargetFingerprint string
	ArchiveSHA256     string
	RestoreVerified   bool
	VerifiedAt        time.Time
}

func (g Gate) Validate(expected string, now time.Time) error {
	if !identifier.MatchString(g.RunID) || !digest.MatchString(g.TargetFingerprint) || g.TargetFingerprint != expected ||
		!digest.MatchString(g.ArchiveSHA256) || !g.RestoreVerified || now.Before(g.VerifiedAt) || now.Sub(g.VerifiedAt) > time.Hour {
		return ErrGate
	}
	return nil
}

func ValidateCatalogue(items []Migration) error {
	if len(items) == 0 {
		return ErrCatalogue
	}
	seen := map[string]bool{}
	for i, item := range items {
		if !identifier.MatchString(item.Version) || item.SQL == "" || destructive.MatchString(item.SQL) || seen[item.Version] {
			return ErrCatalogue
		}
		seen[item.Version] = true
		for _, dependency := range item.Requires {
			if !seen[dependency] || dependency == item.Version {
				return ErrCatalogue
			}
		}
		if i > 0 && len(item.Requires) == 0 {
			return ErrCatalogue
		}
	}
	return nil
}

type Result struct {
	Applied  []string
	Replayed []string
}

// Apply verifies the immutable target identity before acquiring the global
// migration lock. All pending catalogue entries commit atomically. A failed
// installment therefore cannot leave a partly advanced ledger.
func Apply(ctx context.Context, pool *pgxpool.Pool, catalogue []Migration, gate Gate, expectedTarget string, verifyTarget func(context.Context, pgx.Tx) error) (Result, error) {
	if pool == nil || verifyTarget == nil {
		return Result{}, ErrGate
	}
	if err := ValidateCatalogue(catalogue); err != nil {
		return Result{}, err
	}
	if err := gate.Validate(expectedTarget, time.Now().UTC()); err != nil {
		return Result{}, err
	}
	result := Result{}
	err := transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		if err := verifyTarget(ctx, tx); err != nil {
			return ErrGate
		}
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return ErrBusy
		}
		var ledger bool
		if err := tx.QueryRow(ctx, "SELECT to_regclass('v3ops.migrations') IS NOT NULL").Scan(&ledger); err != nil {
			return err
		}
		if !ledger {
			var occupied bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname IN ('v3','v3ops','v3identity'))").Scan(&occupied); err != nil {
				return err
			}
			if occupied {
				return ErrDrift
			}
		}
		known := make(map[string]string, len(catalogue))
		for _, item := range catalogue {
			known[item.Version] = item.Checksum()
		}
		if ledger {
			rows, err := tx.Query(ctx, "SELECT version,checksum FROM v3ops.migrations ORDER BY version")
			if err != nil {
				return err
			}
			for rows.Next() {
				var version, checksum string
				if rows.Scan(&version, &checksum) != nil || known[version] == "" {
					rows.Close()
					return ErrDrift
				}
				if checksum != known[version] {
					rows.Close()
					return ErrConflict
				}
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
		applied := map[string]bool{}
		for _, item := range catalogue {
			if ledger {
				var checksum string
				err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", item.Version).Scan(&checksum)
				if err == nil {
					if checksum != item.Checksum() {
						return ErrConflict
					}
					applied[item.Version] = true
					result.Replayed = append(result.Replayed, item.Version)
					continue
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			for _, dependency := range item.Requires {
				if !applied[dependency] {
					return ErrDrift
				}
			}
			if _, err := tx.Exec(ctx, item.SQL, pgx.QueryExecModeSimpleProtocol); err != nil {
				return err
			}
			ledger = true
			if _, err := tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", item.Version, item.Checksum(), gate.ArchiveSHA256); err != nil {
				return err
			}
			applied[item.Version] = true
			result.Applied = append(result.Applied, item.Version)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Ready is read-only and exact: missing, unknown, or checksum-drifted ledger
// entries keep the runtime unready. Runtime startup never repairs them.
func Ready(ctx context.Context, pool *pgxpool.Pool, catalogue []Migration) error {
	if pool == nil || ValidateCatalogue(catalogue) != nil {
		return ErrCatalogue
	}
	rows, err := pool.Query(ctx, "SELECT version,checksum FROM v3ops.migrations ORDER BY version")
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := map[string]string{}
	for rows.Next() {
		var version, checksum string
		if rows.Scan(&version, &checksum) != nil {
			return ErrDrift
		}
		actual[version] = checksum
	}
	if rows.Err() != nil || len(actual) != len(catalogue) {
		return ErrDrift
	}
	for _, item := range catalogue {
		if actual[item.Version] != item.Checksum() {
			return ErrDrift
		}
	}
	return nil
}
