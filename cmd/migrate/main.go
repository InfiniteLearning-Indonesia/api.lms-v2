// migrate is a forward-only operator entrypoint. This checkpoint enables only
// the fresh local synthetic target; shared environments remain an explicit
// later operational gate. It never accepts caller-provided SQL.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/labdb"
	"lmsv3/internal/migration/runner"
)

func main() {
	if os.Getenv("LMS_MIGRATION_MODE") != "local-synthetic" || os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		os.Exit(1)
	}
	defer pool.Close()
	gate, err := labdb.MigrationGate(time.Now().UTC())
	if err != nil {
		os.Exit(1)
	}
	verify := func(ctx context.Context, tx pgx.Tx) error {
		var marker string
		if err := tx.QueryRow(ctx, "SELECT value FROM qa_cp05.marker").Scan(&marker); err != nil || marker != os.Getenv("QA_PG_MARKER") {
			return errors.New("TARGET_IDENTITY_REJECTED")
		}
		return nil
	}
	// Test-only prefix scope preserves the backup-before-each-installment proof.
	// This process is already fenced to a newly created synthetic cluster.
	catalogue, err := labdb.SyntheticCatalogueScope(os.Getenv("QA_CATALOGUE_SCOPE"))
	if err != nil {
		os.Exit(1)
	}
	result, err := runner.Apply(ctx, pool, catalogue, gate, labdb.Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER")), verify)
	if err != nil {
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "APPLIED", "applied": result.Applied, "replayed": result.Replayed, "scope": "SYNTHETIC_ONLY"})
}
