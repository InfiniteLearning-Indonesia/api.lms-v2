package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"lmsv3/internal/outbox"
)

// RehearsalSink is the only sink used by automated QA. It makes no network
// calls and models provider idempotency using the immutable event ID.
type RehearsalSink struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewRehearsalSink() *RehearsalSink { return &RehearsalSink{seen: map[string]struct{}{}} }

func (s *RehearsalSink) Deliver(ctx context.Context, event outbox.Event) error {
	_, err := s.DeliverWithReceipt(ctx, event)
	return err
}

func (s *RehearsalSink) DeliverWithReceipt(ctx context.Context, event outbox.Event) (outbox.ProviderReceipt, error) {
	if err := ctx.Err(); err != nil {
		return outbox.ProviderReceipt{}, err
	}
	s.mu.Lock()
	_, duplicate := s.seen[event.EventID]
	s.seen[event.EventID] = struct{}{}
	s.mu.Unlock()
	outcome := "CONFIRMED"
	if duplicate {
		outcome = "DUPLICATE"
	}
	digest := sha256.Sum256([]byte("rehearsal:" + event.EventID))
	return outbox.ProviderReceipt{ProviderKey: "rehearsal.email", Outcome: outcome, ReferenceHash: hex.EncodeToString(digest[:])}, nil
}

func (*RehearsalSink) NetworkEnabled() bool { return false }
