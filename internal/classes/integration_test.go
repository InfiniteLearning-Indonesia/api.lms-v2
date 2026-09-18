//go:build qaintegration

package classes

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixtureActor struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func TestSyntheticRuntimeFoundation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal("operator fixture unavailable")
	}
	defer owner.Close()
	if labdb.Install(ctx, owner, strings.Repeat("0", 64)) == nil {
		t.Fatal("checksum mismatch accepted")
	}
	root := os.Getenv("QA_LAB_DIR")
	data, err := os.ReadFile(filepath.Join(root, "pre-v3-proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof labdb.Proof
	if json.Unmarshal(data, &proof) != nil {
		t.Fatal("proof invalid")
	}
	for _, bad := range []labdb.Proof{{RunID: proof.RunID, Target: "wrong", CreatedAt: proof.CreatedAt, ArchiveHash: proof.ArchiveHash, Restored: true}, {RunID: proof.RunID, Target: proof.Target, CreatedAt: time.Now().Add(-2 * time.Hour), ArchiveHash: proof.ArchiveHash, Restored: true}, {RunID: proof.RunID, Target: proof.Target, CreatedAt: proof.CreatedAt, ArchiveHash: strings.Repeat("0", 64), Restored: true}} {
		if labdb.ValidateProof(bad, root, proof.Target, time.Now()) == nil {
			t.Fatal("invalid backup accepted")
		}
	}
	held, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = held.Exec(ctx, "SELECT pg_advisory_xact_lock(810910)"); err != nil {
		t.Fatal(err)
	}
	if labdb.Install(ctx, owner, labdb.Checksum()) == nil {
		t.Fatal("migration lock ignored")
	}
	if err = held.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = labdb.Install(ctx, owner, labdb.Checksum()); err != nil {
		t.Fatal("fixture schema install failed:", err)
	}
	if err = labdb.Install(ctx, owner, labdb.Checksum()); err != nil {
		t.Fatal("migration replay failed")
	}
	var n int
	if owner.QueryRow(ctx, "SELECT count(*) FROM v3ops.migrations WHERE version='class-slice-1'").Scan(&n) != nil || n != 1 {
		t.Fatal("duplicate migration")
	}
	actors := map[string]fixtureActor{}
	data, err = os.ReadFile(filepath.Join(root, "actors.private.json"))
	if err != nil || json.Unmarshal(data, &actors) != nil {
		t.Fatal("actors required")
	}
	for label, a := range actors {
		if !idPattern.MatchString(a.ID) || !opaqueToken(a.Token) {
			t.Fatal("fixture actor invalid")
		}
		_, err = owner.Exec(ctx, "INSERT INTO v3.users(id,active,site_admin) VALUES($1,$2,$3)", a.ID, label != "disabled", label == "admin")
		if err != nil {
			t.Fatal("actor seed failed")
		}
		expiry := time.Now().Add(time.Hour)
		var revoked any
		if label == "expired" {
			expiry = time.Now().Add(-time.Hour)
		}
		if label == "revoked" {
			revoked = time.Now()
		}
		_, err = owner.Exec(ctx, "INSERT INTO v3.sessions(token_hash,user_id,expires_at,revoked_at) VALUES($1,$2,$3,$4)", identity.TokenHash(a.Token), a.ID, expiry, revoked)
		if err != nil {
			t.Fatal("session seed failed")
		}
	}
	password := os.Getenv("QA_RUNTIME_PASSWORD")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(password) {
		t.Fatal("generated runtime password required")
	}
	_, err = owner.Exec(ctx, "CREATE ROLE qa_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '"+password+"'", pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal("runtime fixture role failed")
	}
	_, err = owner.Exec(ctx, `GRANT USAGE ON SCHEMA v3,v3ops,qa_cp05 TO qa_runtime;
 GRANT SELECT ON v3.users,v3.sessions,qa_cp05.marker TO qa_runtime;
 GRANT SELECT ON v3ops.migrations TO qa_runtime;
 GRANT SELECT,INSERT ON v3.audit TO qa_runtime;
 GRANT SELECT,INSERT,UPDATE ON v3.classes,v3.enrollments,v3.roles,v3.episodes,v3.commands TO qa_runtime;
 GRANT SELECT,INSERT ON v3.outbox TO qa_runtime;`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal("runtime fixture grants failed")
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		t.Fatal("restricted runtime failed:", err)
	}
	defer runtime.Close()
	store := New(runtime)
	admin := actors["admin"].Token
	// Metadata proof of destructive/DDL denial; never issue destructive SQL.
	var unsafe bool
	err = runtime.QueryRow(ctx, "SELECT has_schema_privilege(current_user,'v3','CREATE') OR has_table_privilege(current_user,'v3.classes','DELETE,TRUNCATE') OR has_table_privilege(current_user,'v3.audit','UPDATE')").Scan(&unsafe)
	if err != nil || unsafe {
		t.Fatal("runtime overly privileged")
	}
	// Actual denial of a harmless privileged write, rolled back, SQLSTATE-specific.
	_, err = runtime.Exec(ctx, "INSERT INTO v3.users(id) VALUES('denied-runtime-user')")
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("runtime denial absent")
	}
	for _, stage := range []string{"domain", "audit", "outbox"} {
		injected := errors.New("injected-" + stage)
		store.failAfter = func(at string) error {
			if at == stage {
				return injected
			}
			return nil
		}
		request, _ := ID()
		if _, err = store.Execute(ctx, admin, "atomic-"+stage, request, Command{Action: "create", Name: "failed-" + stage}); !errors.Is(err, injected) {
			t.Fatal("injection stage not reached or failure committed")
		}
		if owner.QueryRow(ctx, "SELECT count(*) FROM v3.classes WHERE name=$1", "failed-"+stage).Scan(&n) != nil || n != 0 {
			t.Fatal("partial Class")
		}
	}
	store.failAfter = nil
	for _, table := range []string{"audit", "outbox", "commands"} {
		if owner.QueryRow(ctx, "SELECT count(*) FROM v3."+table).Scan(&n) != nil || n != 0 {
			t.Fatal("partial companion write")
		}
	}
	var wg sync.WaitGroup
	answers := make(chan string, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request, _ := ID()
			body, err := store.Execute(ctx, admin, "concurrent-create", request, Command{Action: "create", Name: "concurrent"})
			if err != nil {
				failures <- err
				return
			}
			var c Class
			if json.Unmarshal(body, &c) != nil {
				failures <- errors.New("invalid result")
				return
			}
			answers <- c.ID
		}()
	}
	wg.Wait()
	close(answers)
	close(failures)
	for failure := range failures {
		var pgerr *pgconn.PgError
		if errors.As(failure, &pgerr) {
			t.Fatal("concurrent command failed SQLSTATE", pgerr.Code)
		}
		t.Fatalf("concurrent command failed (%T)", failure)
	}
	same := ""
	for id := range answers {
		if same != "" && same != id {
			t.Fatal("duplicate Class")
		}
		same = id
	}
	if owner.QueryRow(ctx, "SELECT count(*) FROM v3.classes").Scan(&n) != nil || n != 1 {
		t.Fatal("concurrent duplicate rows")
	}
	request, _ := ID()
	if _, err = store.Execute(ctx, admin, "concurrent-create", request, Command{Action: "create", Name: "conflicting"}); !errors.Is(err, ErrConflict) {
		t.Fatal("key collision not rejected")
	}
	request, _ = ID()
	if _, err = store.Execute(ctx, admin, "second-event", request, Command{Action: "create", Name: "second"}); err != nil {
		t.Fatal(err)
	}
	if owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id").Scan(&n) != nil || n != 2 {
		t.Fatal("events not retained")
	}
}
