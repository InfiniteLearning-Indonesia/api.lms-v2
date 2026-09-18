// Package notifications owns user delivery preferences, safe delivery planning
// and redacted operational visibility. It never resolves secret references or
// performs network I/O.
package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

var (
	ErrInvalid      = errors.New("INVALID_INPUT")
	ErrForbidden    = errors.New("FORBIDDEN")
	ErrConflict     = errors.New("CONFLICT")
	keyPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)
	templatePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

type Store struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

type Preference struct {
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
	Version int64  `json:"version"`
}

type DeliveryStatus struct {
	EventID       string `json:"event_id"`
	Topic         string `json:"topic"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error_code,omitempty"`
	Provider      string `json:"provider_key,omitempty"`
	Outcome       string `json:"provider_outcome,omitempty"`
	ReferenceHash string `json:"reference_hash,omitempty"`
	DeadLetter    bool   `json:"dead_letter"`
}

type NotificationStatus struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	UserID      string `json:"user_id"`
	Channel     string `json:"channel"`
	Template    string `json:"template_key"`
	Source      string `json:"source"`
	Status      string `json:"status"`
	Suppression string `json:"suppression_code,omitempty"`
}

func id() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}

func (s *Store) Preference(ctx context.Context, token string) (Preference, error) {
	result := Preference{Channel: "EMAIL", Enabled: true}
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		a, err := identity.ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT enabled,version FROM v3.notification_preference_events WHERE user_id=$1 AND channel='EMAIL' ORDER BY version DESC LIMIT 1`, a.ID).Scan(&result.Enabled, &result.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return result, err
}

func (s *Store) SetPreference(ctx context.Context, token, key string, expected int64, enabled bool) (json.RawMessage, bool, error) {
	if !keyPattern.MatchString(key) || expected < 0 {
		return nil, false, ErrInvalid
	}
	hash := identity.TokenHash(strings.Join([]string{"EMAIL", key, strconv.FormatInt(expected, 10), strings.ToUpper(jsonBool(enabled))}, ":"))
	var response json.RawMessage
	replayed := false
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		a, err := identity.ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", a.ID+"/notification/"+key); err != nil {
			return err
		}
		var prior string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.notification_commands WHERE actor_id=$1 AND key=$2", a.ID, key).Scan(&prior, &response)
		if err == nil {
			if prior != hash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var current int64
		if err = tx.QueryRow(ctx, `SELECT coalesce(max(version),0) FROM v3.notification_preference_events WHERE user_id=$1 AND channel='EMAIL'`, a.ID).Scan(&current); err != nil {
			return err
		}
		if current != expected {
			return ErrConflict
		}
		result := Preference{Channel: "EMAIL", Enabled: enabled, Version: current + 1}
		if _, err = tx.Exec(ctx, `INSERT INTO v3.notification_preference_events(user_id,channel,version,enabled,actor_id) VALUES($1,'EMAIL',$2,$3,$1)`, a.ID, result.Version, enabled); err != nil {
			return err
		}
		event, genErr := id()
		if genErr != nil {
			return genErr
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,action,request_id) VALUES($1,$2,'notification.preference',$1)", event, a.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id,topic) VALUES($1,'notification.preference.v1')", event); err != nil {
			return err
		}
		response, _ = json.Marshal(result)
		_, err = tx.Exec(ctx, "INSERT INTO v3.notification_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb)", a.ID, key, hash, string(response))
		return err
	})
	return response, replayed, err
}

func jsonBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// Plan creates an immutable intent. Historical imports are always suppressed;
// operational delivery is fail-closed when the provider is absent/disabled.
func (s *Store) Plan(ctx context.Context, eventID, userID, template, source string) (NotificationStatus, error) {
	var result NotificationStatus
	if len(userID) != 32 || !templatePattern.MatchString(template) || (source != "OPERATIONAL" && source != "HISTORICAL_IMPORT") {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		enabled := true
		if err := tx.QueryRow(ctx, `SELECT enabled FROM v3.notification_preference_events WHERE user_id=$1 AND channel='EMAIL' ORDER BY version DESC LIMIT 1`, userID).Scan(&enabled); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		provider := false
		if err := tx.QueryRow(ctx, `SELECT enabled FROM v3ops.provider_binding_events WHERE channel='EMAIL' ORDER BY version DESC LIMIT 1`).Scan(&provider); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		status, code := "QUEUED", ""
		if source == "HISTORICAL_IMPORT" {
			status, code = "SUPPRESSED", "HISTORICAL_IMPORT"
		} else if !enabled {
			status, code = "SUPPRESSED", "PREFERENCE_DISABLED"
		} else if !provider {
			status, code = "SUPPRESSED", "PROVIDER_DISABLED"
		}
		newID, err := id()
		if err != nil {
			return err
		}
		result = NotificationStatus{newID, eventID, userID, "EMAIL", template, source, status, code}
		_, err = tx.Exec(ctx, `INSERT INTO v3.notification_deliveries(id,event_id,user_id,channel,template_key,source,status,suppression_code) VALUES($1,$2,$3,'EMAIL',$4,$5,$6,NULLIF($7,''))`, newID, eventID, userID, template, source, status, code)
		return err
	})
	return result, err
}

func (s *Store) Deliveries(ctx context.Context, token string) ([]DeliveryStatus, error) {
	result := []DeliveryStatus{}
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		a, err := identity.ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if !a.Admin {
			return ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT o.event_id,o.topic,o.status,o.attempts,coalesce(o.last_error_code,''),coalesce(r.provider_key,''),coalesce(r.outcome,''),coalesce(r.reference_hash,''),f.event_id IS NOT NULL
 FROM v3.outbox o LEFT JOIN LATERAL(SELECT provider_key,outcome,reference_hash,occurred_at FROM v3ops.outbox_provider_receipts WHERE event_id=o.event_id ORDER BY attempt_number DESC LIMIT 1)r ON true
 LEFT JOIN v3ops.outbox_failures f ON f.event_id=o.event_id
 ORDER BY (r.provider_key IS NOT NULL) DESC,(f.event_id IS NOT NULL) DESC,r.occurred_at DESC NULLS LAST,o.next_attempt,o.event_id LIMIT 200`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v DeliveryStatus
			if err = rows.Scan(&v.EventID, &v.Topic, &v.Status, &v.Attempts, &v.LastError, &v.Provider, &v.Outcome, &v.ReferenceHash, &v.DeadLetter); err != nil {
				return err
			}
			result = append(result, v)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) Notifications(ctx context.Context, token string) ([]NotificationStatus, error) {
	result := []NotificationStatus{}
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		a, err := identity.ResolveBridge(ctx, tx, token)
		if err != nil {
			return err
		}
		if !a.Admin {
			return ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT id,event_id,user_id,channel,template_key,source,status,coalesce(suppression_code,'') FROM v3.notification_deliveries ORDER BY created_at,id LIMIT 200`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v NotificationStatus
			if err = rows.Scan(&v.ID, &v.EventID, &v.UserID, &v.Channel, &v.Template, &v.Source, &v.Status, &v.Suppression); err != nil {
				return err
			}
			result = append(result, v)
		}
		return rows.Err()
	})
	return result, err
}
