// Package database owns bounded runtime connections, not migrations or dotenv.
package database

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

func Validate(c *pgx.ConnConfig, synthetic bool) error {
	expectedUser := ""
	if synthetic {
		expectedUser = "qa_runtime"
	}
	return validate(c, synthetic, expectedUser)
}

func validate(c *pgx.ConnConfig, synthetic bool, expectedUser string) error {
	if c == nil || c.Host == "" || c.User == "" || c.Database == "" || len(c.Fallbacks) > 0 {
		return errors.New("RUNTIME_CONFIG_REJECTED")
	}
	if synthetic {
		if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != expectedUser || c.Port < 1024 {
			return errors.New("SYNTHETIC_TARGET_REJECTED")
		}
	} else if c.TLSConfig == nil || c.TLSConfig.InsecureSkipVerify || c.TLSConfig.ServerName != c.Host || c.TLSConfig.MinVersion < tls.VersionTLS12 {
		return errors.New("RUNTIME_TLS_REQUIRED")
	}
	return nil
}

// Open accepts an already-verified TLS config; the separate synthetic path is
// available only for the exact local harness identity, never a shared DSN.
func Open(ctx context.Context, c *pgx.ConnConfig) (*pgxpool.Pool, error) {
	return open(ctx, c, false, "", "lmsv3-class-runtime", 4)
}
func OpenSynthetic(ctx context.Context, c *pgx.ConnConfig, ack string) (*pgxpool.Pool, error) {
	if ack != "new-local-synthetic-cluster" {
		return nil, errors.New("SYNTHETIC_ACK_REQUIRED")
	}
	return open(ctx, c, true, "qa_runtime", "lmsv3-class-runtime", 4)
}

func OpenSyntheticWorker(ctx context.Context, c *pgx.ConnConfig, ack string) (*pgxpool.Pool, error) {
	if ack != "new-local-synthetic-cluster" {
		return nil, errors.New("SYNTHETIC_ACK_REQUIRED")
	}
	return open(ctx, c, true, "qa_worker", "lmsv3-outbox-worker", 2)
}

func open(ctx context.Context, c *pgx.ConnConfig, synthetic bool, expectedUser, applicationName string, maxConns int32) (*pgxpool.Pool, error) {
	if err := validate(c, synthetic, expectedUser); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig("postgres://unused@127.0.0.1/unused?sslmode=disable")
	if err != nil {
		return nil, errors.New("POOL_CONFIG_FAILED")
	}
	cfg.ConnConfig = c.Copy()
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "pg_catalog", "statement_timeout": "3000", "lock_timeout": "1000", "idle_in_transaction_session_timeout": "5000", "application_name": applicationName}
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.ConnConfig.StatementCacheCapacity = 0
	cfg.ConnConfig.DescriptionCacheCapacity = 0
	cfg.ConnConfig.Tracer = nil
	cfg.MinConns = 0
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = 15 * time.Minute
	cfg.MaxConnIdleTime = time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("POOL_OPEN_FAILED")
	}
	if expectedUser == "qa_worker" {
		return pool, nil
	}
	var unsafe bool
	err = pool.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreatedb OR r.rolcreaterole
 OR has_schema_privilege(current_user,'v3','CREATE')
 OR has_schema_privilege(current_user,'public','CREATE')
 OR has_table_privilege(current_user,'v3.users','INSERT,UPDATE,DELETE,TRUNCATE')
 OR has_table_privilege(current_user,'v3.sessions','INSERT,UPDATE,DELETE,TRUNCATE')
 OR has_table_privilege(current_user,'v3.audit','UPDATE,DELETE,TRUNCATE')
	OR has_table_privilege(current_user,'v3.outbox','UPDATE,DELETE,TRUNCATE')
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='v3' AND c.relkind='r' AND has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE'))
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN ('v3','v3ops') AND c.relowner=r.oid)
 FROM pg_roles r WHERE r.rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		pool.Close()
		return nil, errors.New("RUNTIME_PRIVILEGES_REJECTED")
	}
	return pool, nil
}
