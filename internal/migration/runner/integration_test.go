//go:build qaintegration

package runner_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/labdb"
	"lmsv3/internal/migration/runner"
)

func TestSyntheticForwardRunner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	gate, err := labdb.MigrationGate(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	target := labdb.Target(os.Getenv("QA_PG_PORT"), os.Getenv("QA_PG_MARKER"))
	if err = gate.Validate(target, time.Now().UTC()); err != nil {
		t.Fatal("gate", err)
	}
	if err = runner.ValidateCatalogue(labdb.Catalogue()); err != nil {
		t.Fatal("catalogue", err)
	}
	verify := func(ctx context.Context, tx pgx.Tx) error {
		var marker string
		if err := tx.QueryRow(ctx, "SELECT value FROM qa_cp05.marker").Scan(&marker); err != nil || marker != os.Getenv("QA_PG_MARKER") {
			return errors.New("identity")
		}
		return nil
	}
	broken := append([]runner.Migration{}, labdb.Catalogue()...)
	broken = append(broken, runner.Migration{Version: "failure-fixture", SQL: "CREATE TABLE v3.invalid(", Requires: []string{"cp12-1"}})
	if _, err = runner.Apply(ctx, owner, broken, gate, target, verify); err == nil || errors.Is(err, runner.ErrGate) || errors.Is(err, runner.ErrCatalogue) {
		t.Fatal("invalid installment committed")
	}
	var exists bool
	if owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='v3')").Scan(&exists) != nil || exists {
		t.Fatal("failed catalogue left partial schema")
	}

	held, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = held.Exec(ctx, "SELECT pg_advisory_xact_lock(810910)"); err != nil {
		t.Fatal(err)
	}
	_, applyErr := runner.Apply(ctx, owner, labdb.Catalogue(), gate, target, verify)
	if err = held.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(applyErr, runner.ErrBusy) {
		t.Fatal("migration lock not enforced", applyErr)
	}

	result, err := runner.Apply(ctx, owner, labdb.Catalogue(), gate, target, verify)
	if err != nil || len(result.Applied) != 3 {
		t.Fatal("catalogue apply failed", err, result)
	}
	if err = runner.Ready(ctx, owner, labdb.Catalogue()); err != nil {
		t.Fatal("ready ledger rejected", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := runner.Apply(ctx, owner, labdb.Catalogue(), gate, target, verify)
			if e == nil && len(r.Applied) != 0 {
				e = errors.New("replay reapplied migration")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for err = range errs {
		if err != nil && !errors.Is(err, runner.ErrBusy) {
			t.Fatal(err)
		}
	}

	drifted := labdb.Catalogue()
	drifted[0].SQL += "\nCREATE TABLE v3.drift(id integer);"
	if _, err = runner.Apply(ctx, owner, drifted, gate, target, verify); !errors.Is(err, runner.ErrConflict) {
		t.Fatal("checksum drift accepted", err)
	}
	badGate := gate
	badGate.RestoreVerified = false
	if _, err = runner.Apply(ctx, owner, labdb.Catalogue(), badGate, target, verify); !errors.Is(err, runner.ErrGate) {
		t.Fatal("missing restore accepted", err)
	}
}
