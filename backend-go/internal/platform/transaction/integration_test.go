//go:build qaintegration

package transaction

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
	"testing"
	"time"
)

// ONLY invoked by qa.postgres_lab against its newly-created synthetic cluster.
// No dotenv, arbitrary host/database/schema or shared runtime configuration.
func syntheticPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	port, err := strconv.Atoi(os.Getenv("QA_PG_PORT"))
	if err != nil || port < 1024 || port > 65535 || os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" || len(os.Getenv("QA_PG_PASSWORD")) < 32 {
		t.Fatal("explicit isolated harness required; NOT SKIPPED")
	}
	cfg, err := pgxpool.ParseConfig("postgres://qa_fixture@127.0.0.1:" + strconv.Itoa(port) + "/qa_cp05?sslmode=disable")
	if err != nil {
		t.Fatal("fixture config failed")
	}
	cfg.ConnConfig.Password = os.Getenv("QA_PG_PASSWORD")
	cfg.ConnConfig.Fallbacks = nil
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "pg_catalog", "application_name": "lmsv3-synthetic-qa", "statement_timeout": "1000"}
	cfg.ConnConfig.ConnectTimeout = 2 * time.Second
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.ConnConfig.StatementCacheCapacity = 0
	cfg.ConnConfig.DescriptionCacheCapacity = 0
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Minute
	cfg.MaxConnIdleTime = time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture pool failed")
	}
	t.Cleanup(pool.Close)
	var marker string
	if pool.QueryRow(ctx, "SELECT value FROM qa_cp05.marker").Scan(&marker) != nil || marker != os.Getenv("QA_PG_MARKER") {
		t.Fatal("wrong fixture identity")
	}
	return pool
}

func TestSyntheticPostgresTransactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := syntheticPool(t, ctx)
	var err error
	for _, stage := range []string{"domain", "audit", "outbox", "success"} {
		err = Within(ctx, pool, func(tx pgx.Tx) error {
			for _, table := range []string{"domain", "audit", "outbox"} {
				// table comes ONLY from this compile-time list, not API input.
				if _, e := tx.Exec(ctx, "INSERT INTO qa_cp05."+table+" (id) VALUES ($1)", stage); e != nil {
					return e
				}
				if stage == table {
					return errors.New("injected fixture failure")
				}
			}
			return nil
		})
		if (err == nil) != (stage == "success") {
			t.Fatal("transaction result mismatch")
		}
		for _, table := range []string{"domain", "audit", "outbox"} {
			var n int
			if pool.QueryRow(ctx, "SELECT count(*) FROM qa_cp05."+table+" WHERE id=$1", stage).Scan(&n) != nil {
				t.Fatal("reconcile failed")
			}
			expected := 0
			if stage == "success" {
				expected = 1
			}
			if n != expected {
				t.Fatal("partial atomic write")
			}
		}
	}
	held, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal("acquire failed")
	}
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = pool.Acquire(short)
	stop()
	held.Release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("pool exhaustion not bounded")
	}
	short, stop = context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = pool.Exec(short, "SELECT pg_catalog.pg_sleep(1)")
	stop()
	if err == nil {
		t.Fatal("cancellation did not stop query")
	}
	var n int
	if pool.QueryRow(ctx, "SELECT 1").Scan(&n) != nil || n != 1 {
		t.Fatal("pool not usable after cancellation")
	}
}

func TestSyntheticPostgresDenials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := syntheticPool(t, ctx)
	// A NOLOGIN role in this freshly created fixture only. No shared grants changed.
	for _, query := range []string{
		"SELECT id FROM qa_cp05.domain",
		"INSERT INTO qa_cp05.domain(id) VALUES ('denied')",
	} {
		err := Within(ctx, pool, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, "SET LOCAL ROLE qa_reader"); e != nil {
				t.Fatal("fixture role switch failed")
			}
			var marker string
			if e := tx.QueryRow(ctx, "SELECT value FROM qa_cp05.marker").Scan(&marker); e != nil || marker != os.Getenv("QA_PG_MARKER") {
				t.Fatal("approved fixture read failed")
			}
			_, e := tx.Exec(ctx, query)
			return e
		})
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "42501" {
			t.Fatal("expected real insufficient_privilege SQLSTATE")
		}
	}
	// Check destructive capabilities without executing destructive statements.
	var destructive bool
	if err := pool.QueryRow(ctx, "SELECT has_table_privilege('qa_reader','qa_cp05.domain','DELETE,TRUNCATE')").Scan(&destructive); err != nil || destructive {
		t.Fatal("fixture reader has destructive privilege")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM qa_cp05.domain WHERE id='denied'").Scan(&count); err != nil || count != 0 {
		t.Fatal("denied write persisted or connection failed to recover")
	}
}
