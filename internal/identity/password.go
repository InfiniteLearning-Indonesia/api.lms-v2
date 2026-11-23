package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

const PasswordIssuer = "urn:lms-v3:password"
const passwordIterations = 600000
const passwordSaltSize = 16
const passwordScheme = "pbkdf2-sha256:600000"

var ErrPasswordInput = errors.New("PASSWORD_INPUT_INVALID")
var ErrPasswordRate = errors.New("PASSWORD_RATE_LIMITED")
var localUserID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// PasswordSchema requires the core and identity migrations. The runtime role
// receives DML grants separately and never installs the schema.
const PasswordSchema = `CREATE SCHEMA v3password;
CREATE TABLE v3password.accounts (
 user_id text PRIMARY KEY REFERENCES v3identity.links(user_id),
 email text NOT NULL UNIQUE CHECK(length(email) BETWEEN 3 AND 254 AND email=lower(email)),
 activated_at timestamptz,
 suspended_at timestamptz
);
CREATE TABLE v3password.credentials (
 user_id text PRIMARY KEY REFERENCES v3password.accounts(user_id),
 scheme text NOT NULL DEFAULT 'pbkdf2-sha256:600000' CHECK(scheme='pbkdf2-sha256:600000'),
 salt bytea NOT NULL CHECK(octet_length(salt)=16),
 password_hash bytea NOT NULL CHECK(octet_length(password_hash)=32),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE v3password.tokens (
 token_hash text PRIMARY KEY CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 user_id text NOT NULL REFERENCES v3password.accounts(user_id),
 purpose text NOT NULL CHECK(purpose IN ('invite','reset')),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
CREATE UNIQUE INDEX password_one_live_token ON v3password.tokens(user_id,purpose) WHERE consumed_at IS NULL;
CREATE TABLE v3password.deliveries (
 id text PRIMARY KEY CHECK(id ~ '^[a-f0-9]{64}$'),
 token_hash text NOT NULL UNIQUE REFERENCES v3password.tokens(token_hash),
 payload bytea NOT NULL CHECK(octet_length(payload)=92),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE v3password.limits (
 scope text NOT NULL CHECK(scope IN ('login','reset','consume')),
 bucket integer NOT NULL CHECK(bucket BETWEEN 0 AND 4096),
 window_start timestamptz NOT NULL,
 attempts integer NOT NULL CHECK(attempts>0),
 PRIMARY KEY(scope,bucket)
);`

// Passwords shares the session lifecycle and authorization fence. Delivery
// records carry an authenticated ciphertext, never a reusable plaintext token.
type Passwords struct {
	lifecycle *Lifecycle
	delivery  cipher.AEAD
	key       [32]byte
	hashing   chan struct{}
}

func NewPasswords(lifecycle *Lifecycle, key []byte) (*Passwords, error) {
	if lifecycle == nil || lifecycle.Pool == nil || len(key) != 32 {
		return nil, ErrForbidden
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	return &Passwords{lifecycle: lifecycle, delivery: gcm, key: [32]byte(key), hashing: make(chan struct{}, 2)}, nil
}

func accountEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) < 3 || len(email) > 254 {
		return "", ErrPasswordInput
	}
	for _, r := range email {
		if r < 33 || r > 126 {
			return "", ErrPasswordInput
		}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email || strings.Count(email, "@") != 1 {
		return "", ErrPasswordInput
	}
	return email, nil
}

func validPassword(password string) bool {
	return len(password) <= 1024 && utf8.ValidString(password) && utf8.RuneCountInString(password) >= 15
}

func (s *Passwords) hash(ctx context.Context, password string, salt []byte) ([]byte, error) {
	select {
	case s.hashing <- struct{}{}:
		defer func() { <-s.hashing }()
	default:
		return nil, ErrPasswordRate
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
}

func (s *Passwords) bucket(value string) int {
	// An unpredictable bucket distribution bounds persistent rows without
	// storing addresses or allowing targeted collisions without the server key.
	digest := hmac.New(sha256.New, s.key[:])
	digest.Write([]byte("lms-v3:password-rate\x00" + value))
	return 1 + int(binary.BigEndian.Uint16(digest.Sum(nil)[:2])%4096)
}

func (s *Passwords) rate(ctx context.Context, scope, identifier string, perBucket, global int) error {
	var allowed bool
	err := transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		for _, pair := range [][2]int{{0, global}, {s.bucket(identifier), perBucket}} {
			var hit int
			if err := tx.QueryRow(ctx, `INSERT INTO v3password.limits(scope,bucket,window_start,attempts)
    VALUES($1,$2,date_trunc('minute',statement_timestamp()),1)
    ON CONFLICT(scope,bucket) DO UPDATE SET window_start=GREATEST(v3password.limits.window_start,EXCLUDED.window_start),
    attempts=CASE WHEN v3password.limits.window_start>=EXCLUDED.window_start
      THEN LEAST(v3password.limits.attempts+1,1000) ELSE 1 END RETURNING attempts`, scope, pair[0]).Scan(&hit); err != nil {
				return err
			}
			if hit > pair[1] {
				return nil
			}
		}
		allowed = true
		return nil
	})
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPasswordRate
	}
	return nil
}

func (s *Passwords) queue(ctx context.Context, tx pgx.Tx, user, purpose string) error {
	token, err := RandomToken()
	if err != nil {
		return err
	}
	id, err := RandomToken()
	if err != nil {
		return err
	}
	lifetime := "30 minutes"
	if purpose == "invite" {
		lifetime = "24 hours"
	}
	if _, err = tx.Exec(ctx, `UPDATE v3password.tokens SET consumed_at=clock_timestamp()
 WHERE user_id=$1 AND purpose=$2 AND consumed_at IS NULL`, user, purpose); err != nil {
		return err
	}
	hash := TokenHash(token)
	if _, err = tx.Exec(ctx, `INSERT INTO v3password.tokens(token_hash,user_id,purpose,expires_at)
 VALUES($1,$2,$3,clock_timestamp()+$4::text::interval)`, hash, user, purpose, lifetime); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v3password.deliveries(id,token_hash,payload) VALUES($1,$2,$3)`,
		id, hash, s.delivery.Seal(nil, nil, []byte(token), deliveryAAD(id, hash, user, purpose)))
	return err
}

func deliveryAAD(id, hash, user, purpose string) []byte {
	return []byte("lms-v3:account-delivery\x00" + id + "\x00" + hash + "\x00" + user + "\x00" + purpose)
}

// Invite provisions a new password-authority identity or reissues an invitation
// for an unactivated identity. Enrollment and Class roles are M05 concerns.
func (s *Passwords) Invite(ctx context.Context, session, csrf, rawEmail string) (string, error) {
	email, err := accountEmail(rawEmail)
	if err != nil {
		return "", err
	}
	var user string
	err = transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		actor, err := ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if !actor.Admin {
			return ErrForbidden
		}
		if err = csrfCheck(ctx, tx, session, csrf); err != nil {
			return err
		}
		user, err = s.inviteAccount(ctx, tx, actor.ID, email, false)
		return err
	})
	if err != nil {
		return "", err
	}
	return user, nil
}

// ProvisionForClass runs under the caller's exclusive identity fence and in
// the same transaction as the Class command. The gateway validates CSRF and
// the caller resolves the session actor again under that fence.
func (s *Passwords) ProvisionForClass(ctx context.Context, tx pgx.Tx, actor Actor, rawEmail string) (string, error) {
	if !actor.Admin {
		return "", ErrForbidden
	}
	email, err := accountEmail(rawEmail)
	if err != nil {
		return "", err
	}
	return s.inviteAccount(ctx, tx, actor.ID, email, true)
}

func (s *Passwords) inviteAccount(ctx context.Context, tx pgx.Tx, actor, email string, reuse bool) (string, error) {
	var user, issuer string
	var activated, suspended, enabled, linked bool
	err := tx.QueryRow(ctx, `SELECT a.user_id,l.issuer,a.activated_at IS NOT NULL,a.suspended_at IS NOT NULL,
 u.active,l.active AND l.valid_until>clock_timestamp()
 FROM v3password.accounts a JOIN v3identity.links l ON l.user_id=a.user_id JOIN v3.users u ON u.id=a.user_id
 WHERE a.email=$1`, email).Scan(&user, &issuer, &activated, &suspended, &enabled, &linked)
	if errors.Is(err, pgx.ErrNoRows) {
		key, err := RandomToken()
		if err != nil {
			return "", err
		}
		user = key[:32]
		if _, err = tx.Exec(ctx, "INSERT INTO v3.users(id,active,site_admin) VALUES($1,false,false)", user); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v3identity.links(user_id,issuer,subject,version,active,valid_until)
 VALUES($1,$2,$1,1,false,'infinity')`, user, PasswordIssuer); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3identity.site_roles(user_id) VALUES($1)", user); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3password.accounts(user_id,email) VALUES($1,$2)", user, email); err != nil {
			return "", err
		}
	} else {
		if err != nil {
			return "", err
		}
		if issuer != PasswordIssuer || suspended || (activated && (!reuse || !enabled || !linked)) ||
			(!activated && (enabled || linked)) {
			return "", ErrConflict
		}
		if activated {
			return user, nil
		}
		if reuse {
			var live bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3password.tokens WHERE user_id=$1
 AND purpose='invite' AND consumed_at IS NULL AND expires_at>clock_timestamp())`, user).Scan(&live); err != nil {
				return "", err
			}
			if live {
				return user, nil
			}
		}
	}
	if err = s.queue(ctx, tx, user, "invite"); err != nil {
		return "", err
	}
	return user, s.lifecycle.event(ctx, tx, actor, "identity.invite_account")
}

// RequestReset returns success for both known and unknown addresses. Only
// activated, enabled local accounts create a delivery.
func (s *Passwords) RequestReset(ctx context.Context, rawEmail string) error {
	email, err := accountEmail(rawEmail)
	if err != nil {
		return err
	}
	if err = s.rate(ctx, "reset", email, 3, 60); err != nil {
		return err
	}
	return transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		var user string
		err := tx.QueryRow(ctx, `SELECT a.user_id FROM v3password.accounts a
   JOIN v3.users u ON u.id=a.user_id JOIN v3identity.links l ON l.user_id=a.user_id
   WHERE a.email=$1 AND a.activated_at IS NOT NULL AND a.suspended_at IS NULL
   AND u.active AND l.active AND l.issuer=$2`, email, PasswordIssuer).Scan(&user)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var pending bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3password.tokens
			WHERE user_id=$1 AND purpose='reset' AND consumed_at IS NULL AND expires_at>clock_timestamp())`, user).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return nil
		}
		if err = s.queue(ctx, tx, user, "reset"); err != nil {
			return err
		}
		return s.lifecycle.event(ctx, tx, user, "identity.reset_requested")
	})
}

func (s *Passwords) SetPassword(ctx context.Context, token, purpose, password string) error {
	if (purpose != "invite" && purpose != "reset") || !opaque.MatchString(token) || !validPassword(password) {
		return ErrPasswordInput
	}
	if err := s.rate(ctx, "consume", TokenHash(token), 6, 60); err != nil {
		return err
	}
	var salt [passwordSaltSize]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return err
	}
	hash, err := s.hash(ctx, password, salt[:])
	if err != nil {
		return err
	}
	return transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		user, err := consumeAccountToken(ctx, tx, TokenHash(token), purpose)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v3password.credentials(user_id,salt,password_hash) VALUES($1,$2,$3)
   ON CONFLICT(user_id) DO UPDATE SET salt=EXCLUDED.salt,password_hash=EXCLUDED.password_hash,updated_at=clock_timestamp()`, user, salt[:], hash); err != nil {
			return err
		}
		if purpose == "invite" {
			if err = activateAccount(ctx, tx, user); err != nil {
				return err
			}
		} else if _, err = tx.Exec(ctx, `UPDATE v3identity.sessions SET revoked_at=clock_timestamp()
   WHERE user_id=$1 AND revoked_at IS NULL`, user); err != nil {
			return err
		}
		return s.lifecycle.event(ctx, tx, user, "identity.password_"+purpose)
	})
}

func consumeAccountToken(ctx context.Context, tx pgx.Tx, hash, purpose string) (string, error) {
	var user string
	err := tx.QueryRow(ctx, `UPDATE v3password.tokens t SET consumed_at=clock_timestamp()
   FROM v3password.accounts a JOIN v3identity.links l ON l.user_id=a.user_id JOIN v3.users u ON u.id=a.user_id
   WHERE t.token_hash=$1 AND t.user_id=a.user_id AND t.purpose=$2 AND t.consumed_at IS NULL
    AND t.expires_at>clock_timestamp() AND a.suspended_at IS NULL AND l.issuer=$3
    AND (($2='invite' AND a.activated_at IS NULL AND NOT l.active AND NOT u.active)
      OR ($2='reset' AND a.activated_at IS NOT NULL AND l.active AND u.active))
   RETURNING t.user_id`, hash, purpose, PasswordIssuer).Scan(&user)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnauthenticated
	}
	return user, err
}

func activateAccount(ctx context.Context, tx pgx.Tx, user string) error {
	if _, err := tx.Exec(ctx, "UPDATE v3password.accounts SET activated_at=clock_timestamp() WHERE user_id=$1", user); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE v3.users SET active=true WHERE id=$1", user); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE v3identity.links SET active=true WHERE user_id=$1 AND issuer=$2", user, PasswordIssuer)
	return err
}

type passwordProof struct {
	user, email string
	salt, hash  []byte
}

func (s *Passwords) verify(ctx context.Context, rawEmail, password string) (passwordProof, error) {
	email, err := accountEmail(rawEmail)
	if err != nil || !validPassword(password) {
		return passwordProof{}, ErrUnauthenticated
	}
	if err = s.rate(ctx, "login", email, 8, 120); err != nil {
		return passwordProof{}, err
	}
	var user string
	salt, expected := make([]byte, passwordSaltSize), make([]byte, 32)
	err = s.lifecycle.Pool.QueryRow(ctx, `SELECT a.user_id,c.salt,c.password_hash
  FROM v3password.accounts a JOIN v3password.credentials c ON c.user_id=a.user_id WHERE a.email=$1 AND c.scheme=$2`, email, passwordScheme).Scan(&user, &salt, &expected)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return passwordProof{}, err
	}
	computed, err := s.hash(ctx, password, salt)
	if err != nil {
		return passwordProof{}, err
	}
	if subtle.ConstantTimeCompare(computed, expected) != 1 || !found {
		return passwordProof{}, ErrUnauthenticated
	}
	return passwordProof{user: user, email: email, salt: salt, hash: expected}, nil
}

func (p passwordProof) version(ctx context.Context, tx pgx.Tx) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `SELECT l.version FROM v3identity.links l
   JOIN v3.users u ON u.id=l.user_id JOIN v3password.accounts a ON a.user_id=l.user_id
   JOIN v3password.credentials c ON c.user_id=l.user_id
   WHERE l.user_id=$1 AND c.password_hash=$2 AND c.salt=$3 AND a.email=$4 AND c.scheme=$6
   AND a.activated_at IS NOT NULL AND a.suspended_at IS NULL AND l.issuer=$5
   AND l.active AND u.active AND l.valid_until>clock_timestamp()`, p.user, p.hash, p.salt, p.email, PasswordIssuer, passwordScheme).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUnauthenticated
	}
	return version, err
}

func (s *Passwords) Login(ctx context.Context, rawEmail, password string) (Credential, error) {
	proof, err := s.verify(ctx, rawEmail, password)
	if err != nil {
		return Credential{}, err
	}
	var result Credential
	err = transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		version, err := proof.version(ctx, tx)
		if err != nil {
			return err
		}
		result, err = issue(ctx, tx, proof.user, version, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		return s.lifecycle.event(ctx, tx, proof.user, "identity.password_login")
	})
	if err != nil {
		return Credential{}, err
	}
	return result, nil
}

// OpenDelivery authenticates a queued payload for an authorized mail worker.
// The caller must select only an unconsumed, unexpired token and verify its
// account state before sending. The return value is never an API response.
func (s *Passwords) OpenDelivery(id, hash, user, purpose string, payload []byte) (string, error) {
	raw, err := s.delivery.Open(nil, nil, payload, deliveryAAD(id, hash, user, purpose))
	if err != nil || !opaque.Match(raw) || TokenHash(string(raw)) != hash {
		return "", ErrUnauthenticated
	}
	return string(raw), nil
}

func (s *Passwords) Suspend(ctx context.Context, session, csrf, target string) error {
	if !localUserID.MatchString(target) {
		return ErrPasswordInput
	}
	return transaction.Within(ctx, s.lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		actor, err := ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if !actor.Admin {
			return ErrForbidden
		}
		if err = csrfCheck(ctx, tx, session, csrf); err != nil {
			return err
		}
		var admin, suspended bool
		err = tx.QueryRow(ctx, `SELECT coalesce(r.admin,false),a.suspended_at IS NOT NULL
   FROM v3password.accounts a JOIN v3identity.links l ON l.user_id=a.user_id
   LEFT JOIN v3identity.site_roles r ON r.user_id=a.user_id WHERE a.user_id=$1 AND l.issuer=$2`,
			target, PasswordIssuer).Scan(&admin, &suspended)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		if suspended {
			return nil
		}
		if admin {
			var others bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3identity.site_roles r
    JOIN v3identity.links l ON l.user_id=r.user_id JOIN v3.users u ON u.id=l.user_id
    WHERE r.user_id<>$1 AND r.admin AND u.active AND l.active AND l.valid_until>clock_timestamp())`, target).Scan(&others); err != nil {
				return err
			}
			if !others {
				return ErrConflict
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE v3password.accounts SET suspended_at=clock_timestamp() WHERE user_id=$1", target); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3.users SET active=false WHERE id=$1", target); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.links SET active=false WHERE user_id=$1 AND issuer=$2", target, PasswordIssuer); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=$1 AND revoked_at IS NULL", target); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE v3password.tokens SET consumed_at=clock_timestamp() WHERE user_id=$1 AND consumed_at IS NULL", target); err != nil {
			return err
		}
		return s.lifecycle.event(ctx, tx, actor.ID, "identity.suspend_account")
	})
}
