package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

// GoogleSchema is additive. The existing identity link remains the session's
// account-state reference. This table adds one Google credential per account.
const GoogleSchema = `CREATE SCHEMA v3google;
CREATE TABLE v3google.links (
 issuer text NOT NULL CHECK(issuer='https://accounts.google.com'),
 subject text NOT NULL CHECK(length(subject) BETWEEN 1 AND 255),
 user_id text NOT NULL UNIQUE REFERENCES v3password.accounts(user_id),
 linked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(issuer,subject)
);
CREATE TABLE v3google.flows (
 state_hash text PRIMARY KEY CHECK(state_hash ~ '^[a-f0-9]{64}$'),
 binding_hash text NOT NULL CHECK(binding_hash ~ '^[a-f0-9]{64}$'),
 mode text NOT NULL CHECK(mode IN ('login','invite','link')),
 user_id text REFERENCES v3password.accounts(user_id),
 proof_hash text CHECK(proof_hash ~ '^[a-f0-9]{64}$'),
 expires_at timestamptz NOT NULL DEFAULT (clock_timestamp()+interval '5 minutes'),
 consumed_at timestamptz,
 CHECK((mode='login' AND user_id IS NULL AND proof_hash IS NULL) OR
       (mode IN ('invite','link') AND user_id IS NOT NULL AND proof_hash IS NOT NULL))
);
CREATE TABLE v3google.start_budget (
 singleton boolean PRIMARY KEY CHECK(singleton),
 window_start timestamptz NOT NULL,
 attempts integer NOT NULL CHECK(attempts BETWEEN 1 AND 121)
);`

type Google struct {
	passwords *Passwords
	provider  *googleProvider
	flowKey   [32]byte
}

func NewGoogle(passwords *Passwords, config GoogleConfig) (*Google, error) {
	if passwords == nil || len(config.FlowKey) != 32 {
		return nil, ErrPasswordInput
	}
	provider, err := newGoogleProvider(config)
	if err != nil {
		return nil, err
	}
	return &Google{passwords: passwords, provider: provider, flowKey: [32]byte(config.FlowKey)}, nil
}

type googleStart struct {
	Mode        string `json:"mode"`
	InviteToken string `json:"invite_token"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (s *Google) derive(state, purpose string) string {
	mac := hmac.New(sha256.New, s.flowKey[:])
	mac.Write([]byte("lms-v3:google:" + purpose + "\x00" + state))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Google) nonce(state string) string {
	// Hash to the same fixed hex format already used for opaque nonces.
	return TokenHash(s.derive(state, "nonce"))
}

func (s *Google) start(ctx context.Context, input googleStart, session, csrf string) (string, string, error) {
	switch input.Mode {
	case "login":
		if input.InviteToken != "" || input.Email != "" || input.Password != "" {
			return "", "", ErrPasswordInput
		}
	case "invite":
		if !opaque.MatchString(input.InviteToken) || input.Email != "" || input.Password != "" {
			return "", "", ErrPasswordInput
		}
	case "link":
		if input.InviteToken != "" || !opaque.MatchString(session) || !opaque.MatchString(csrf) {
			return "", "", ErrForbidden
		}
	default:
		return "", "", ErrPasswordInput
	}
	lifecycle := s.passwords.lifecycle
	var attempts int
	err := lifecycle.Pool.QueryRow(ctx, `INSERT INTO v3google.start_budget VALUES(true,date_trunc('minute',statement_timestamp()),1)
 ON CONFLICT(singleton) DO UPDATE SET window_start=GREATEST(v3google.start_budget.window_start,EXCLUDED.window_start),
 attempts=CASE WHEN v3google.start_budget.window_start>=EXCLUDED.window_start
 THEN LEAST(v3google.start_budget.attempts+1,121) ELSE 1 END RETURNING attempts`).Scan(&attempts)
	if err != nil {
		return "", "", err
	}
	if attempts > 120 {
		return "", "", ErrPasswordRate
	}
	var proof passwordProof
	if input.Mode == "link" {
		if _, err = lifecycle.Validate(ctx, session, csrf, true); err != nil {
			return "", "", err
		}
		proof, err = s.passwords.verify(ctx, input.Email, input.Password)
		if err != nil {
			return "", "", err
		}
	}
	state, err := RandomToken()
	if err != nil {
		return "", "", err
	}
	binding, err := RandomToken()
	if err != nil {
		return "", "", err
	}
	err = transaction.Within(ctx, lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, false); err != nil {
			return err
		}
		var target, bindingProof string
		switch input.Mode {
		case "invite":
			bindingProof = TokenHash(input.InviteToken)
			err := tx.QueryRow(ctx, `SELECT t.user_id FROM v3password.tokens t JOIN v3password.accounts a USING(user_id)
 WHERE t.token_hash=$1 AND t.purpose='invite' AND t.consumed_at IS NULL AND t.expires_at>clock_timestamp()
 AND a.activated_at IS NULL AND a.suspended_at IS NULL`, bindingProof).Scan(&target)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrUnauthenticated
			}
			if err != nil {
				return err
			}
		case "link":
			actor, err := ResolveBridge(ctx, tx, session)
			if err != nil {
				return err
			}
			if actor.ID != proof.user {
				return ErrForbidden
			}
			if err = csrfCheck(ctx, tx, session, csrf); err != nil {
				return err
			}
			if _, err = proof.version(ctx, tx); err != nil {
				return err
			}
			target, bindingProof = actor.ID, TokenHash(session)
		}
		_, err := tx.Exec(ctx, `INSERT INTO v3google.flows(state_hash,binding_hash,mode,user_id,proof_hash)
 VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''))`, TokenHash(state), TokenHash(binding), input.Mode, target, bindingProof)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return s.provider.authorizationURL(state, s.nonce(state), s.derive(state, "pkce")), binding, nil
}

func (s *Google) complete(ctx context.Context, state, code, binding, session string) (Credential, error) {
	if !opaque.MatchString(state) || !opaque.MatchString(binding) || len(code) < 1 || len(code) > 2048 ||
		strings.ContainsAny(code, " \t\r\n\x00") {
		return Credential{}, ErrUnauthenticated
	}
	lifecycle := s.passwords.lifecycle
	var mode, target, proof string
	// Claim state before outbound exchange. A rejected/failed exchange must be
	// restarted, and simultaneous callbacks cannot invoke Google twice.
	err := lifecycle.Pool.QueryRow(ctx, `UPDATE v3google.flows SET consumed_at=clock_timestamp()
 WHERE state_hash=$1 AND binding_hash=$2 AND consumed_at IS NULL AND expires_at>clock_timestamp()
 AND (mode<>'link' OR proof_hash=$3) RETURNING mode,coalesce(user_id,''),coalesce(proof_hash,'')`,
		TokenHash(state), TokenHash(binding), TokenHash(session)).Scan(&mode, &target, &proof)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrUnauthenticated
	}
	if err != nil {
		return Credential{}, err
	}
	claims, err := s.provider.exchange(ctx, code, s.nonce(state), s.derive(state, "pkce"))
	if err != nil {
		return Credential{}, err
	}
	var result Credential
	err = transaction.Within(ctx, lifecycle.Pool, func(tx pgx.Tx) error {
		if err := Fence(ctx, tx, true); err != nil {
			return err
		}
		if mode == "login" {
			err := tx.QueryRow(ctx, `SELECT user_id FROM v3google.links WHERE issuer=$1 AND subject=$2`,
				claims.Issuer, claims.Subject).Scan(&target)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrUnauthenticated
			}
			if err != nil {
				return err
			}
		} else if err := s.link(ctx, tx, mode, target, proof, session, claims); err != nil {
			return err
		}
		var version int64
		err := tx.QueryRow(ctx, `SELECT l.version FROM v3identity.links l JOIN v3.users u ON u.id=l.user_id
 JOIN v3password.accounts a ON a.user_id=l.user_id
 WHERE l.user_id=$1 AND l.issuer=$2 AND l.active AND u.active AND a.activated_at IS NOT NULL
 AND a.suspended_at IS NULL AND l.valid_until>clock_timestamp() AND $3::timestamptz>clock_timestamp()`,
			target, PasswordIssuer, claims.ExpiresAt.Time).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if mode == "link" {
			// Retire the reauthenticated session as the browser receives a new one.
			if _, err = tx.Exec(ctx, `UPDATE v3identity.sessions SET revoked_at=clock_timestamp()
 WHERE token_hash=$1 AND revoked_at IS NULL`, proof); err != nil {
				return err
			}
		}
		result, err = issue(ctx, tx, target, version, time.Now().Add(time.Hour))
		if err != nil {
			return err
		}
		return lifecycle.event(ctx, tx, target, "identity.google_"+mode)
	})
	if err != nil {
		return Credential{}, err
	}
	return result, nil
}

func (s *Google) link(ctx context.Context, tx pgx.Tx, mode, target, proof, session string, claims googleClaims) error {
	if mode == "link" {
		actor, err := ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if actor.ID != target {
			return ErrForbidden
		}
	} else {
		user, err := consumeAccountToken(ctx, tx, proof, "invite")
		if err != nil {
			return err
		}
		if user != target {
			return ErrForbidden
		}
		if err = activateAccount(ctx, tx, target); err != nil {
			return err
		}
	}
	email, err := accountEmail(claims.Email)
	if err != nil || !claims.EmailVerified {
		return ErrUnauthenticated
	}
	var expected string
	if err = tx.QueryRow(ctx, `SELECT email FROM v3password.accounts WHERE user_id=$1`, target).Scan(&expected); err != nil {
		return err
	}
	if expected != email {
		return ErrUnauthenticated
	}
	tag, err := tx.Exec(ctx, `INSERT INTO v3google.links(issuer,subject,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
		claims.Issuer, claims.Subject, target)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
