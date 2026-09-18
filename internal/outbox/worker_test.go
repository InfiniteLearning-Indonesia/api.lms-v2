package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeQueue struct {
	mu      sync.Mutex
	events  []Event
	acked   []Lease
	retried []Lease
}

func (q *fakeQueue) ClaimBatch(_ context.Context, limit int, _ time.Duration) ([]Event, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.events) < limit {
		limit = len(q.events)
	}
	items := append([]Event(nil), q.events[:limit]...)
	q.events = q.events[limit:]
	return items, nil
}
func (q *fakeQueue) Ack(_ context.Context, lease Lease) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.acked = append(q.acked, lease)
	return nil
}
func (q *fakeQueue) RetryWithCode(_ context.Context, lease Lease, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.retried = append(q.retried, lease)
	return nil
}

type fakeSink struct {
	mu       sync.Mutex
	fail     map[string]bool
	seen     map[string]int
	active   int
	max      int
	blocking chan struct{}
	started  chan struct{}
}

func (s *fakeSink) Deliver(ctx context.Context, event Event) error {
	s.mu.Lock()
	s.active++
	if s.active > s.max {
		s.max = s.active
	}
	s.seen[event.EventID]++
	if s.started != nil {
		s.started <- struct{}{}
	}
	fail := s.fail[event.EventID]
	delete(s.fail, event.EventID)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	if s.blocking != nil {
		select {
		case <-s.blocking:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if fail {
		return errors.New("synthetic provider failure")
	}
	return nil
}

func testConfig() Config {
	return Config{BatchSize: 2, Parallelism: 2, PollInterval: time.Millisecond, LeaseDuration: time.Second, DeliveryTimeout: 100 * time.Millisecond}
}

func TestWorkerConfiguration(t *testing.T) {
	valid := Worker{Queue: &fakeQueue{}, Sink: &fakeSink{}, Config: testConfig()}
	if valid.Validate() != nil {
		t.Fatal("valid worker rejected")
	}
	for _, invalid := range []Worker{
		{},
		{Queue: &fakeQueue{}, Sink: &fakeSink{}, Config: Config{}},
		{Queue: &fakeQueue{}, Sink: &fakeSink{}, Config: Config{BatchSize: 101, Parallelism: 1, PollInterval: time.Second, LeaseDuration: time.Second, DeliveryTimeout: time.Second}},
	} {
		if invalid.Validate() == nil {
			t.Fatal("unsafe worker config accepted")
		}
	}
}

func TestStoreInputBoundary(t *testing.T) {
	for _, topic := range []string{"audit.v1", "qa-worker_2"} {
		if got, err := (Store{Topic: topic}).topic(); err != nil || got != topic {
			t.Fatal("valid topic rejected")
		}
	}
	if _, err := (Store{Topic: "../unsafe"}).topic(); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("unsafe topic accepted")
	}
	store := Store{}
	if _, err := store.Claim(context.Background()); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nil claim accepted")
	}
	if err := store.Ack(context.Background(), Lease{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("empty ack accepted")
	}
	if err := store.Retry(context.Background(), Lease{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("empty retry accepted")
	}
}

func TestWorkerRetryConcurrencyAndShutdown(t *testing.T) {
	queue := &fakeQueue{events: []Event{
		{Lease: Lease{EventID: "event-1", Token: "lease-1", Attempt: 1}},
		{Lease: Lease{EventID: "event-2", Token: "lease-2", Attempt: 1}},
	}}
	sink := &fakeSink{fail: map[string]bool{"event-2": true}, seen: map[string]int{}, blocking: make(chan struct{}), started: make(chan struct{}, 2)}
	worker := Worker{Queue: queue, Sink: sink, Config: testConfig()}
	result := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := worker.RunOnce(context.Background())
		result <- struct {
			n   int
			err error
		}{n, err}
	}()
	<-sink.started
	<-sink.started
	sink.mu.Lock()
	maximum := sink.max
	sink.mu.Unlock()
	close(sink.blocking)
	outcome := <-result
	processed, err := outcome.n, outcome.err
	if err != nil || processed != 2 || len(queue.acked) != 1 || len(queue.retried) != 1 || maximum != 2 {
		t.Fatal("bounded ack/retry behavior failed", processed, err)
	}

	blocked := make(chan struct{})
	queue.events = []Event{{Lease: Lease{EventID: "event-3", Token: "lease-3", Attempt: 1}}}
	sink.blocking = blocked
	sink.started = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	deadline := time.After(time.Second)
	for {
		sink.mu.Lock()
		started := sink.seen["event-3"] == 1
		sink.mu.Unlock()
		if started {
			break
		}
		select {
		case <-deadline:
			t.Fatal("delivery did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if len(queue.acked) != 1 || len(queue.retried) != 1 {
		t.Fatal("cancelled delivery changed durable state")
	}
}
