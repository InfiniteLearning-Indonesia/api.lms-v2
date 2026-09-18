//go:build qaintegration

package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/classes"
	"lmsv3/internal/egress"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type fileFixture struct {
	owner   *pgxpool.Pool
	runtime *pgxpool.Pool
	store   *Store
	tokens  map[string]string
	users   map[string]string
}

func openFileFixture(t *testing.T, ctx context.Context, checkpoint string, scanner Scanner, options ...Option) *fileFixture {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallFiles(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallFiles(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	grants := "GRANT SELECT,INSERT,UPDATE ON v3.file_assets TO qa_runtime; GRANT SELECT,INSERT ON v3.file_commands TO qa_runtime;"
	if checkpoint == "m06-cp02" {
		grants += " GRANT SELECT,INSERT ON v3.file_download_grants,v3.file_access_events TO qa_runtime;"
	}
	if _, err = owner.Exec(ctx, grants, pgx.QueryExecModeSimpleProtocol); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	objects, err := NewLocalStore(filepath.Join(os.Getenv("QA_LAB_DIR"), checkpoint+"-objects"))
	if err != nil {
		runtime.Close()
		owner.Close()
		t.Fatal(err)
	}
	store, err := New(runtime, objects, scanner, time.Minute, options...)
	if err != nil {
		t.Fatal(err)
	}
	f := &fileFixture{owner: owner, runtime: runtime, store: store, tokens: map[string]string{}, users: map[string]string{}}
	for _, subject := range []string{"admin", "teacher", "student", "student2", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		// Earlier identity checkpoint fixtures deliberately use short authority
		// windows. Refresh only already-active synthetic links so this checkpoint
		// tests file authorization instead of inheriting wall-clock expiry.
		tag, refreshErr := owner.Exec(ctx, `UPDATE v3identity.links
 SET valid_until=GREATEST(valid_until,clock_timestamp()+interval '5 minutes')
 WHERE user_id=$1 AND active`, user)
		if refreshErr != nil {
			t.Fatal(refreshErr)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("synthetic identity %s is not active", subject)
		}
		token, _ := randomHex(32)
		if _, err = owner.Exec(ctx, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until)
 VALUES($1,$2,$3,$4,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, identity.TokenHash(token), user, version, identity.TokenHash(token)); err != nil {
			t.Fatal(err)
		}
		f.users[subject], f.tokens[subject] = user, token
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return f
}

type remoteFixture struct{ calls int }

func (r *remoteFixture) Fetch(_ context.Context, rawURL string) (egress.Result, error) {
	r.calls++
	if rawURL == "https://files.example.com/material.txt" || rawURL == "https://files.example.com/other.txt" {
		return egress.Result{Data: []byte("remote material\n"), MediaType: "text/plain"}, nil
	}
	return egress.Result{}, egress.ErrPolicy
}

func TestSyntheticBoundedRemoteImport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	remote := &remoteFixture{}
	f := openFileFixture(t, ctx, "m06-cp02", FixtureScanner{}, WithRemoteFetcher(remote))
	classID := f.classroom(t, ctx)
	input := Upload{ClassID: classID, OwnerUserID: f.users["teacher"], Purpose: "MATERIAL", OriginalName: "remote.txt", DeclaredType: "text/plain"}
	if _, _, err := f.store.ImportRemote(ctx, f.tokens["outsider"], mustRandom(), mustRandom(), "https://files.example.com/material.txt", input); !errors.Is(err, ErrForbidden) || remote.calls != 0 {
		t.Fatal("foreign actor consumed outbound budget", err, remote.calls)
	}
	key := mustRandom()
	asset, replayed, err := f.store.ImportRemote(ctx, f.tokens["teacher"], key, mustRandom(), "https://files.example.com/material.txt", input)
	if err != nil || replayed || asset.State != "AVAILABLE" || remote.calls != 1 {
		t.Fatal("authorized remote import failed", asset, replayed, err)
	}
	replay, replayed, err := f.store.ImportRemote(ctx, f.tokens["teacher"], key, mustRandom(), "https://files.example.com/material.txt", input)
	if err != nil || !replayed || replay.ID != asset.ID || remote.calls != 2 {
		t.Fatal("remote import retry was not idempotent", replayed, err)
	}
	if _, _, err = f.store.ImportRemote(ctx, f.tokens["teacher"], key, mustRandom(), "https://files.example.com/other.txt", input); !errors.Is(err, ErrConflict) {
		t.Fatal("changed remote request reused idempotency key", err)
	}
	var events int
	if err = f.runtime.QueryRow(ctx, `SELECT count(*) FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id WHERE a.class_id=$1 AND a.action='file.upload.available'`, classID).Scan(&events); err != nil || events != 1 {
		t.Fatal("remote import audit/outbox mismatch", events, err)
	}
}

func (f *fileFixture) classroom(t *testing.T, ctx context.Context) string {
	t.Helper()
	classStore := classes.NewContextual(f.runtime)
	exec := func(subject string, command classes.Command) json.RawMessage {
		key, _ := randomHex(16)
		request, _ := randomHex(16)
		raw, err := classStore.Execute(ctx, f.tokens[subject], key, request, command)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	created := exec("admin", classes.Command{Action: "create", Name: "M06 files"})
	var class classes.Class
	if err := json.Unmarshal(created, &class); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ subject, role string }{{"teacher", "teacher"}, {"student", "student"}, {"student2", "student"}} {
		exec("admin", classes.Command{Action: "add", ClassID: class.ID, UserID: f.users[row.subject], Role: row.role})
	}
	_, err := classStore.Execute(ctx, f.tokens["admin"], mustRandom(), mustRandom(), classes.Command{Action: "publish", ClassID: class.ID, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	return class.ID
}

func mustRandom() string { value, _ := randomHex(16); return value }

func TestSyntheticPrivateFileOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := openFileFixture(t, ctx, "m06-cp01", FixtureScanner{})
	classID := f.classroom(t, ctx)
	asset, replayed, err := f.store.Upload(ctx, f.tokens["teacher"], mustRandom(), mustRandom(), Upload{ClassID: classID, OwnerUserID: f.users["teacher"], Purpose: "MATERIAL", OriginalName: "lesson.txt", DeclaredType: "text/plain", Data: []byte("lesson material\n")})
	if err != nil || replayed || asset.State != "AVAILABLE" || asset.objectKey == "" {
		t.Fatal("owned asset failed", asset, err)
	}
	metadata, err := f.store.Metadata(ctx, f.tokens["student"], asset.ID)
	if err != nil || metadata.ID != asset.ID || metadata.objectKey == "" {
		t.Fatal("published Class material unavailable", err)
	}
	if _, err = f.store.Metadata(ctx, f.tokens["outsider"], asset.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign-context metadata exposed", err)
	}
	if _, _, err = f.store.Upload(ctx, f.tokens["student"], mustRandom(), mustRandom(), Upload{ClassID: classID, OwnerUserID: f.users["student2"], Purpose: "EVIDENCE", OriginalName: "foreign.txt", DeclaredType: "text/plain", Data: []byte("foreign")}); !errors.Is(err, ErrForbidden) {
		t.Fatal("student assigned foreign owner", err)
	}
	encoded, _ := json.Marshal(asset)
	if bytes.Contains(encoded, []byte("object_key")) || bytes.Contains(encoded, []byte(asset.objectKey)) {
		t.Fatal("private object key exposed")
	}
	badID, _ := randomHex(16)
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at)
	 VALUES($1,$2,$3,'MATERIAL',$4,'bad.txt','text/plain','text/plain',1,$5,'AVAILABLE','CLEAN','CLEAN',$3,clock_timestamp())`, badID, mustRandom(), f.users["teacher"], strings.Repeat("a", 64), strings.Repeat("b", 64))
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "23503" {
		t.Fatal("Class FK ownership fence missing", err)
	}
	badID, _ = randomHex(16)
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at)
	 VALUES($1,$2,$3,'MATERIAL',$4,'bad.txt','text/plain','text/plain',1,$4,'AVAILABLE','CLEAN','CLEAN',$3,clock_timestamp())`, badID, classID, f.users["teacher"], strings.Repeat("c", 64))
	if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
		t.Fatal("object locator/checksum separation constraint missing", err)
	}
	badID, _ = randomHex(16)
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at)
	 VALUES($1,$2,$3,'MATERIAL',$4,'duplicate.txt','text/plain','text/plain',1,$5,'AVAILABLE','CLEAN','CLEAN',$3,clock_timestamp())`, badID, classID, f.users["teacher"], asset.objectKey, strings.Repeat("d", 64))
	if !errors.As(err, &pgerr) || pgerr.Code != "23505" {
		t.Fatal("object locator uniqueness constraint missing", err)
	}
	var unsafe bool
	if err = f.runtime.QueryRow(ctx, "SELECT has_table_privilege(current_user,'v3.file_assets','DELETE') OR has_table_privilege(current_user,'v3.file_assets','TRUNCATE')").Scan(&unsafe); err != nil || unsafe {
		t.Fatal("runtime has destructive file privileges", err)
	}
}

type switchScanner struct{ unavailable bool }

func (s *switchScanner) Scan(context.Context, []byte) error {
	if s.unavailable {
		return ErrScanUnavailable
	}
	return nil
}

func TestSyntheticFileDeliveryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	scanner := &switchScanner{unavailable: true}
	f := openFileFixture(t, ctx, "m06-cp02", scanner)
	classID := f.classroom(t, ctx)
	key := mustRandom()
	input := Upload{ClassID: classID, OwnerUserID: f.users["student"], Purpose: "EVIDENCE", OriginalName: "proof.txt", DeclaredType: "text/plain", Data: []byte("proof evidence\n")}
	asset, replayed, err := f.store.Upload(ctx, f.tokens["student"], key, mustRandom(), input)
	if err != nil || replayed || asset.State != "QUARANTINED" {
		t.Fatal("scanner failure did not quarantine", asset, err)
	}
	if _, err = f.store.Rescan(ctx, f.tokens["student"], asset.ID, mustRandom()); !errors.Is(err, ErrForbidden) {
		t.Fatal("student invoked scanner", err)
	}
	scanner.unavailable = false
	asset, err = f.store.Rescan(ctx, f.tokens["teacher"], asset.ID, mustRandom())
	if err != nil || asset.State != "AVAILABLE" {
		t.Fatal("authorized clean rescan failed", asset, err)
	}
	retried, replayed, err := f.store.Upload(ctx, f.tokens["student"], key, mustRandom(), input)
	if err != nil || !replayed || retried.ID != asset.ID {
		t.Fatal("upload idempotency failed", replayed, err)
	}
	grant, err := f.store.Grant(ctx, f.tokens["student"], asset.ID, mustRandom())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.Download(ctx, f.tokens["student2"], grant.Token, mustRandom()); !errors.Is(err, ErrForbidden) {
		t.Fatal("grant was not target-bound", err)
	}
	downloaded, data, err := f.store.Download(ctx, f.tokens["student"], grant.Token, mustRandom())
	if err != nil || downloaded.ID != asset.ID || string(data) != string(input.Data) {
		t.Fatal("authorized download failed", err)
	}
	f.store.now = func() time.Time { return grant.ExpiresAt.Add(time.Nanosecond) }
	if _, _, err = f.store.Download(ctx, f.tokens["student"], grant.Token, mustRandom()); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired grant accepted", err)
	}
	var access, auditWithoutOutbox int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.file_access_events WHERE asset_id=$1", asset.ID).Scan(&access); err != nil || access != 1 {
		t.Fatal("delivery audit missing", access, err)
	}
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'file.%' AND o.event_id IS NULL").Scan(&auditWithoutOutbox); err != nil || auditWithoutOutbox != 0 {
		t.Fatal("file audit/outbox not atomic", auditWithoutOutbox, err)
	}
}
