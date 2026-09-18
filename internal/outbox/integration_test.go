//go:build qaintegration

package outbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lmsv3/internal/labdb"
)

type idempotentSink struct {
	mu      sync.Mutex
	fail    map[string]bool
	calls   map[string]int
	effects map[string]int
	started chan struct{}
	block   bool
}

func (s *idempotentSink) Deliver(ctx context.Context, event Event) error {
	s.mu.Lock()
	s.calls[event.EventID]++
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.fail[event.EventID] {
		delete(s.fail, event.EventID)
		s.mu.Unlock()
		return errors.New("synthetic provider unavailable")
	}
	blocked := s.block
	s.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return ctx.Err()
	}
	s.mu.Lock()
	if s.effects[event.EventID] == 0 {
		s.effects[event.EventID] = 1
	}
	s.mu.Unlock()
	return nil
}

func TestSyntheticWorkerLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	password := os.Getenv("QA_WORKER_PASSWORD")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(password) {
		t.Fatal("worker password required")
	}
	var workerExists bool
	if err = owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='qa_worker')").Scan(&workerExists); err != nil {
		t.Fatal(err)
	}
	if !workerExists {
		if _, err = owner.Exec(ctx, "CREATE ROLE qa_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '"+password+"'", pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	grants := `GRANT USAGE ON SCHEMA qa_cp05,v3,v3ops TO qa_worker;
GRANT SELECT ON qa_cp05.marker,v3.audit,v3.outbox,v3ops.migrations,v3ops.outbox_failures TO qa_worker;
GRANT UPDATE(status,lease_id,lease_until,attempts,next_attempt,sent_at,last_error_code) ON v3.outbox TO qa_worker;
GRANT INSERT ON v3ops.outbox_attempts,v3ops.outbox_failures TO qa_worker;`
	if _, err = owner.Exec(ctx, grants, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	pool, err := labdb.WorkerRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// The dedicated worker cannot mutate domain/audit rows or delete history.
	_, deniedErr := pool.Exec(ctx, "INSERT INTO v3.classes(id,name) VALUES('worker-denied','denied')")
	var denied *pgconn.PgError
	if !errors.As(deniedErr, &denied) || denied.Code != "42501" {
		t.Fatal("worker domain denial absent")
	}
	var actor string
	if err = owner.QueryRow(ctx, "SELECT id FROM v3.users ORDER BY id LIMIT 1").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	seed := func(topic string, count int) []string {
		ids := make([]string, count)
		for i := range ids {
			ids[i] = fmt.Sprintf("m03-cp05-%s-%02d", topic, i)
			if _, seedErr := owner.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,action,request_id) VALUES($1,$2,'qa.worker','m03-cp05')", ids[i], actor); seedErr != nil {
				t.Fatal(seedErr)
			}
			if _, seedErr := owner.Exec(ctx, "INSERT INTO v3.outbox(event_id,topic) VALUES($1,$2)", ids[i], topic); seedErr != nil {
				t.Fatal(seedErr)
			}
		}
		return ids
	}

	concurrentIDs := seed("qa.concurrent.v1", 4)
	queue := Store{Pool: pool, Topic: "qa.concurrent.v1", BackoffBase: 5 * time.Millisecond}
	claimed := make(chan []Event, 2)
	errorsSeen := make(chan error, 2)
	for range 2 {
		go func() {
			items, claimErr := queue.ClaimBatch(ctx, 2, time.Second)
			claimed <- items
			errorsSeen <- claimErr
		}()
	}
	seen := map[string]bool{}
	var first Lease
	for range 2 {
		if err = <-errorsSeen; err != nil {
			t.Fatal(err)
		}
		for _, item := range <-claimed {
			if seen[item.EventID] {
				t.Fatal("duplicate concurrent claim")
			}
			seen[item.EventID] = true
			if first.EventID == "" {
				first = item.Lease
			}
			if err = queue.Ack(ctx, item.Lease); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(seen) != len(concurrentIDs) {
		t.Fatal("concurrent claims incomplete")
	}
	wrong := first
	wrong.Token = "00000000000000000000000000000000"
	if !errors.Is(queue.Ack(ctx, wrong), ErrLeaseLost) {
		t.Fatal("stale/wrong fence accepted")
	}

	retryID := seed("qa.retry.v1", 1)[0]
	retryQueue := Store{Pool: pool, Topic: "qa.retry.v1", BackoffBase: 5 * time.Millisecond}
	retrySink := &idempotentSink{fail: map[string]bool{retryID: true}, calls: map[string]int{}, effects: map[string]int{}}
	worker := Worker{Queue: retryQueue, Sink: retrySink, Config: Config{BatchSize: 1, Parallelism: 1, PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 100 * time.Millisecond}}
	if n, runErr := worker.RunOnce(ctx); runErr != nil || n != 1 {
		t.Fatal("retry scheduling failed", n, runErr)
	}
	time.Sleep(20 * time.Millisecond)
	if n, runErr := worker.RunOnce(ctx); runErr != nil || n != 1 {
		t.Fatal("retry delivery failed", n, runErr)
	}
	if retrySink.calls[retryID] != 2 || retrySink.effects[retryID] != 1 {
		t.Fatal("retry did not preserve provider idempotency")
	}
	failedID := seed("qa.failed.v1", 1)[0]
	failedQueue := Store{Pool: pool, Topic: "qa.failed.v1", MaxAttempts: 1}
	failedSink := &idempotentSink{fail: map[string]bool{failedID: true}, calls: map[string]int{}, effects: map[string]int{}}
	failedWorker := Worker{Queue: failedQueue, Sink: failedSink, Config: Config{BatchSize: 1, Parallelism: 1, PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 100 * time.Millisecond}}
	if n, runErr := failedWorker.RunOnce(ctx); runErr != nil || n != 1 {
		t.Fatal("terminal failure was not recorded", n, runErr)
	}
	if items, claimErr := failedQueue.ClaimBatch(ctx, 1, time.Second); claimErr != nil || len(items) != 0 {
		t.Fatal("terminal event was reclaimed", claimErr)
	}

	crashID := seed("qa.crash.v1", 1)[0]
	crashQueue := Store{Pool: pool, Topic: "qa.crash.v1"}
	crashSink := &idempotentSink{fail: map[string]bool{}, calls: map[string]int{}, effects: map[string]int{}}
	crashed, err := crashQueue.ClaimBatch(ctx, 1, 150*time.Millisecond)
	if err != nil || len(crashed) != 1 || crashSink.Deliver(ctx, crashed[0]) != nil {
		t.Fatal("crash fixture failed", err)
	}
	// Simulate provider success followed by process loss before Ack.
	time.Sleep(180 * time.Millisecond)
	crashWorker := Worker{Queue: crashQueue, Sink: crashSink, Config: Config{BatchSize: 1, Parallelism: 1, PollInterval: 5 * time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 100 * time.Millisecond}}
	if n, runErr := crashWorker.RunOnce(ctx); runErr != nil || n != 1 || crashSink.calls[crashID] != 2 || crashSink.effects[crashID] != 1 {
		t.Fatal("crash recovery/idempotency failed", n, runErr)
	}
	if !errors.Is(crashQueue.Ack(ctx, crashed[0].Lease), ErrLeaseLost) {
		t.Fatal("expired lease regained authority")
	}

	shutdownID := seed("qa.shutdown.v1", 1)[0]
	shutdownQueue := Store{Pool: pool, Topic: "qa.shutdown.v1"}
	shutdownSink := &idempotentSink{fail: map[string]bool{}, calls: map[string]int{}, effects: map[string]int{}, started: make(chan struct{}, 1), block: true}
	shutdownWorker := Worker{Queue: shutdownQueue, Sink: shutdownSink, Config: Config{BatchSize: 1, Parallelism: 1, PollInterval: 5 * time.Millisecond, LeaseDuration: 250 * time.Millisecond, DeliveryTimeout: 200 * time.Millisecond}}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- shutdownWorker.Run(workerCtx) }()
	select {
	case <-shutdownSink.started:
	case <-time.After(time.Second):
		t.Fatal("shutdown fixture did not start")
	}
	stop()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var status string
	if err = owner.QueryRow(ctx, "SELECT status FROM v3.outbox WHERE event_id=$1", shutdownID).Scan(&status); err != nil || status != "LEASED" {
		t.Fatal("cancelled delivery was acknowledged")
	}
	time.Sleep(280 * time.Millisecond)
	recovered, err := shutdownQueue.ClaimBatch(ctx, 1, time.Second)
	if err != nil || len(recovered) != 1 || recovered[0].EventID != shutdownID {
		t.Fatal("shutdown lease was not recoverable", err)
	}
	if err = shutdownQueue.Ack(ctx, recovered[0].Lease); err != nil {
		t.Fatal(err)
	}

	// Pool pressure is bounded and acquisition obeys caller cancellation.
	one, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	two, err := pool.Acquire(ctx)
	if err != nil {
		one.Release()
		t.Fatal(err)
	}
	pressureCtx, pressureCancel := context.WithTimeout(ctx, 40*time.Millisecond)
	_, pressureErr := pool.Acquire(pressureCtx)
	pressureCancel()
	two.Release()
	one.Release()
	if !errors.Is(pressureErr, context.DeadlineExceeded) {
		t.Fatal("pool pressure did not fail bounded", pressureErr)
	}

	var missing, retryRows, retained int
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='qa.worker' AND o.event_id IS NULL").Scan(&missing); err != nil || missing != 0 {
		t.Fatal("worker outbox orphan", missing, err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3ops.outbox_attempts WHERE outcome='RETRY_SCHEDULED' AND error_code='PROVIDER_RETRY'").Scan(&retryRows); err != nil || retryRows < 1 {
		t.Fatal("retry history missing", retryRows, err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3.outbox WHERE event_id LIKE 'm03-cp05-%'").Scan(&retained); err != nil || retained != 8 {
		t.Fatal("worker rows were not retained", retained, err)
	}
	if err = owner.QueryRow(ctx, "SELECT count(*) FROM v3ops.outbox_failures WHERE event_id=$1 AND final_attempt=1", failedID).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("terminal failure ledger missing", retained, err)
	}
}

// TestSyntheticWorkerConcurrentReplay runs after the lifecycle fixture has
// provisioned qa_worker. CP06 executes this test under the Linux race detector.
func TestSyntheticWorkerConcurrentReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	pool, err := labdb.WorkerRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var actor string
	if err = owner.QueryRow(ctx, "SELECT id FROM v3.users ORDER BY id LIMIT 1").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		id := fmt.Sprintf("m03-cp06-race-%02d", i)
		if _, err = owner.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,action,request_id) VALUES($1,$2,'qa.worker.race','m03-cp06')", id, actor); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, "INSERT INTO v3.outbox(event_id,topic) VALUES($1,'qa.race.v1')", id); err != nil {
			t.Fatal(err)
		}
	}
	queue := Store{Pool: pool, Topic: "qa.race.v1"}
	claimed := make(chan Event, 8)
	failures := make(chan error, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			items, claimErr := queue.ClaimBatch(ctx, 1, time.Second)
			if claimErr != nil || len(items) != 1 {
				if claimErr == nil {
					claimErr = errors.New("missing race claim")
				}
				failures <- claimErr
				return
			}
			if claimErr = queue.Ack(ctx, items[0].Lease); claimErr != nil {
				failures <- claimErr
				return
			}
			claimed <- items[0]
		}()
	}
	group.Wait()
	close(failures)
	close(claimed)
	for failure := range failures {
		t.Fatal(failure)
	}
	seen := map[string]bool{}
	for item := range claimed {
		if seen[item.EventID] {
			t.Fatal("duplicate event under concurrent replay")
		}
		seen[item.EventID] = true
	}
	if len(seen) != 8 {
		t.Fatal("race claims incomplete", len(seen))
	}
}
