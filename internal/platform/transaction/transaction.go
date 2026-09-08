// Package transaction provides a single transaction boundary. Callers must put
// domain writes, audit and outbox writes inside the same callback. No retries:
// a lost commit acknowledgement may have committed and needs reconciliation.
package transaction

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Beginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func Within(ctx context.Context, db Beginner, action func(pgx.Tx) error) error {
	if db == nil || action == nil {
		return errors.New("TRANSACTION_INPUT_REQUIRED")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err = action(tx); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
