package logbook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

var ErrInvalid = errors.New("INVALID_INPUT")
var ErrForbidden = errors.New("FORBIDDEN")
var ErrNotFound = errors.New("NOT_FOUND")
var ErrConflict = errors.New("CONFLICT")
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)
var timezonePattern = regexp.MustCompile(`^(UTC|[A-Za-z_]+(?:/[A-Za-z0-9_+.-]+)+)$`)

type Period struct {
	ID       string    `json:"id"`
	Ordinal  int       `json:"ordinal"`
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	DueAt    time.Time `json:"due_at"`
}

type PeriodSet struct {
	ID       string   `json:"id"`
	ClassID  string   `json:"class_id"`
	Title    string   `json:"title"`
	Timezone string   `json:"timezone"`
	Revision int64    `json:"revision"`
	State    string   `json:"state"`
	Periods  []Period `json:"periods"`
}

type Command struct {
	Action           string
	ClassID          string
	SetID            string
	Title            string
	Timezone         string
	Periods          []Period
	ExpectedActiveID string
}

type Store struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func token() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func safeText(s string, max int) bool {
	if len(s) < 1 || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validate(c Command) error {
	if !idPattern.MatchString(c.ClassID) {
		return ErrInvalid
	}
	switch c.Action {
	case "create":
		if c.SetID != "" || c.ExpectedActiveID != "" || !safeText(c.Title, 160) || !timezonePattern.MatchString(c.Timezone) || len(c.Timezone) > 64 || len(c.Periods) < 1 || len(c.Periods) > 24 {
			return ErrInvalid
		}
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			return ErrInvalid
		}
		for i, p := range c.Periods {
			if p.ID != "" || p.Ordinal != 0 || !safeText(p.Name, 160) || p.StartsAt.IsZero() || p.EndsAt.IsZero() || p.DueAt.IsZero() || !p.StartsAt.Before(p.EndsAt) || p.DueAt.Before(p.EndsAt) {
				return ErrInvalid
			}
			if i > 0 && c.Periods[i-1].EndsAt.After(p.StartsAt) {
				return ErrInvalid
			}
		}
	case "publish":
		if !idPattern.MatchString(c.SetID) || c.Title != "" || c.Timezone != "" || len(c.Periods) != 0 || (c.ExpectedActiveID != "" && !idPattern.MatchString(c.ExpectedActiveID)) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func digest(c Command) string {
	data, _ := json.Marshal(c)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Store) authorize(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string, write bool) (bool, error) {
	query := "SELECT state FROM v3.classes WHERE id=$1 FOR SHARE"
	if write {
		query = "SELECT state FROM v3.classes WHERE id=$1 FOR UPDATE"
	}
	var state string
	err := tx.QueryRow(ctx, query, classID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	var closed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", classID).Scan(&closed); err != nil {
		return false, err
	}
	if actor.Admin {
		if write && (state == "ARCHIVED" || closed) {
			return false, ErrConflict
		}
		return true, nil
	}
	rows, err := tx.Query(ctx, `SELECT r.role FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) FOR SHARE OF e,r`, classID, actor.ID)
	if err != nil {
		return false, err
	}
	teacher, student := false, false
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return false, err
		}
		teacher = teacher || role == "teacher"
		student = student || role == "student"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if write {
		if !teacher {
			return false, ErrForbidden
		}
		if state == "ARCHIVED" || closed {
			return false, ErrConflict
		}
		return true, nil
	}
	if !teacher && (!student || state != "PUBLISHED" || closed) {
		return false, ErrForbidden
	}
	return teacher, nil
}

func audit(ctx context.Context, tx pgx.Tx, actor, classID, action, requestID string) error {
	id, err := token()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", id, actor, classID, action, requestID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", id)
	return err
}

func (s *Store) Execute(ctx context.Context, session, key, requestID string, c Command) (json.RawMessage, bool, error) {
	if s == nil || s.Pool == nil || !keyPattern.MatchString(key) || requestID == "" {
		return nil, false, ErrInvalid
	}
	if err := validate(c); err != nil {
		return nil, false, err
	}
	hash := digest(c)
	var response json.RawMessage
	replayed := false
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if _, err = s.authorize(ctx, tx, actor, c.ClassID, true); err != nil {
			return err
		}
		var prior string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.logbook_period_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
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
		set := PeriodSet{ClassID: c.ClassID, Periods: []Period{}}
		if c.Action == "create" {
			var existing int
			if err = tx.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_period_sets WHERE class_id=$1", c.ClassID).Scan(&existing); err != nil {
				return err
			}
			if existing >= 1000 {
				return ErrConflict
			}
			set.ID, err = token()
			if err != nil {
				return err
			}
			set.Title, set.Timezone, set.State = c.Title, c.Timezone, "DRAFT"
			if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_period_sets(id,class_id,title,timezone,created_by) VALUES($1,$2,$3,$4,$5)`, set.ID, set.ClassID, set.Title, set.Timezone, actor.ID); err != nil {
				return err
			}
			for i, p := range c.Periods {
				p.ID, err = token()
				if err != nil {
					return err
				}
				p.Ordinal = i + 1
				if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_periods(id,set_id,class_id,ordinal,name,starts_at,ends_at,due_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, set.ID, c.ClassID, p.Ordinal, p.Name, p.StartsAt, p.EndsAt, p.DueAt); err != nil {
					return err
				}
				set.Periods = append(set.Periods, p)
			}
		} else {
			err = tx.QueryRow(ctx, `SELECT id,class_id,title,timezone FROM v3.logbook_period_sets WHERE id=$1 AND class_id=$2`, c.SetID, c.ClassID).Scan(&set.ID, &set.ClassID, &set.Title, &set.Timezone)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			var current string
			var revision int64
			err = tx.QueryRow(ctx, `SELECT set_id,revision FROM v3.logbook_period_publications WHERE class_id=$1 ORDER BY revision DESC LIMIT 1`, c.ClassID).Scan(&current, &revision)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if current != c.ExpectedActiveID {
				return ErrConflict
			}
			publicationID, e := token()
			if e != nil {
				return e
			}
			_, err = tx.Exec(ctx, `INSERT INTO v3.logbook_period_publications(id,class_id,set_id,revision,previous_set_id,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, publicationID, c.ClassID, set.ID, revision+1, nilIfEmpty(current), actor.ID)
			if err != nil {
				return err
			}
			set.Revision, set.State = revision+1, "PUBLISHED"
			if err = loadPeriods(ctx, tx, &set); err != nil {
				return err
			}
		}
		if err = audit(ctx, tx, actor.ID, c.ClassID, "logbook.period."+c.Action, requestID); err != nil {
			return err
		}
		response, err = json.Marshal(map[string]any{"set": set})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO v3.logbook_period_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4)`, actor.ID, key, hash, response)
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514") {
			return nil, false, ErrConflict
		}
		return nil, false, err
	}
	return response, replayed, nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func loadPeriods(ctx context.Context, tx pgx.Tx, set *PeriodSet) error {
	rows, err := tx.Query(ctx, `SELECT id,ordinal,name,starts_at,ends_at,due_at FROM v3.logbook_periods WHERE set_id=$1 ORDER BY ordinal`, set.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.Ordinal, &p.Name, &p.StartsAt, &p.EndsAt, &p.DueAt); err != nil {
			return err
		}
		set.Periods = append(set.Periods, p)
	}
	return rows.Err()
}

// Read without a set ID returns only the active publication; a teacher/admin
// may request one exact historical or draft set by ID.
func (s *Store) Read(ctx context.Context, session, classID, setID string) (PeriodSet, error) {
	if s == nil || s.Pool == nil || !idPattern.MatchString(classID) || (setID != "" && !idPattern.MatchString(setID)) {
		return PeriodSet{}, ErrInvalid
	}
	var result PeriodSet
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		teacher, err := s.authorize(ctx, tx, actor, classID, false)
		if err != nil {
			return err
		}
		if setID != "" && !teacher {
			return ErrForbidden
		}
		query := `SELECT s.id,s.class_id,s.title,s.timezone,coalesce(p.revision,0) FROM v3.logbook_period_sets s LEFT JOIN v3.logbook_period_publications p ON p.set_id=s.id WHERE s.class_id=$1 AND s.id=$2`
		if setID == "" {
			query = `SELECT s.id,s.class_id,s.title,s.timezone,p.revision FROM v3.logbook_period_publications p JOIN v3.logbook_period_sets s ON s.id=p.set_id AND s.class_id=p.class_id WHERE p.class_id=$1 ORDER BY p.revision DESC LIMIT 1`
			err = tx.QueryRow(ctx, query, classID).Scan(&result.ID, &result.ClassID, &result.Title, &result.Timezone, &result.Revision)
		} else {
			err = tx.QueryRow(ctx, query, classID, setID).Scan(&result.ID, &result.ClassID, &result.Title, &result.Timezone, &result.Revision)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		result.State, result.Periods = "DRAFT", []Period{}
		if result.Revision > 0 {
			result.State = "PUBLISHED"
		}
		return loadPeriods(ctx, tx, &result)
	})
	if err != nil {
		return PeriodSet{}, err
	}
	return result, nil
}
