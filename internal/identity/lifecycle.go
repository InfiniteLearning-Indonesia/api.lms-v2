package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
	"time"
)

var ErrConflict = errors.New("IDENTITY_CONFLICT")
var ErrForbidden = errors.New("IDENTITY_FORBIDDEN")

// One database-wide authorization fence for this slice. Domain transactions
// share it; credential/authority mutations take it exclusively. All participants
// must follow this protocol; it does not serialize an external legacy server.
func Fence(ctx context.Context, tx pgx.Tx, exclusive bool) error {
	q := "SELECT pg_advisory_xact_lock_shared(1112001)"
	if exclusive {
		q = "SELECT pg_advisory_xact_lock(1112001)"
	}
	_, err := tx.Exec(ctx, q)
	return err
}

func ResolveBridge(ctx context.Context, tx pgx.Tx, token string) (Actor, error) {
	var a Actor
	if !opaque.MatchString(token) {
		return a, ErrUnauthenticated
	}
	if err := Fence(ctx, tx, false); err != nil {
		return a, err
	}
	err := tx.QueryRow(ctx, `SELECT u.id,coalesce(r.admin,false) FROM v3identity.sessions s
 JOIN v3identity.links l ON l.user_id=s.user_id JOIN v3.users u ON u.id=l.user_id
 LEFT JOIN v3identity.site_roles r ON r.user_id=u.id
 WHERE s.token_hash=$1 AND u.active AND l.active AND l.valid_until>clock_timestamp()
 AND s.authority_version=l.version AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.absolute_until>clock_timestamp()`, TokenHash(token)).Scan(&a.ID, &a.Admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, ErrUnauthenticated
	}
	return a, err
}

type Lifecycle struct {
	Pool      *pgxpool.Pool
	Verifier  Verifier
	failAfter func(string) error
}
type Credential struct {
	Token, CSRF       string
	Expires, Absolute time.Time
}

func RandomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Lifecycle) event(ctx context.Context, tx pgx.Tx, user, action string) error {
	id, err := RandomToken()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,action,request_id) VALUES($1,$2,$3,$1)", id, user, action); err != nil {
		return err
	}
	if s.failAfter != nil {
		if err = s.failAfter("audit"); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", id); err != nil {
		return err
	}
	if s.failAfter != nil {
		return s.failAfter("outbox")
	}
	return nil
}
func (s *Lifecycle) Challenge(ctx context.Context) (nonce, binding string, err error) {
	nonce, err = RandomToken()
	if err != nil {
		return
	}
	binding, err = RandomToken()
	if err != nil {
		return
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO v3identity.challenges(nonce_hash,binding_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '60 seconds')`, TokenHash(nonce), TokenHash(binding))
	return
}
func consume(ctx context.Context, tx pgx.Tx, c Assertion) error {
	tag, err := tx.Exec(ctx, "INSERT INTO v3identity.consumed(issuer,jti) VALUES($1,$2) ON CONFLICT DO NOTHING", c.Issuer, c.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
func issue(ctx context.Context, tx pgx.Tx, user string, version int64, absolute time.Time) (Credential, error) {
	var c Credential
	var err error
	c.Token, err = RandomToken()
	if err != nil {
		return c, err
	}
	c.CSRF, err = RandomToken()
	if err != nil {
		return c, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until)
 VALUES($1,$2,$3,$4,LEAST(clock_timestamp()+interval '15 minutes',$5),$5) RETURNING expires_at,absolute_until`, TokenHash(c.Token), user, version, TokenHash(c.CSRF), absolute).Scan(&c.Expires, &c.Absolute)
	return c, err
}
func (s *Lifecycle) Exchange(ctx context.Context, raw, binding string) (Credential, error) {
	c, err := s.Verifier.Verify(raw, "login", time.Now())
	if err != nil || !opaque.MatchString(binding) {
		return Credential{}, ErrUnauthenticated
	}
	var result Credential
	err = transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		var user string
		// Recheck assertion expiry with DB time after waiting for locks.
		err := tx.QueryRow(ctx, `SELECT l.user_id FROM v3identity.links l JOIN v3.users u ON u.id=l.user_id WHERE l.issuer=$1 AND l.subject=$2 AND l.version=$3 AND l.active AND u.active AND $4>clock_timestamp()`, c.Issuer, c.Subject, c.Version, c.ExpiresAt.Time).Scan(&user)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE v3identity.challenges SET consumed_at=clock_timestamp() WHERE nonce_hash=$1 AND binding_hash=$2 AND consumed_at IS NULL AND expires_at>clock_timestamp()`, TokenHash(c.Nonce), TokenHash(binding))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if err = consume(ctx, tx, c); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.links SET valid_until=GREATEST(valid_until,$2) WHERE user_id=$1", user, c.ExpiresAt.Time); err != nil {
			return err
		}
		result, err = issue(ctx, tx, user, c.Version, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		return s.event(ctx, tx, user, "identity.exchange")
	})
	if err != nil {
		return Credential{}, err
	}
	return result, nil
}
func csrfCheck(ctx context.Context, tx pgx.Tx, token, csrf string) error {
	if !opaque.MatchString(csrf) {
		return ErrForbidden
	}
	var hash string
	if err := tx.QueryRow(ctx, "SELECT csrf_hash FROM v3identity.sessions WHERE token_hash=$1", TokenHash(token)).Scan(&hash); err != nil {
		return ErrUnauthenticated
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(TokenHash(csrf))) != 1 {
		return ErrForbidden
	}
	return nil
}
func (s *Lifecycle) Validate(ctx context.Context, token, csrf string, mutation bool) (Actor, error) {
	var actor Actor
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		actor, err = ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if mutation {
			return csrfCheck(ctx, tx, token, csrf)
		}
		return nil
	})
	return actor, err
}
func (s *Lifecycle) SessionCommand(ctx context.Context, token, csrf, action string) (Credential, error) {
	if action != "rotate" && action != "logout" && action != "revoke-all" {
		return Credential{}, ErrForbidden
	}
	var result Credential
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		a, err := ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if err = csrfCheck(ctx, tx, token, csrf); err != nil {
			return err
		}
		if action == "rotate" {
			var v int64
			var absolute time.Time
			if err = tx.QueryRow(ctx, "SELECT authority_version,absolute_until FROM v3identity.sessions WHERE token_hash=$1", TokenHash(token)).Scan(&v, &absolute); err != nil {
				return err
			}
			result, err = issue(ctx, tx, a.ID, v, absolute)
			if err != nil {
				return err
			}
		}
		q := "UPDATE v3identity.sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1 AND revoked_at IS NULL"
		value := TokenHash(token)
		if action == "revoke-all" {
			q = "UPDATE v3identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=$1 AND revoked_at IS NULL"
			value = a.ID
		}
		if _, err = tx.Exec(ctx, q, value); err != nil {
			return err
		}
		return s.event(ctx, tx, a.ID, "identity."+action)
	})
	if err != nil {
		return Credential{}, err
	}
	return result, nil
}
func (s *Lifecycle) Project(ctx context.Context, raw string) error {
	c, err := s.Verifier.Verify(raw, "account_state", time.Now())
	if err != nil {
		return err
	}
	return transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		var user string
		var oldVersion int64
		var oldActive bool
		err := tx.QueryRow(ctx, "SELECT user_id,version,active FROM v3identity.links WHERE issuer=$1 AND subject=$2 AND $3>clock_timestamp()", c.Issuer, c.Subject, c.ExpiresAt.Time).Scan(&user, &oldVersion, &oldActive)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if c.Version <= oldVersion {
			return ErrConflict
		}
		if err = consume(ctx, tx, c); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3identity.account_events(user_id,version,previous_version,previous_active,active) VALUES($1,$2,$3,$4,$5)", user, c.Version, oldVersion, oldActive, *c.Active); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.links SET version=$2,active=$3,valid_until=$4 WHERE user_id=$1", user, c.Version, *c.Active, c.ExpiresAt.Time); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=$1 AND revoked_at IS NULL", user); err != nil {
			return err
		}
		return s.event(ctx, tx, user, "identity.account_projection")
	})
}
func (s *Lifecycle) SiteRole(ctx context.Context, token, csrf, target string, admin bool, version int64) error {
	if len(target) != 32 || version < 1 {
		return ErrForbidden
	}
	return transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		a, err := ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if !a.Admin {
			return ErrForbidden
		}
		if err = csrfCheck(ctx, tx, token, csrf); err != nil {
			return err
		}
		var old bool
		var current int64
		if err = tx.QueryRow(ctx, "SELECT admin,version FROM v3identity.site_roles WHERE user_id=$1", target).Scan(&old, &current); errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		} else if err != nil {
			return err
		}
		if current != version {
			return ErrConflict
		}
		if old && !admin {
			var remaining bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3identity.site_roles r JOIN v3identity.links l ON l.user_id=r.user_id JOIN v3.users u ON u.id=l.user_id WHERE r.admin AND l.active AND u.active AND r.user_id<>$1)`, target).Scan(&remaining); err != nil {
				return err
			}
			if !remaining {
				return ErrConflict
			}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3identity.role_events(user_id,version,previous_admin,admin,actor_id) VALUES($1,$2,$3,$4,$5)", target, current+1, old, admin, a.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.site_roles SET admin=$2,version=version+1 WHERE user_id=$1", target, admin); err != nil {
			return err
		}
		return s.event(ctx, tx, a.ID, "identity.site_role")
	})
}
