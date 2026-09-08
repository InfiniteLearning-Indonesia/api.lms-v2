package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseLost = errors.New("OUTBOX_LEASE_LOST")

type Lease struct {
	EventID string
	Token   string
}
type Store struct{ Pool *pgxpool.Pool }

// Delivery is at-least-once. Consumers must deduplicate immutable EventID;
// providers must never be called inside the transaction which creates events.
func (s Store) Claim(ctx context.Context) (Lease, error) {
	var nonce [16]byte
	_, err := rand.Read(nonce[:])
	token := hex.EncodeToString(nonce[:])
	if err != nil {
		return Lease{}, err
	}
	var id string
	err = s.Pool.QueryRow(ctx, `WITH candidate AS (SELECT event_id FROM v3.outbox WHERE
 (status='PENDING' AND next_attempt<=now()) OR (status='LEASED' AND lease_until<=now())
 ORDER BY event_id FOR UPDATE SKIP LOCKED LIMIT 1)
 UPDATE v3.outbox o SET status='LEASED',lease_id=$1,lease_until=now()+interval '30 seconds',attempts=attempts+1
 FROM candidate c WHERE o.event_id=c.event_id RETURNING o.event_id`, token).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, nil
	}
	return Lease{id, token}, err
}
func (s Store) Ack(ctx context.Context, l Lease) error {
	tag, err := s.Pool.Exec(ctx, "UPDATE v3.outbox SET status='SENT' WHERE event_id=$1 AND lease_id=$2 AND status='LEASED' AND lease_until>now()", l.EventID, l.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (s Store) Retry(ctx context.Context, l Lease) error {
	tag, err := s.Pool.Exec(ctx, "UPDATE v3.outbox SET status='PENDING',next_attempt=now()+interval '1 second'*least(300,power(2,least(attempts,8))) WHERE event_id=$1 AND lease_id=$2 AND status='LEASED' AND lease_until>now()", l.EventID, l.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
