package outbox

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Queue interface {
	ClaimBatch(context.Context, int, time.Duration) ([]Event, error)
	Ack(context.Context, Lease) error
	RetryWithCode(context.Context, Lease, string) error
}

// Sink must use EventID as its provider idempotency key. The worker guarantees
// at-least-once invocation, not exactly-once network side effects.
type Sink interface {
	Deliver(context.Context, Event) error
}

// ProviderReceipt is deliberately reference-only. ReferenceHash is a SHA-256
// digest of the provider response identifier; raw provider identifiers, payloads
// and credentials are never persisted by this boundary.
type ProviderReceipt struct {
	ProviderKey   string
	Outcome       string
	ReferenceHash string
}

type ReceiptSink interface {
	Sink
	DeliverWithReceipt(context.Context, Event) (ProviderReceipt, error)
}

type ReceiptQueue interface {
	Queue
	RecordReceipt(context.Context, Lease, ProviderReceipt) error
}

type Config struct {
	BatchSize       int
	Parallelism     int
	PollInterval    time.Duration
	LeaseDuration   time.Duration
	DeliveryTimeout time.Duration
}

func (c Config) validate() error {
	if c.BatchSize < 1 || c.BatchSize > 100 || c.Parallelism < 1 || c.Parallelism > 16 ||
		c.PollInterval < time.Millisecond || c.PollInterval > time.Minute ||
		c.LeaseDuration < 100*time.Millisecond || c.LeaseDuration > 10*time.Minute ||
		c.DeliveryTimeout <= 0 || c.DeliveryTimeout >= c.LeaseDuration {
		return ErrInvalidInput
	}
	return nil
}

type Worker struct {
	Queue  Queue
	Sink   Sink
	Config Config
}

func (w Worker) Validate() error {
	if w.Queue == nil || w.Sink == nil {
		return ErrInvalidInput
	}
	_, sinkReceipts := w.Sink.(ReceiptSink)
	_, queueReceipts := w.Queue.(ReceiptQueue)
	if sinkReceipts && !queueReceipts {
		return ErrInvalidInput
	}
	return w.Config.validate()
}

// RunOnce claims in a short DB statement, then performs provider work outside
// all DB transactions with bounded concurrency and per-delivery timeouts.
func (w Worker) RunOnce(ctx context.Context) (int, error) {
	if err := w.Validate(); err != nil {
		return 0, err
	}
	events, err := w.Queue.ClaimBatch(ctx, w.Config.BatchSize, w.Config.LeaseDuration)
	if err != nil || len(events) == 0 {
		return len(events), err
	}
	sem := make(chan struct{}, w.Config.Parallelism)
	errs := make(chan error, len(events))
	var group sync.WaitGroup
	started := 0
	for _, event := range events {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			group.Wait()
			close(errs)
			return started, nil
		}
		started++
		group.Add(1)
		go func(item Event) {
			defer group.Done()
			defer func() { <-sem }()
			deliveryCtx, cancel := context.WithTimeout(ctx, w.Config.DeliveryTimeout)
			var deliveryErr error
			if sink, ok := w.Sink.(ReceiptSink); ok {
				var receipt ProviderReceipt
				receipt, deliveryErr = sink.DeliverWithReceipt(deliveryCtx, item)
				if deliveryErr == nil {
					deliveryErr = w.Queue.(ReceiptQueue).RecordReceipt(deliveryCtx, item.Lease, receipt)
				}
			} else {
				deliveryErr = w.Sink.Deliver(deliveryCtx, item)
			}
			cancel()
			if deliveryErr == nil {
				errs <- w.Queue.Ack(ctx, item.Lease)
				return
			}
			// Cancellation leaves the lease intact for expiry-based crash recovery.
			if ctx.Err() != nil {
				errs <- nil
				return
			}
			errs <- w.Queue.RetryWithCode(ctx, item.Lease, "PROVIDER_RETRY")
		}(event)
	}
	group.Wait()
	close(errs)
	for processErr := range errs {
		if processErr != nil && !errors.Is(processErr, ErrLeaseLost) {
			return started, processErr
		}
	}
	return started, nil
}

func (w Worker) Run(ctx context.Context) error {
	if err := w.Validate(); err != nil {
		return err
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			processed, err := w.RunOnce(ctx)
			if err != nil {
				return err
			}
			wait := w.Config.PollInterval
			if processed == w.Config.BatchSize {
				wait = time.Millisecond
			}
			timer.Reset(wait)
		}
	}
}
