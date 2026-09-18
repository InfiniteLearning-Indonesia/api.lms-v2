//go:build qaintegration

package foundation

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"lmsv3/internal/labdb"
)

func withRows(p *Plan, rows ...Row) {
	p.Rows = rows
	for i := range p.Observed {
		o := &p.Observed[i]
		if o.Kind != "TABLE" {
			continue
		}
		parts := []string{}
		for _, r := range rows {
			if r.Table == o.Name {
				parts = append(parts, r.PK+"\x00"+r.Version+"\x00"+r.RawSHA256)
			}
		}
		if len(parts) > 0 {
			sort.Strings(parts)
			o.Count = int64(len(parts))
			o.SHA256 = digest(strings.Join(parts, "\n"))
		}
	}
	p.Declared = append([]Object(nil), p.Observed...)
	inventoryHash, _, _ := inventory(p.Observed)
	p.SourceSnapshotSHA256 = syntheticSnapshotHash(inventoryHash)
}

func TestSyntheticPreflight(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	p := NewSyntheticPlan(labdb.TargetFromEnvironment(), "cp01-dry-run")
	var before, after int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3.audit").Scan(&before); err != nil {
		t.Fatal(err)
	}
	hash, err := Preflight(ctx, pool, p)
	if err != nil || !validHash(hash) {
		t.Fatal("valid dry-run rejected", err)
	}
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.ExpectedTarget = digest("wrong-target") },
		func(p *Plan) { p.SourceEnvironment = "production" },
		func(p *Plan) { p.Declared = p.Declared[:len(p.Declared)-1] },
		func(p *Plan) { p.Observed = p.Observed[1:] },
		func(p *Plan) { p.SourceSchemaSHA256 = "unverified" },
		func(p *Plan) { p.SourceSchemaSHA256 = digest("wrong-adapter") },
		func(p *Plan) { p.SourceSnapshotSHA256 = digest("wrong-snapshot") },
	} {
		bad := p
		bad.Observed = append([]Object(nil), p.Observed...)
		bad.Declared = append([]Object(nil), p.Declared...)
		mutate(&bad)
		if _, err = Preflight(ctx, pool, bad); !errors.Is(err, ErrPreflight) {
			t.Fatal("unsafe preflight accepted", err)
		}
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3.audit").Scan(&after); err != nil || before != after {
		t.Fatal("dry-run changed domain", before, after, err)
	}
	var schemaExists bool
	if err = pool.QueryRow(ctx, "SELECT to_regnamespace('v3migration') IS NOT NULL").Scan(&schemaExists); err != nil || schemaExists {
		t.Fatal("preflight installed schema", err)
	}
}

func TestSyntheticProvenance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = labdb.InstallMigrationFoundation(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallMigrationFoundation(ctx, pool); err != nil {
		t.Fatal("migration replay", err)
	}
	p := NewSyntheticPlan(labdb.TargetFromEnvironment(), "cp02-landing")
	withRows(&p, Row{Table: "users", PK: "same-pk", Version: "v1", RawSHA256: digest("user-private-bytes"), SealedRef: "sealed/users/1"}, Row{Table: "session", PK: "same-pk", Version: "v1", RawSHA256: digest("session-private-bytes"), SealedRef: "sealed/session/1"})
	if err = Capture(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if err = Capture(ctx, pool, p); err != nil {
		t.Fatal("same-plan replay", err)
	}
	var objects, rows, origins int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.scope_objects WHERE snapshot_id=$1", p.SnapshotID).Scan(&objects); err != nil || objects != 22 {
		t.Fatal("scope incomplete", objects, err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.landing_rows WHERE snapshot_id=$1", p.SnapshotID).Scan(&rows); err != nil || rows != 2 {
		t.Fatal("landing incomplete", rows, err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.origin_keys WHERE source_pk='same-pk'").Scan(&origins); err != nil || origins != 2 {
		t.Fatal("environment/table/namespace provenance collapsed", origins, err)
	}
	var readable bool
	if err = pool.QueryRow(ctx, `SELECT has_schema_privilege('qa_runtime','v3migration','USAGE') OR has_table_privilege('qa_runtime','v3migration.landing_rows','SELECT')`).Scan(&readable); err != nil || readable {
		t.Fatal("runtime can read restricted landing", err)
	}
	var column bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='v3migration' AND table_name='landing_rows' AND column_name IN ('raw_payload','secret','cookie'))`).Scan(&column); err != nil || column {
		t.Fatal("raw sensitive payload exposed", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE v3migration.landing_rows SET sealed_ref='changed' WHERE snapshot_id=$1", p.SnapshotID); err == nil {
		t.Fatal("immutable landing was changed")
	}
	bad := p
	bad.Rows = append([]Row(nil), p.Rows...)
	bad.Rows[0].SealedRef = "sealed/users/altered"
	if err = Capture(ctx, pool, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	bad = p
	bad.SnapshotID = "cp02-next-version"
	bad.Rows = append([]Row(nil), p.Rows...)
	bad.Rows[0].Version = "v2"
	bad.Rows[0].RawSHA256 = digest("next-user-version")
	withRows(&bad, bad.Rows...)
	if err = Capture(ctx, pool, bad); err != nil {
		t.Fatal("new snapshot rejected", err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM v3migration.origin_keys WHERE source_pk='same-pk'").Scan(&origins); err != nil || origins != 2 {
		t.Fatal("origin key duplication", origins, err)
	}
}
