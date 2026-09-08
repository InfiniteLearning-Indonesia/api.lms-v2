//go:build qaintegration

package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lmsv3/internal/labdb"
	"lmsv3/internal/migration/backup"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSyntheticIdentityLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = labdb.InstallLifecycle(ctx, owner, "cp11"); err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallLifecycle(ctx, owner, "cp11"); err != nil {
		t.Fatal("install replay", err)
	}
	password := os.Getenv("QA_IDENTITY_PASSWORD")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(password) {
		t.Fatal("identity password required")
	}
	if _, err = owner.Exec(ctx, "CREATE ROLE qa_identity LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '"+password+"'", pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	grants := `GRANT USAGE ON SCHEMA v3identity TO qa_runtime,qa_identity;
GRANT SELECT ON ALL TABLES IN SCHEMA v3identity TO qa_runtime,qa_identity;
GRANT USAGE ON SCHEMA v3,qa_cp05 TO qa_identity;
GRANT SELECT ON v3.users,qa_cp05.marker TO qa_identity;
GRANT SELECT,INSERT ON v3.audit,v3.outbox TO qa_identity;
GRANT INSERT,UPDATE ON v3identity.links,v3identity.sessions,v3identity.challenges,v3identity.site_roles TO qa_identity;
GRANT INSERT ON v3identity.consumed,v3identity.account_events,v3identity.role_events TO qa_identity;`
	if _, err = owner.Exec(ctx, grants, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	actors := map[string]struct {
		ID string `json:"id"`
	}{}
	data, err := os.ReadFile(filepath.Join(os.Getenv("QA_LAB_DIR"), "actors.private.json"))
	if err != nil || json.Unmarshal(data, &actors) != nil {
		t.Fatal("actors missing")
	}
	for label, a := range actors {
		if _, err = owner.Exec(ctx, "INSERT INTO v3identity.links(user_id,issuer,subject,version,active,valid_until) VALUES($1,'https://synthetic-issuer.invalid',$2,1,$3,clock_timestamp())", a.ID, label, label != "disabled"); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, "INSERT INTO v3identity.site_roles(user_id,admin) VALUES($1,$2)", a.ID, label == "admin"); err != nil {
			t.Fatal(err)
		}
	}
	user := strings.Repeat("a", 32)
	if _, err = owner.Exec(ctx, "INSERT INTO v3.users(id) VALUES($1)", user); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, "INSERT INTO v3identity.links(user_id,issuer,subject,version,active,valid_until) VALUES($1,'https://synthetic-issuer.invalid','go-fixture',1,true,clock_timestamp())", user); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, "INSERT INTO v3identity.site_roles(user_id) VALUES($1)", user); err != nil {
		t.Fatal(err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	root := os.Getenv("QA_LAB_DIR")
	if backup.WriteNew(filepath.Join(root, "issuer-seed.private"), []byte(hex.EncodeToString(key.Seed()))) != nil || backup.WriteNew(filepath.Join(root, "issuer-public.txt"), []byte(hex.EncodeToString(pub))) != nil {
		t.Fatal("exclusive key artifact failed")
	}
	writer, err := labdb.IdentityRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	s := &Lifecycle{Pool: writer, Verifier: Verifier{Issuer: "https://synthetic-issuer.invalid", Audience: "lms-v3-local", Keys: map[string]ed25519.PublicKey{"fixture-1": pub}}}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	for _, pool := range []interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	}{runtime, writer} {
		_, e := pool.Exec(ctx, "INSERT INTO v3.users(id) VALUES('unauthorized')")
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "42501" {
			t.Fatal("identity users write denial missing")
		}
	}
	if _, e := runtime.Exec(ctx, "INSERT INTO v3identity.challenges(nonce_hash,binding_hash,expires_at) VALUES('denied','denied',now())"); e == nil {
		t.Fatal("Class role can issue identity")
	}
	sign := func(subject, nonce, purpose string, version int64, active bool) string {
		jti, _ := RandomToken()
		now := time.Now()
		c := Assertion{RegisteredClaims: jwt.RegisteredClaims{Issuer: s.Verifier.Issuer, Audience: jwt.ClaimStrings{s.Verifier.Audience}, Subject: subject, ID: jti, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, Purpose: purpose, Nonce: nonce, Version: version, Active: &active}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
		token.Header["kid"] = "fixture-1"
		token.Header["typ"] = "lms-bridge+jwt"
		raw, _ := token.SignedString(key)
		return raw
	}
	nonce, binding, err := s.Challenge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw := sign("go-fixture", nonce, "login", 1, true)
	for _, stage := range []string{"audit", "outbox"} {
		injected := errors.New("injected-" + stage)
		s.failAfter = func(at string) error {
			if at == stage {
				return injected
			}
			return nil
		}
		if _, e := s.Exchange(ctx, raw, binding); !errors.Is(e, injected) {
			t.Fatal("exact injection not reached", e)
		}
		var count int
		if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3identity.sessions WHERE user_id=$1", user).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial session")
		}
	}
	s.failAfter = nil
	var wg sync.WaitGroup
	results := make(chan Credential, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, e := s.Exchange(ctx, raw, binding); results <- c; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrConflict) {
			t.Fatal("unexpected exchange failure", e)
		}
	}
	if success != 1 {
		t.Fatal("single-use violated")
	}
	var credential Credential
	for c := range results {
		if c.Token != "" {
			credential = c
		}
	}
	rotated := make(chan Credential, 2)
	rotateErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := s.SessionCommand(ctx, credential.Token, credential.CSRF, "rotate")
			rotated <- c
			rotateErrors <- e
		}()
	}
	wg.Wait()
	close(rotated)
	close(rotateErrors)
	success = 0
	for e := range rotateErrors {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrUnauthenticated) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("rotation concurrent reuse")
	}
	for c := range rotated {
		if c.Token != "" {
			credential = c
		}
	}
	if err = s.Project(ctx, sign("go-fixture", "", "account_state", 2, false)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Validate(ctx, credential.Token, "", false); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled account accepted", err)
	}
	if err = s.Project(ctx, sign("go-fixture", "", "account_state", 1, true)); !errors.Is(err, ErrConflict) {
		t.Fatal("stale projection accepted")
	}
	if err = s.Project(ctx, sign("go-fixture", "", "account_state", 3, true)); err != nil {
		t.Fatal(err)
	}
	nonce, binding, err = s.Challenge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	credential, err = s.Exchange(ctx, sign("go-fixture", nonce, "login", 3, true), binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, "UPDATE v3identity.links SET valid_until=clock_timestamp()-interval '1 second' WHERE user_id=$1", user); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Validate(ctx, credential.Token, "", false); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("stale authority/outage access")
	}
	var n int
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3identity.sessions WHERE user_id=$1", user).Scan(&n); err != nil || n != 3 {
		t.Fatal("session history missing", n, err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3identity.account_events WHERE user_id=$1", user).Scan(&n); err != nil || n != 2 {
		t.Fatal("account version history missing")
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'identity.%' AND o.event_id IS NULL").Scan(&n); err != nil || n != 0 {
		t.Fatal("audit/outbox mismatch")
	}
	login := func(subject string) Credential {
		n, b, e := s.Challenge(ctx)
		if e != nil {
			t.Fatal(e)
		}
		c, e := s.Exchange(ctx, sign(subject, n, "login", 1, true), b)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	a := login("admin")
	b := login("teacher2")
	aID, bID := actors["admin"].ID, actors["teacher2"].ID
	for _, stage := range []string{"audit", "outbox"} {
		injected := errors.New("authority-" + stage)
		s.failAfter = func(at string) error {
			if at == stage {
				return injected
			}
			return nil
		}
		if e := s.SiteRole(ctx, a.Token, a.CSRF, bID, true, 1); !errors.Is(e, injected) {
			t.Fatal("site role rollback", e)
		}
		if e := s.Project(ctx, sign("teacher2", "", "account_state", 2, false)); !errors.Is(e, injected) {
			t.Fatal("projection rollback", e)
		}
		for _, action := range []string{"logout", "revoke-all"} {
			if _, e := s.SessionCommand(ctx, b.Token, b.CSRF, action); !errors.Is(e, injected) {
				t.Fatal("revoke rollback", e)
			}
		}
		actor, e := s.Validate(ctx, b.Token, "", false)
		if e != nil || actor.Admin {
			t.Fatal("authority partially changed", e)
		}
		var version int64
		if e = owner.QueryRow(ctx, "SELECT version FROM v3identity.links WHERE user_id=$1", bID).Scan(&version); e != nil || version != 1 {
			t.Fatal("projection version rollback")
		}
	}
	s.failAfter = nil
	if err = s.SiteRole(ctx, a.Token, a.CSRF, bID, true, 1); err != nil {
		t.Fatal(err)
	}
	// Two admins concurrently demote themselves. Exactly one must remain.
	demotions := make(chan error, 2)
	go func() { demotions <- s.SiteRole(ctx, a.Token, a.CSRF, aID, false, 1) }()
	go func() { demotions <- s.SiteRole(ctx, b.Token, b.CSRF, bID, false, 2) }()
	success = 0
	for i := 0; i < 2; i++ {
		e := <-demotions
		if e == nil {
			success++
		} else if !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("last admin concurrency invariant")
	}
	var aAdmin bool
	if err = owner.QueryRow(ctx, "SELECT admin FROM v3identity.site_roles WHERE user_id=$1", aID).Scan(&aAdmin); err != nil {
		t.Fatal(err)
	}
	if !aAdmin { // Restore fixture through the real command, preserving history.
		if err = s.SiteRole(ctx, b.Token, b.CSRF, aID, true, 2); err != nil {
			t.Fatal(err)
		}
		if err = s.SiteRole(ctx, a.Token, a.CSRF, bID, false, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3identity.site_roles WHERE admin").Scan(&n); err != nil || n != 1 {
		t.Fatal("admin reconciliation")
	}
	// Sessions store hashes, and rotation retains the original absolute deadline.
	var hash, csrfHash string
	var absolute time.Time
	if err = owner.QueryRow(ctx, "SELECT token_hash,csrf_hash,absolute_until FROM v3identity.sessions WHERE token_hash=$1", TokenHash(a.Token)).Scan(&hash, &csrfHash, &absolute); err != nil || hash == a.Token || csrfHash == a.CSRF {
		t.Fatal("credential hashing")
	}
	for _, stage := range []string{"audit", "outbox"} {
		injected := errors.New("rotate-" + stage)
		s.failAfter = func(at string) error {
			if at == stage {
				return injected
			}
			return nil
		}
		if _, e := s.SessionCommand(ctx, a.Token, a.CSRF, "rotate"); !errors.Is(e, injected) {
			t.Fatal("rotation rollback", e)
		}
		if _, e := s.Validate(ctx, a.Token, "", false); e != nil {
			t.Fatal("rotation rollback revoked original")
		}
	}
	s.failAfter = nil
	r, e := s.SessionCommand(ctx, a.Token, a.CSRF, "rotate")
	if e != nil || !r.Absolute.Equal(absolute) {
		t.Fatal("absolute deadline changed", e)
	}
	if _, err = owner.Exec(ctx, "UPDATE v3identity.sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1", TokenHash(r.Token)); err != nil {
		t.Fatal(err)
	}
	if _, e = s.Validate(ctx, r.Token, "", false); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("expired session accepted")
	}
	var rawRows int
	if err = owner.QueryRow(ctx, `SELECT count(*) FROM v3identity.sessions WHERE token_hash !~ '^[a-f0-9]{64}$' OR csrf_hash !~ '^[a-f0-9]{64}$'`).Scan(&rawRows); err != nil || rawRows != 0 {
		t.Fatal("invalid stored credential format")
	}
}
