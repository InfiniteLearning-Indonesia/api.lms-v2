package dbinspect

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Report struct {
	Version                        int             `json:"version"`
	Status                         string          `json:"status"`
	ObservedAt                     string          `json:"observed_at"`
	Environment                    string          `json:"environment"`
	ConnectionFingerprint          string          `json:"connection_fingerprint"`
	DatabaseNameHash               string          `json:"database_name_hash,omitempty"`
	RoleNameHash                   string          `json:"role_name_hash,omitempty"`
	ServerVersion                  int             `json:"server_version,omitempty"`
	TLSVerified                    bool            `json:"tls_verified"`
	PeerCertificateSHA256          string          `json:"peer_certificate_sha256,omitempty"`
	ReadOnlyTransaction            bool            `json:"read_only_transaction"`
	RoleFlags                      map[string]bool `json:"role_flags,omitempty"`
	LeastPrivilegeVerified         bool            `json:"least_privilege_verified"`
	ProductionDistinctnessVerified bool            `json:"production_distinctness_verified"`
	Catalog                        json.RawMessage `json:"public_tables,omitempty"`
	CatalogSHA256                  string          `json:"catalog_sha256,omitempty"`
	DatabaseBytes                  int64           `json:"database_bytes,omitempty"`
	Statistics                     json.RawMessage `json:"public_table_statistics,omitempty"`
	Extensions                     json.RawMessage `json:"extensions,omitempty"`
	Dependencies                   json.RawMessage `json:"dependency_metadata,omitempty"`
	DependenciesSHA256             string          `json:"dependency_metadata_sha256,omitempty"`
	Blockers                       []string        `json:"blockers"`
	Mutations                      string          `json:"database_mutations"`
}

func Inspect(ctx context.Context, s Settings) Report {
	r := Report{Version: 1, Status: "BLOCKED", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Environment: "USER_IDENTIFIED_STAGING", ConnectionFingerprint: s.Fingerprint(), Mutations: "NONE",
		Blockers: []string{"PRODUCTION_IDENTITY_NOT_VERIFIED", "LEAST_PRIVILEGE_NOT_CERTIFIED", "BACKUP_RESTORE_NOT_VERIFIED", "FULL_SCHEMA_JOBS_STORAGE_REVIEW_PENDING", "DATA_API_EXPOSURE_CONFIG_NOT_VERIFIED"}}
	fail := func(code string) Report { r.Blockers = append(r.Blockers, code); return r }
	cfg, err := s.DriverConfig()
	if err != nil {
		return fail(err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fail(SafeError(err))
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	tlsConn, ok := conn.PgConn().Conn().(*tls.Conn)
	if !ok {
		return fail("TLS_CONNECTION_NOT_VERIFIED")
	}
	state := tlsConn.ConnectionState()
	if !state.HandshakeComplete || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return fail("TLS_CHAIN_NOT_VERIFIED")
	}
	r.TLSVerified = true
	r.PeerCertificateSHA256 = digest(state.PeerCertificates[0].Raw)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fail(SafeError(err))
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	// These settings are transaction-local, not DDL or persisted config changes.
	for _, sql := range []string{"SET LOCAL statement_timeout = '5s'", "SET LOCAL lock_timeout = '1s'", "SET LOCAL idle_in_transaction_session_timeout = '5s'", "SET LOCAL search_path = pg_catalog"} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return fail(SafeError(err))
		}
	}
	var database, role, readonly string
	var super, bypass, createdb, createrole, createSchema bool
	err = tx.QueryRow(ctx, identitySQL).Scan(&database, &role, &r.ServerVersion, &readonly, &super, &bypass, &createdb, &createrole, &createSchema)
	if err != nil {
		return fail(SafeError(err))
	}
	if database != s.database {
		return fail("SERVER_DATABASE_NAME_MISMATCH")
	}
	if readonly != "on" {
		return fail("READ_ONLY_TRANSACTION_NOT_VERIFIED")
	}
	r.ReadOnlyTransaction = true
	r.DatabaseNameHash = digest([]byte(database))
	r.RoleNameHash = digest([]byte(role))
	r.RoleFlags = map[string]bool{"superuser": super, "bypass_rls": bypass, "create_database": createdb, "create_role": createrole, "create_in_public_schema": createSchema}
	if err = tx.QueryRow(ctx, catalogSQL).Scan(&r.Catalog); err != nil {
		return fail(SafeError(err))
	}
	if err = tx.QueryRow(ctx, statisticsSQL).Scan(&r.DatabaseBytes, &r.Statistics); err != nil {
		return fail(SafeError(err))
	}
	if err = tx.QueryRow(ctx, extensionsSQL).Scan(&r.Extensions); err != nil {
		return fail(SafeError(err))
	}
	r.CatalogSHA256 = digest(r.Catalog)
	if err = tx.QueryRow(ctx, dependenciesSQL).Scan(&r.Dependencies); err != nil {
		return fail(SafeError(err))
	}
	r.DependenciesSHA256 = digest(r.Dependencies)
	if err = tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fail(SafeError(err))
	}
	r.Status = "READ_ONLY_OBSERVATION_COMPLETE"
	return r
}
