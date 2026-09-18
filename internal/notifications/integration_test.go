//go:build qaintegration

package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"lmsv3/internal/outbox"
)

func ownerAndWorker(t *testing.T, ctx context.Context) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallDurableDelivery(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	password := os.Getenv("QA_WORKER_PASSWORD")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(password) {
		owner.Close()
		t.Fatal("worker password required")
	}
	var exists bool
	if err = owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='qa_worker')").Scan(&exists); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if !exists {
		if _, err = owner.Exec(ctx, "CREATE ROLE qa_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '"+password+"'", pgx.QueryExecModeSimpleProtocol); err != nil {
			owner.Close()
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, `GRANT USAGE ON SCHEMA qa_cp05,v3,v3ops TO qa_worker;
GRANT SELECT ON qa_cp05.marker,v3.audit,v3.outbox,v3ops.migrations,v3ops.outbox_failures TO qa_worker;
GRANT UPDATE(status,lease_id,lease_until,attempts,next_attempt,sent_at,last_error_code) ON v3.outbox TO qa_worker;
GRANT INSERT ON v3ops.outbox_attempts,v3ops.outbox_failures TO qa_worker`, pgx.QueryExecModeSimpleProtocol); err != nil {
			owner.Close()
			t.Fatal(err)
		}
	}
	if err = labdb.InstallDurableDelivery(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	_, err = owner.Exec(ctx, `GRANT SELECT ON v3ops.outbox_provider_receipts,v3ops.outbox_failures TO qa_runtime;
GRANT SELECT ON v3ops.outbox_provider_receipts TO qa_worker;
GRANT INSERT ON v3ops.outbox_provider_receipts TO qa_worker`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	worker, err := labdb.WorkerRuntime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { worker.Close(); owner.Close() })
	return owner, worker
}

func seedOutbox(t *testing.T, ctx context.Context, owner *pgxpool.Pool, topic, suffix string) string {
	t.Helper()
	var actor string
	if err := owner.QueryRow(ctx, "SELECT id FROM v3.users ORDER BY id LIMIT 1").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(topic + suffix + time.Now().String()))
	event := hex.EncodeToString(h[:16])
	if _, err := owner.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,action,request_id) VALUES($1,$2,'qa.notification',$1)", event, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, "INSERT INTO v3.outbox(event_id,topic) VALUES($1,$2)", event, topic); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestSyntheticDurableDeliveryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, workerPool := ownerAndWorker(t, ctx)
	event := seedOutbox(t, ctx, owner, "qa.m13.receipt.v1", "normal")
	queue := outbox.Store{Pool: workerPool, Topic: "qa.m13.receipt.v1", BackoffBase: time.Millisecond}
	sink := NewRehearsalSink()
	w := outbox.Worker{Queue: queue, Sink: sink, Config: outbox.Config{BatchSize: 1, Parallelism: 1, PollInterval: time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 200 * time.Millisecond}}
	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var status, outcome, reference string
	if err := owner.QueryRow(ctx, `SELECT o.status,r.outcome,r.reference_hash FROM v3.outbox o JOIN v3ops.outbox_provider_receipts r ON r.event_id=o.event_id WHERE o.event_id=$1`, event).Scan(&status, &outcome, &reference); err != nil || status != "SENT" || outcome != "CONFIRMED" || len(reference) != 64 {
		t.Fatal("receipt/ack missing", status, outcome, err)
	}

	crash := seedOutbox(t, ctx, owner, "qa.m13.crash.v1", "crash")
	crashQueue := outbox.Store{Pool: workerPool, Topic: "qa.m13.crash.v1"}
	claimed, err := crashQueue.ClaimBatch(ctx, 1, 120*time.Millisecond)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err)
	}
	if err = sink.Deliver(ctx, claimed[0]); err != nil {
		t.Fatal(err)
	} // provider success; process dies before receipt/ACK
	time.Sleep(150 * time.Millisecond)
	crashWorker := outbox.Worker{Queue: crashQueue, Sink: sink, Config: outbox.Config{BatchSize: 1, Parallelism: 1, PollInterval: time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 200 * time.Millisecond}}
	if n, runErr := crashWorker.RunOnce(ctx); runErr != nil || n != 1 {
		t.Fatal(n, runErr)
	}
	var attempt int
	if err = owner.QueryRow(ctx, "SELECT attempt_number,outcome FROM v3ops.outbox_provider_receipts WHERE event_id=$1", crash).Scan(&attempt, &outcome); err != nil || attempt != 2 || outcome != "DUPLICATE" {
		t.Fatal("duplicate recovery absent", attempt, outcome, err)
	}
}

func TestSyntheticDurableDeliveryBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, workerPool := ownerAndWorker(t, ctx)
	event := seedOutbox(t, ctx, owner, "qa.m13.fence.v1", "stale")
	queue := outbox.Store{Pool: workerPool, Topic: "qa.m13.fence.v1"}
	items, err := queue.ClaimBatch(ctx, 1, 110*time.Millisecond)
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	time.Sleep(140 * time.Millisecond)
	h := sha256.Sum256([]byte(event))
	receipt := outbox.ProviderReceipt{ProviderKey: "rehearsal.email", Outcome: "CONFIRMED", ReferenceHash: hex.EncodeToString(h[:])}
	if !errors.Is(queue.RecordReceipt(ctx, items[0].Lease, receipt), outbox.ErrLeaseLost) {
		t.Fatal("expired lease wrote receipt")
	}
	if !errors.Is(queue.RecordReceipt(ctx, outbox.Lease{}, receipt), outbox.ErrInvalidInput) {
		t.Fatal("invalid receipt accepted")
	}
	var unsafe bool
	if err = owner.QueryRow(ctx, "SELECT has_table_privilege('qa_worker','v3ops.outbox_provider_receipts','UPDATE,DELETE,TRUNCATE')").Scan(&unsafe); err != nil || unsafe {
		t.Fatal("unsafe worker privilege", err)
	}
}

func notificationFixture(t *testing.T, ctx context.Context) (*pgxpool.Pool, *pgxpool.Pool, *Store, string, string) {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallNotificationSafety(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallNotificationSafety(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	_, err = owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.notification_preference_events,v3.notification_commands,v3.notification_deliveries TO qa_runtime;
GRANT SELECT ON v3ops.provider_binding_events,v3ops.outbox_provider_receipts,v3ops.outbox_failures TO qa_runtime`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	var user string
	var version int64
	if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject='student'").Scan(&user, &version); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, "UPDATE v3identity.links SET valid_until=clock_timestamp()+interval '10 minutes' WHERE user_id=$1", user); err != nil {
		t.Fatal(err)
	}
	token, _ := identity.RandomToken()
	if _, err = owner.Exec(ctx, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until) VALUES($1,$2,$3,$1,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, identity.TokenHash(token), user, version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return owner, runtime, New(runtime), token, user
}

func TestSyntheticNotificationLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, _, store, token, user := notificationFixture(t, ctx)
	pref, err := store.Preference(ctx, token)
	if err != nil || !pref.Enabled || pref.Version != 0 {
		t.Fatal(pref, err)
	}
	raw, replay, err := store.SetPreference(ctx, token, "notify_pref_001", 0, false)
	if err != nil || replay || len(raw) == 0 {
		t.Fatal(err)
	}
	_, replay, err = store.SetPreference(ctx, token, "notify_pref_001", 0, false)
	if err != nil || !replay {
		t.Fatal("idempotency missing", err)
	}
	pref, err = store.Preference(ctx, token)
	if err != nil || pref.Enabled || pref.Version != 1 {
		t.Fatal(pref, err)
	}
	event := seedOutbox(t, ctx, owner, "qa.m13.notification.v1", "preference")
	planned, err := store.Plan(ctx, event, user, "class.updated", "OPERATIONAL")
	if err != nil || planned.Status != "SUPPRESSED" || planned.Suppression != "PREFERENCE_DISABLED" {
		t.Fatal(planned, err)
	}
	var auditCount int
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='notification.preference'").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("preference transaction split", auditCount, err)
	}
}

func TestSyntheticNotificationBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, runtime, store, _, user := notificationFixture(t, ctx)
	var admin string
	if err := owner.QueryRow(ctx, "SELECT user_id FROM v3identity.links WHERE subject='admin'").Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO v3ops.provider_binding_events(channel,version,adapter,secret_ref,enabled,actor_id) VALUES('EMAIL',1,'REHEARSAL','smtp-password-plaintext',true,$1)`, admin); err == nil {
		t.Fatal("raw secret accepted")
	}
	if _, err := owner.Exec(ctx, `INSERT INTO v3ops.provider_binding_events(channel,version,adapter,secret_ref,enabled,actor_id) VALUES('EMAIL',1,'REHEARSAL','secretref://qa/rehearsal/email',true,$1)`, admin); err != nil {
		t.Fatal(err)
	}
	event := seedOutbox(t, ctx, owner, "qa.m13.import.v1", "import")
	planned, err := store.Plan(ctx, event, user, "class.imported", "HISTORICAL_IMPORT")
	if err != nil || planned.Status != "SUPPRESSED" || planned.Suppression != "HISTORICAL_IMPORT" {
		t.Fatal(planned, err)
	}
	_, err = runtime.Exec(ctx, "DELETE FROM v3.notification_deliveries WHERE id=$1", planned.ID)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("delete privilege leaked", err)
	}
	_, err = runtime.Exec(ctx, "UPDATE v3.notification_deliveries SET status='QUEUED' WHERE id=$1", planned.ID)
	if !errors.As(err, &denied) {
		t.Fatal("immutable delivery changed", err)
	}
	if _, err = owner.Exec(ctx, fmt.Sprintf("INSERT INTO v3.notification_deliveries(id,event_id,user_id,channel,template_key,source,status) VALUES('%s',$1,$2,'EMAIL','bypass','HISTORICAL_IMPORT','QUEUED')", "ffffffffffffffffffffffffffffffff"), event, user); err == nil {
		t.Fatal("import outbound bypass accepted")
	}
}
