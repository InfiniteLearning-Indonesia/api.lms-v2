// Package outbox owns the durable, leased hand-off between domain commits and
// external providers. Provider calls happen after ClaimBatch commits, never
// while a database row lock or domain transaction is held.
package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrLeaseLost    = errors.New("OUTBOX_LEASE_LOST")
	ErrInvalidInput = errors.New("OUTBOX_INPUT_REJECTED")
	topicPattern    = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
	errorPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	providerPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,160}$`)
	hashPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

const (
	defaultTopic         = "audit.v1"
	defaultLeaseDuration = 30 * time.Second
	defaultBackoff       = time.Second
	maximumBackoff       = 5 * time.Minute
)

type Lease struct {
	EventID string
	Token   string
	Attempt int
}

// RecordReceipt is fenced by the live lease. A provider call may still happen
// more than once after a crash, but every confirmed/duplicate observation is
// retained and cannot be rewritten.
func (s Store) RecordReceipt(ctx context.Context, lease Lease, receipt ProviderReceipt) error {
	if s.Pool == nil || lease.EventID == "" || lease.Token == "" || lease.Attempt < 1 ||
		!providerPattern.MatchString(receipt.ProviderKey) || !hashPattern.MatchString(receipt.ReferenceHash) ||
		(receipt.Outcome != "CONFIRMED" && receipt.Outcome != "DUPLICATE") {
		return ErrInvalidInput
	}
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO v3ops.outbox_provider_receipts(event_id,attempt_number,lease_id,provider_key,outcome,reference_hash)
 SELECT event_id,$3,$2,$4,$5,$6 FROM v3.outbox
 WHERE event_id=$1 AND lease_id=$2 AND attempts=$3 AND status='LEASED' AND lease_until>clock_timestamp()
 RETURNING event_id`, lease.EventID, lease.Token, lease.Attempt, receipt.ProviderKey, receipt.Outcome, receipt.ReferenceHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	return err
}

type Event struct {
	Lease
	Topic   string
	Action  string
	ClassID *string
}

type Store struct {
	Pool        *pgxpool.Pool
	Topic       string
	BackoffBase time.Duration
	MaxAttempts int
}

func (s Store) topic() (string, error) {
	topic := s.Topic
	if topic == "" {
		topic = defaultTopic
	}
	if !topicPattern.MatchString(topic) {
		return "", ErrInvalidInput
	}
	return topic, nil
}

func token() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

// Claim preserves the original single-item API while using the durable batch
// implementation. Delivery remains at-least-once: sinks must deduplicate the
// immutable EventID when calling a provider.
func (s Store) Claim(ctx context.Context) (Lease, error) {
	events, err := s.ClaimBatch(ctx, 1, defaultLeaseDuration)
	if err != nil || len(events) == 0 {
		return Lease{}, err
	}
	return events[0].Lease, nil
}

func (s Store) ClaimBatch(ctx context.Context, limit int, leaseDuration time.Duration) ([]Event, error) {
	if s.Pool == nil || limit < 1 || limit > 100 || leaseDuration <= 0 || leaseDuration > 10*time.Minute {
		return nil, ErrInvalidInput
	}
	topic, err := s.topic()
	if err != nil {
		return nil, err
	}
	leaseID, err := token()
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `WITH candidate AS (
	SELECT q.event_id FROM v3.outbox q
	WHERE q.topic=$1 AND ((q.status='PENDING' AND q.next_attempt<=clock_timestamp()) OR
	                      (q.status='LEASED' AND q.lease_until<=clock_timestamp()))
	 AND NOT EXISTS(SELECT 1 FROM v3ops.outbox_failures f WHERE f.event_id=q.event_id)
	ORDER BY q.next_attempt,q.event_id FOR UPDATE OF q SKIP LOCKED LIMIT $2
), claimed AS (
 UPDATE v3.outbox o SET status='LEASED',lease_id=$3,
  lease_until=clock_timestamp()+($4::bigint*interval '1 millisecond'),
  attempts=o.attempts+1,last_error_code=NULL
 FROM candidate c WHERE o.event_id=c.event_id
 RETURNING o.event_id,o.lease_id,o.attempts,o.topic
), recorded AS (
 INSERT INTO v3ops.outbox_attempts(event_id,lease_id,attempt_number,outcome)
 SELECT event_id,lease_id,attempts,'CLAIMED' FROM claimed
)
SELECT c.event_id,c.lease_id,c.attempts,c.topic,a.action,a.class_id
FROM claimed c JOIN v3.audit a ON a.id=c.event_id ORDER BY c.event_id`, topic, limit, leaseID, leaseDuration.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		if err = rows.Scan(&event.EventID, &event.Token, &event.Attempt, &event.Topic, &event.Action, &event.ClassID); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Ack is a fenced, atomic state transition plus immutable attempt record.
func (s Store) Ack(ctx context.Context, lease Lease) error {
	if s.Pool == nil || lease.EventID == "" || lease.Token == "" || lease.Attempt < 1 {
		return ErrInvalidInput
	}
	var id string
	err := s.Pool.QueryRow(ctx, `WITH changed AS (
 UPDATE v3.outbox SET status='SENT',sent_at=clock_timestamp(),lease_id=NULL,lease_until=NULL,last_error_code=NULL
 WHERE event_id=$1 AND lease_id=$2 AND attempts=$3 AND status='LEASED' AND lease_until>clock_timestamp()
 RETURNING event_id
), recorded AS (
 INSERT INTO v3ops.outbox_attempts(event_id,lease_id,attempt_number,outcome)
 SELECT event_id,$2,$3,'DELIVERED' FROM changed
)
SELECT event_id FROM changed`, lease.EventID, lease.Token, lease.Attempt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	return err
}

// Retry retains the compatibility entry point and records a bounded generic
// provider error rather than raw error text, which may contain secrets or PII.
func (s Store) Retry(ctx context.Context, lease Lease) error {
	return s.RetryWithCode(ctx, lease, "PROVIDER_RETRY")
}

func (s Store) RetryWithCode(ctx context.Context, lease Lease, code string) error {
	if s.Pool == nil || lease.EventID == "" || lease.Token == "" || lease.Attempt < 1 || !errorPattern.MatchString(code) {
		return ErrInvalidInput
	}
	base := s.BackoffBase
	if base <= 0 {
		base = defaultBackoff
	}
	if base > maximumBackoff {
		base = maximumBackoff
	}
	multiplier := int64(1) << min(lease.Attempt, 8)
	delay := time.Duration(multiplier) * base
	if delay > maximumBackoff {
		delay = maximumBackoff
	}
	maxAttempts := s.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 12
	}
	if maxAttempts < 1 || maxAttempts > 1000 {
		return ErrInvalidInput
	}
	var id string
	err := s.Pool.QueryRow(ctx, `WITH changed AS (
 UPDATE v3.outbox SET status='PENDING',next_attempt=clock_timestamp()+($4::bigint*interval '1 millisecond'),
  lease_id=NULL,lease_until=NULL,last_error_code=$5
 WHERE event_id=$1 AND lease_id=$2 AND attempts=$3 AND status='LEASED' AND lease_until>clock_timestamp()
	RETURNING event_id,attempts
), failed AS (
	INSERT INTO v3ops.outbox_failures(event_id,final_attempt,error_code)
	SELECT event_id,attempts,$5 FROM changed WHERE attempts >= $6
), recorded AS (
	INSERT INTO v3ops.outbox_attempts(event_id,lease_id,attempt_number,outcome,error_code)
	SELECT event_id,$2,$3,CASE WHEN attempts >= $6 THEN 'FAILED' ELSE 'RETRY_SCHEDULED' END,$5 FROM changed
)
SELECT event_id FROM changed`, lease.EventID, lease.Token, lease.Attempt, delay.Milliseconds(), code, maxAttempts).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	return err
}
