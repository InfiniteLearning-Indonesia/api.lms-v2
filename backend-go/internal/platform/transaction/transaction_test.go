package transaction

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

type fakeTx struct {
	pgx.Tx
	commits, rollbacks   int
	commitError          error
	rollbackContextValid bool
}

func (t *fakeTx) Commit(context.Context) error { t.commits++; return t.commitError }
func (t *fakeTx) Rollback(ctx context.Context) error {
	t.rollbacks++
	t.rollbackContextValid = ctx.Err() == nil
	return nil
}

type fakeDB struct {
	tx     *fakeTx
	err    error
	begins int
}

func (d *fakeDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	d.begins++
	return d.tx, d.err
}

func TestTransactionBoundaries(t *testing.T) {
	failure := errors.New("fixture failure")
	for _, stage := range []string{"begin", "domain", "audit", "outbox", "commit", "success"} {
		tx := &fakeTx{}
		db := &fakeDB{tx: tx}
		if stage == "begin" {
			db.err = failure
		}
		if stage == "commit" {
			tx.commitError = failure
		}
		err := Within(context.Background(), db, func(pgx.Tx) error {
			if stage == "domain" || stage == "audit" || stage == "outbox" {
				return failure
			}
			return nil
		})
		if (err == nil) != (stage == "success") {
			t.Fatal("failure lost")
		}
		if db.begins != 1 {
			t.Fatal("unexpected retry")
		}
		if stage == "begin" {
			if tx.rollbacks != 0 {
				t.Fatal("rollback before begin")
			}
		} else if tx.rollbacks != 1 || !tx.rollbackContextValid {
			t.Fatal("cleanup missing")
		}
		expected := 0
		if stage == "commit" || stage == "success" {
			expected = 1
		}
		if tx.commits != expected {
			t.Fatal("incorrect commit")
		}
	}
	if Within(context.Background(), nil, func(pgx.Tx) error { return nil }) == nil || Within(context.Background(), &fakeDB{}, nil) == nil {
		t.Fatal("invalid input accepted")
	}
}

func TestCancellationAndPanicRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db := &fakeDB{tx: &fakeTx{}}
	if Within(ctx, db, func(pgx.Tx) error { return nil }) == nil || db.begins != 0 {
		t.Fatal("cancelled request began")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	db = &fakeDB{tx: &fakeTx{}}
	if Within(ctx, db, func(pgx.Tx) error { cancel(); return nil }) == nil || db.tx.commits != 0 || !db.tx.rollbackContextValid {
		t.Fatal("cancel cleanup failed")
	}
	db = &fakeDB{tx: &fakeTx{}}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic swallowed")
			}
		}()
		_ = Within(context.Background(), db, func(pgx.Tx) error { panic("synthetic") })
	}()
	if db.tx.commits != 0 || db.tx.rollbacks != 1 {
		t.Fatal("panic cleanup failed")
	}
}
