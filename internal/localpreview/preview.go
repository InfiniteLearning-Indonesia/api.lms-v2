// Package localpreview composes the Go application on a dedicated, synthetic
// loopback database. Its credentials and fixture privileges are local only.
package localpreview

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/migration/backup"
)

const Database = "preview_qa"
const Role = "preview_runtime"

var errTarget = errors.New("LOCAL_PREVIEW_TARGET_REJECTED")

type Config struct {
	Root, WebRoot, Marker, Password, Port, SystemID string
	DeliveryKey                                     []byte
}

func Load(env func(string) string) (Config, error) {
	c := Config{Root: env("LMS_PREVIEW_ROOT"), WebRoot: env("LMS_PREVIEW_WEB_ROOT"), Marker: env("LMS_PREVIEW_MARKER"),
		Password: env("LMS_PREVIEW_DB_PASSWORD"), Port: env("LMS_PREVIEW_DB_PORT"), SystemID: env("LMS_PREVIEW_DB_SYSTEM_ID")}
	key, err := hex.DecodeString(env("LMS_PREVIEW_DELIVERY_KEY"))
	port, portErr := strconv.Atoi(c.Port)
	systemID, idErr := strconv.ParseUint(c.SystemID, 10, 64)
	if err != nil || len(key) != 32 || idErr != nil || systemID == 0 || portErr != nil || port < 1024 || port > 65535 || strconv.Itoa(port) != c.Port ||
		!hexToken(c.Marker) || !hexToken(c.Password) || !backup.SafeRoot(c.Root) {
		return Config{}, errTarget
	}
	c.DeliveryKey = key
	return c, nil
}

func hexToken(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 64 && err == nil
}

func Open(ctx context.Context, c Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig("postgres://" + Role + "@127.0.0.1:" + c.Port + "/" + Database + "?sslmode=disable")
	if err != nil {
		return nil, errTarget
	}
	cfg.ConnConfig.Password, cfg.ConnConfig.Fallbacks = c.Password, nil
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "pg_catalog", "statement_timeout": "8s", "lock_timeout": "3s"}
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errTarget
	}
	if err = Ready(ctx, pool, c); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Ready validates the named fixture and confirms that its restricted runtime
// role cannot manage the database or delete application rows.
func Ready(ctx context.Context, pool *pgxpool.Pool, c Config) error {
	var marker, database, address, systemID string
	var port int
	var safe bool
	err := pool.QueryRow(ctx, `SELECT f.marker,current_database(),host(inet_server_addr()),(pg_control_system()).system_identifier::text,inet_server_port(),
 NOT (r.rolsuper OR r.rolbypassrls OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR pg_is_in_recovery())
 AND r.rolname='preview_runtime' AND NOT has_database_privilege(current_database(),'CREATE')
 AND NOT EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname IN ('v3','v3identity','v3password','v3google','preview_guard')
  AND has_schema_privilege(n.oid,'CREATE'))
 AND NOT EXISTS(SELECT 1 FROM pg_class t JOIN pg_namespace n ON n.oid=t.relnamespace
  WHERE n.nspname IN ('v3','v3identity','v3password','v3google','preview_guard') AND t.relkind IN ('r','p')
  AND has_table_privilege(t.oid,'DELETE,TRUNCATE'))
 AND to_regclass('v3.invitations') IS NOT NULL AND to_regclass('v3password.accounts') IS NOT NULL
 AND to_regclass('v3google.links') IS NOT NULL AND to_regclass('v3.grade_releases') IS NOT NULL
 AND EXISTS(SELECT 1 FROM v3identity.site_roles sr JOIN v3identity.links l USING(user_id)
  JOIN v3.users u ON u.id=sr.user_id WHERE sr.admin AND l.active AND u.active)
 FROM preview_guard.target f CROSS JOIN pg_roles r WHERE r.rolname=current_user`).Scan(&marker, &database, &address, &systemID, &port, &safe)
	if err != nil || marker != c.Marker || database != Database || address != "127.0.0.1" ||
		strconv.Itoa(port) != c.Port || systemID != c.SystemID || !safe {
		return errTarget
	}
	return nil
}
