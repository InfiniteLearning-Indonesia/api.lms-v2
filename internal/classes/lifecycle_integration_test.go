//go:build qaintegration

package classes

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyntheticContextLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = labdb.InstallLifecycle(ctx, owner, "cp12"); err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallLifecycle(ctx, owner, "cp12"); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, `GRANT SELECT,INSERT,UPDATE ON v3.class_access,v3.enrollment_access TO qa_runtime; GRANT SELECT,INSERT ON v3.transition_events TO qa_runtime;`, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	writer, err := labdb.IdentityRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	seedText, err := os.ReadFile(filepath.Join(os.Getenv("QA_LAB_DIR"), "issuer-seed.private"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := hex.DecodeString(string(seedText))
	if err != nil || len(seed) != 32 {
		t.Fatal("key invalid")
	}
	key := ed25519.NewKeyFromSeed(seed)
	svc := &identity.Lifecycle{Pool: writer, Verifier: identity.Verifier{Issuer: "https://synthetic-issuer.invalid", Audience: "lms-v3-local", Keys: map[string]ed25519.PublicKey{"fixture-1": key.Public().(ed25519.PublicKey)}}}
	sign := func(subject, nonce, purpose string, version int64, active bool) string {
		jti, _ := identity.RandomToken()
		now := time.Now()
		c := identity.Assertion{RegisteredClaims: jwt.RegisteredClaims{Issuer: svc.Verifier.Issuer, Audience: jwt.ClaimStrings{svc.Verifier.Audience}, Subject: subject, ID: jti, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, Purpose: purpose, Nonce: nonce, Version: version, Active: &active}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
		token.Header["kid"] = "fixture-1"
		token.Header["typ"] = "lms-bridge+jwt"
		raw, _ := token.SignedString(key)
		return raw
	}
	login := func(subject string, version int64) identity.Credential {
		n, b, e := svc.Challenge(ctx)
		if e != nil {
			t.Fatal(e)
		}
		c, e := svc.Exchange(ctx, sign(subject, n, "login", version, true), b)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	admin := login("admin", 1)
	teacher := login("go-fixture", 3)
	store := NewContextual(runtime)
	execute := func(token string, c Command) (json.RawMessage, error) {
		id, _ := ID()
		return store.Execute(ctx, token, id, id, c)
	}
	raw, err := execute(admin.Token, Command{Action: "create", Name: "CP12 fence"})
	if err != nil {
		t.Fatal(err)
	}
	var cls Class
	if json.Unmarshal(raw, &cls) != nil {
		t.Fatal("Class response")
	}
	target := strings.Repeat("a", 32)
	if _, err = execute(admin.Token, Command{Action: "add", ClassID: cls.ID, UserID: target, Role: "teacher"}); err != nil {
		t.Fatal(err)
	}
	if _, err = execute(teacher.Token, Command{Action: "publish", ClassID: cls.ID, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = execute(admin.Token, Command{Action: "suspend", ClassID: cls.ID, UserID: target}); !errors.Is(err, ErrConflict) {
		t.Fatal("last teacher suspension accepted", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	writeDone := make(chan error, 1)
	store.failAfter = func(at string) error {
		if at == "domain" {
			close(entered)
			<-release
		}
		return nil
	}
	go func() {
		_, e := execute(teacher.Token, Command{Action: "edit", ClassID: cls.ID, Name: "Committed before revoke", Version: 2})
		writeDone <- e
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("write barrier not reached")
	}
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- svc.Project(ctx, sign("go-fixture", "", "account_state", 4, false)) }()
	// Observe an actual waiting exclusive lock, not merely a goroutine timer.
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=1112001 AND mode='ExclusiveLock' AND NOT granted)").Scan(&waiting); err != nil {
			close(release)
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		close(release)
		t.Fatal("revocation was not fenced by in-flight write")
	}
	close(release)
	if err = <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err = <-revokeDone; err != nil {
		t.Fatal(err)
	}
	store.failAfter = nil
	if _, err = execute(teacher.Token, Command{Action: "edit", ClassID: cls.ID, Name: "Forbidden after revoke", Version: 3}); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatal("post-revoke write accepted", err)
	}
	var name string
	if err = owner.QueryRow(ctx, "SELECT name FROM v3.classes WHERE id=$1", cls.ID).Scan(&name); err != nil || name != "Committed before revoke" {
		t.Fatal("fenced state wrong")
	}
	if err = svc.Project(ctx, sign("go-fixture", "", "account_state", 5, true)); err != nil {
		t.Fatal(err)
	}
	teacher = login("go-fixture", 5)
	// Keep a second Teacher so the real end command preserves the invariant.
	var second string
	if err = owner.QueryRow(ctx, "SELECT user_id FROM v3identity.links WHERE subject='teacher'").Scan(&second); err != nil {
		t.Fatal(err)
	}
	if _, err = execute(admin.Token, Command{Action: "add", ClassID: cls.ID, UserID: second, Role: "teacher"}); err != nil {
		t.Fatal(err)
	}
	endEntered := make(chan struct{})
	endRelease := make(chan struct{})
	endDone := make(chan error, 1)
	store.failAfter = func(at string) error {
		if at == "domain" {
			close(endEntered)
			<-endRelease
		}
		return nil
	}
	go func() {
		_, e := execute(admin.Token, Command{Action: "end", ClassID: cls.ID, UserID: target})
		endDone <- e
	}()
	select {
	case <-endEntered:
	case <-ctx.Done():
		t.Fatal("end barrier not reached")
	}
	editDone := make(chan error, 1)
	go func() {
		_, e := execute(teacher.Token, Command{Action: "edit", ClassID: cls.ID, Name: "Role must be current", Version: 3})
		editDone <- e
	}()
	waiting = false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='transactionid' AND NOT granted)").Scan(&waiting); err != nil {
			close(endRelease)
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(endRelease)
	if err = <-endDone; err != nil {
		t.Fatal(err)
	}
	if err = <-editDone; !errors.Is(err, ErrForbidden) {
		t.Fatal("stale role grant used", err)
	}
	store.failAfter = nil
	if !waiting {
		t.Fatal("edit did not wait for role transition")
	}
	var history int
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3.transition_events WHERE class_id=$1 AND action='end' AND previous_state='ACTIVE' AND next_state='ENDED'", cls.ID).Scan(&history); err != nil || history != 1 {
		t.Fatal("end history missing")
	}
}
