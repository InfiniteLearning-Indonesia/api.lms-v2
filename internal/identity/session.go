package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"regexp"
)

var ErrUnauthenticated = errors.New("UNAUTHENTICATED")
var opaque = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Actor struct {
	ID    string
	Admin bool
}

func TokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// No email/header-role trust and no legacy session import. Tokens are issued
// only by an independent verified issuer (operator fixtures in the current lab).
func Resolve(ctx context.Context, tx pgx.Tx, token string) (Actor, error) {
	var a Actor
	if !opaque.MatchString(token) {
		return a, ErrUnauthenticated
	}
	err := tx.QueryRow(ctx, `SELECT u.id,u.site_admin FROM v3.sessions s JOIN v3.users u ON u.id=s.user_id
 WHERE s.token_hash=$1 AND u.active AND s.revoked_at IS NULL AND s.expires_at>now()`, TokenHash(token)).Scan(&a.ID, &a.Admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, ErrUnauthenticated
	}
	return a, err
}
