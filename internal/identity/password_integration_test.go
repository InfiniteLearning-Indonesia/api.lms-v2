//go:build password_rehearsal

package identity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

const fixtureAdmin = "00000000000000000000000000000001"
const fixtureTeacher = "00000000000000000000000000000002"
const learnerEmail = "password-fixture@example.invalid"
const firstPassword = "first test password with spaces"
const secondPassword = "second test password with spaces"

func connectPasswordFixture(t *testing.T, ctx context.Context, user, password string) *pgxpool.Pool {
	t.Helper()
	port := os.Getenv("QA_M04_PORT")
	number, err := strconv.Atoi(port)
	if err != nil || number < 1024 || number > 65535 || len(password) != 64 {
		t.Fatal("fixture port or password missing")
	}
	cfg, err := pgxpool.ParseConfig("postgres://" + user + "@127.0.0.1:" + port + "/password_qa?sslmode=disable")
	if err != nil {
		t.Fatal("fixture configuration rejected")
	}
	cfg.ConnConfig.Host, cfg.ConnConfig.Port = "127.0.0.1", uint16(number)
	cfg.ConnConfig.Database, cfg.ConnConfig.User = "password_qa", user
	cfg.ConnConfig.Password, cfg.ConnConfig.TLSConfig, cfg.ConnConfig.Fallbacks = password, nil, nil
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "pg_catalog", "statement_timeout": "6s", "lock_timeout": "3s"}
	cfg.MaxConns = 6
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture connection rejected")
	}
	t.Cleanup(pool.Close)
	return pool
}

func fixtureExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			t.Fatalf("fixture statement failed: SQLSTATE=%s table=%s column=%s constraint=%s", pgErr.Code, pgErr.TableName, pgErr.ColumnName, pgErr.ConstraintName)
		}
		t.Fatalf("fixture statement failed: %T", err)
	}
}

func newPasswordFixture(t *testing.T, ctx context.Context) (admin *pgxpool.Pool, lifecycle *identity.Lifecycle, passwords *identity.Passwords, sessions, csrf [2]string, key [32]byte) {
	t.Helper()
	admin = connectPasswordFixture(t, ctx, "lms_baseline_admin", os.Getenv("QA_M04_ADMIN"))
	var marker, directory, host, database string
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT marker,current_setting('data_directory'),host(inet_server_addr()),current_database(),
  to_regnamespace('v3') IS NOT NULL FROM qa_password_guard.target`).Scan(&marker, &directory, &host, &database, &exists); err != nil ||
		len(marker) != 64 || marker != os.Getenv("QA_M04_MARKER") || !strings.EqualFold(filepath.Clean(directory), filepath.Clean(os.Getenv("QA_M04_CLUSTER"))) ||
		host != "127.0.0.1" || database != "password_qa" || exists {
		t.Fatal("fresh target identity guard rejected")
	}
	fixtureExec(t, ctx, admin, labdb.CoreDDL()+"\n"+labdb.IdentityDDL()+"\n"+identity.PasswordSchema, pgx.QueryExecModeSimpleProtocol)
	fixtureExec(t, ctx, admin, `GRANT USAGE ON SCHEMA v3,v3identity,v3password TO m04_password_writer;
  GRANT SELECT,INSERT ON v3.users TO m04_password_writer;
  GRANT UPDATE(active) ON v3.users TO m04_password_writer;
  GRANT INSERT ON v3.audit,v3.outbox TO m04_password_writer;
  GRANT SELECT,INSERT ON v3identity.links,v3identity.site_roles,v3identity.sessions TO m04_password_writer;
  GRANT UPDATE(active) ON v3identity.links TO m04_password_writer;
  GRANT UPDATE(revoked_at) ON v3identity.sessions TO m04_password_writer;
  GRANT SELECT,INSERT ON ALL TABLES IN SCHEMA v3password TO m04_password_writer;
  GRANT UPDATE(activated_at,suspended_at) ON v3password.accounts TO m04_password_writer;
  GRANT UPDATE(salt,password_hash,updated_at) ON v3password.credentials TO m04_password_writer;
  GRANT UPDATE(consumed_at) ON v3password.tokens TO m04_password_writer;
  GRANT UPDATE(window_start,attempts) ON v3password.limits TO m04_password_writer;`, pgx.QueryExecModeSimpleProtocol)
	for i, user := range []string{fixtureAdmin, fixtureTeacher} {
		sessions[i], _ = identity.RandomToken()
		csrf[i], _ = identity.RandomToken()
		fixtureExec(t, ctx, admin, "INSERT INTO v3.users(id) VALUES($1)", user)
		fixtureExec(t, ctx, admin, `INSERT INTO v3identity.links(user_id,issuer,subject,version,active,valid_until)
   VALUES($1,'synthetic:password-qa',$1,1,true,'infinity')`, user)
		fixtureExec(t, ctx, admin, "INSERT INTO v3identity.site_roles(user_id,admin) VALUES($1,$2)", user, i == 0)
		fixtureExec(t, ctx, admin, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until)
   VALUES($1,$2,1,$3,clock_timestamp()+interval '10 minutes',clock_timestamp()+interval '1 hour')`,
			identity.TokenHash(sessions[i]), user, identity.TokenHash(csrf[i]))
	}
	fixtureExec(t, ctx, admin, "INSERT INTO v3.classes(id,name) VALUES('fixture-class','Synthetic Class')")
	fixtureExec(t, ctx, admin, "INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES('fixture-enrollment','fixture-class',$1,'ACTIVE')", fixtureTeacher)
	fixtureExec(t, ctx, admin, "INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES('fixture-role','fixture-enrollment','fixture-class','teacher')")
	writer := connectPasswordFixture(t, ctx, "m04_password_writer", os.Getenv("QA_M04_WRITER"))
	var unsafe bool
	if err := writer.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole
  OR has_schema_privilege(current_user,'v3password','CREATE')
  OR has_table_privilege(current_user,'v3password.accounts','DELETE,TRUNCATE')
  FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("runtime role has unexpected privileges")
	}
	lifecycle = &identity.Lifecycle{Pool: writer}
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal("fixture key generation failed")
	}
	var err error
	passwords, err = identity.NewPasswords(lifecycle, key[:])
	if err != nil {
		t.Fatal("password authority configuration rejected")
	}
	return
}

func fixtureDelivery(t *testing.T, ctx context.Context, admin *pgxpool.Pool, passwords *identity.Passwords, user, purpose string) string {
	t.Helper()
	var id, hash string
	var payload []byte
	if err := admin.QueryRow(ctx, `SELECT d.id,t.token_hash,d.payload FROM v3password.deliveries d
 JOIN v3password.tokens t ON t.token_hash=d.token_hash
 WHERE t.user_id=$1 AND t.purpose=$2 AND t.consumed_at IS NULL`, user, purpose).Scan(&id, &hash, &payload); err != nil {
		t.Fatal("delivery evidence missing")
	}
	token, err := passwords.OpenDelivery(id, hash, user, purpose, payload)
	if err != nil || token == hash || bytes.Contains(payload, []byte(token)) {
		t.Fatal("delivery integrity failed")
	}
	return token
}

func TestM04PasswordPostgresJourney(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	admin, lifecycle, passwords, sessions, csrf, key := newPasswordFixture(t, ctx)
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	server.Config.Handler = identity.GatewayWithPasswords(lifecycle, passwords, origin, http.NotFoundHandler(), logger)
	server.StartTLS()
	defer server.Close()

	probe := exec.CommandContext(ctx, os.Getenv("QA_M04_PYTHON"), "-B", os.Getenv("QA_M04_API_PROBE"))
	payload, err := json.Marshal(map[string]string{"origin": origin})
	if err != nil {
		t.Fatal("fixture origin could not be encoded")
	}
	probe.Stdin = bytes.NewReader(payload)
	if output, err := probe.Output(); err != nil || strings.TrimSpace(string(output)) != "PASS_M04_PASSWORD_PLAYWRIGHT_5" {
		t.Fatal("Playwright request smoke failed")
	}

	type reply struct {
		status int
		cookie string
		body   map[string]any
	}
	request := func(method, path, session, csrfValue, requestOrigin string, input map[string]string) (reply, error) {
		var result reply
		data, err := json.Marshal(input)
		if err != nil {
			return result, err
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+"/v3/auth/"+path, bytes.NewReader(data))
		if err != nil {
			return result, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", requestOrigin)
		req.Header.Set("X-CSRF-Token", csrfValue)
		if session != "" {
			req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: session})
		}
		response, err := server.Client().Do(req)
		if err != nil {
			return result, err
		}
		defer response.Body.Close()
		result.status = response.StatusCode
		for _, cookie := range response.Cookies() {
			if cookie.Name == identity.SessionCookie && cookie.Secure && cookie.HttpOnly && cookie.SameSite == http.SameSiteStrictMode {
				result.cookie = cookie.Value
			}
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result.body)
		return result, err
	}
	run := func(status int, path, session, csrfValue string, body map[string]string) reply {
		t.Helper()
		r, err := request("POST", path, session, csrfValue, origin, body)
		if err != nil || r.status != status {
			t.Fatalf("%s expected %d, got %d", path, status, r.status)
		}
		return r
	}
	input := map[string]string{"email": learnerEmail}
	run(403, "accounts/invite", sessions[1], csrf[1], input)
	run(403, "accounts/invite", sessions[0], "", input)
	if r, e := request("POST", "accounts/invite", sessions[0], csrf[0], "https://foreign.invalid", input); e != nil || r.status != 403 {
		t.Fatal("foreign origin was accepted")
	}
	r := run(202, "accounts/invite", sessions[0], csrf[0], input)
	user, ok := r.body["user_id"].(string)
	if !ok || len(user) != 32 || len(r.body) != 2 {
		t.Fatal("invalid invitation response")
	}
	inviteToken := fixtureDelivery(t, ctx, admin, passwords, user, "invite")
	run(401, "password/login", "", "", map[string]string{"email": learnerEmail, "password": firstPassword})
	run(401, "password/reset/confirm", "", "", map[string]string{"token": inviteToken, "password": firstPassword})
	first := map[string]string{"token": inviteToken, "password": firstPassword}
	var concurrent sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		concurrent.Add(1)
		go func() {
			defer concurrent.Done()
			r, err := request("POST", "password/invite/accept", "", "", origin, first)
			if err != nil {
				results <- 0
				return
			}
			results <- r.status
		}()
	}
	concurrent.Wait()
	if a, b := <-results, <-results; !((a == 200 && b == 401) || (a == 401 && b == 200)) {
		t.Fatal("token consumption did not have exactly one winner")
	}
	run(409, "accounts/invite", sessions[0], csrf[0], input)
	logIn := func(password string) reply {
		return run(200, "password/login", "", "", map[string]string{"email": " PASSWORD-FIXTURE@example.invalid ", "password": password})
	}
	r = logIn(firstPassword)
	if r.cookie == "" || r.body["csrf_token"] == nil {
		t.Fatal("secure session contract missing")
	}
	firstSession := r.cookie
	known := run(202, "password/reset/request", "", "", input)
	unknown := run(202, "password/reset/request", "", "", map[string]string{"email": "absent@example.invalid"})
	if known.body["status"] != unknown.body["status"] || len(known.body) != 1 || len(unknown.body) != 1 {
		t.Fatal("reset response exposes account existence")
	}
	reset := fixtureDelivery(t, ctx, admin, passwords, user, "reset")
	fixtureExec(t, ctx, admin, "UPDATE v3password.tokens SET expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1", identity.TokenHash(reset))
	run(401, "password/reset/confirm", "", "", map[string]string{"token": reset, "password": secondPassword})
	run(202, "password/reset/request", "", "", input)
	reset = fixtureDelivery(t, ctx, admin, passwords, user, "reset")
	run(200, "password/reset/confirm", "", "", map[string]string{"token": reset, "password": secondPassword})
	run(401, "password/reset/confirm", "", "", map[string]string{"token": reset, "password": firstPassword})
	if me, e := request("GET", "me", firstSession, "", origin, nil); e != nil || me.status != 401 {
		t.Fatal("reset did not revoke prior session")
	}
	run(401, "password/login", "", "", map[string]string{"email": learnerEmail, "password": firstPassword})
	r = logIn(secondPassword)
	fixtureExec(t, ctx, admin, "UPDATE v3identity.site_roles SET admin=(user_id=$1) WHERE user_id IN ($1,$2)", user, fixtureAdmin)
	run(409, "accounts/suspend", r.cookie, r.body["csrf_token"].(string), map[string]string{"user_id": user})
	fixtureExec(t, ctx, admin, "UPDATE v3identity.site_roles SET admin=(user_id=$2) WHERE user_id IN ($1,$2)", user, fixtureAdmin)
	run(403, "accounts/suspend", sessions[1], csrf[1], map[string]string{"user_id": user})
	run(200, "accounts/suspend", sessions[0], csrf[0], map[string]string{"user_id": user})
	if me, e := request("GET", "me", r.cookie, "", origin, nil); e != nil || me.status != 401 {
		t.Fatal("suspension did not revoke session")
	}
	fixtureExec(t, ctx, admin, `UPDATE v3password.limits SET attempts=7,window_start=clock_timestamp()+interval '10 seconds'
  WHERE scope='login' AND bucket<>0`)
	run(401, "password/login", "", "", map[string]string{"email": learnerEmail, "password": secondPassword})
	run(429, "password/login", "", "", map[string]string{"email": learnerEmail, "password": secondPassword})
	server.Close()
	if bytes.Contains(logs.Bytes(), []byte(firstPassword)) || bytes.Contains(logs.Bytes(), []byte(inviteToken)) ||
		bytes.Contains(logs.Bytes(), []byte(learnerEmail)) || bytes.Contains(logs.Bytes(), []byte(firstSession)) ||
		bytes.Contains(logs.Bytes(), []byte(hex.EncodeToString(key[:]))) {
		t.Fatal("sensitive authentication material in logs")
	}
}
