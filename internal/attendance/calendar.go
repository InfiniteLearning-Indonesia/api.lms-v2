package attendance

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
var timezonePattern = regexp.MustCompile(`^[A-Za-z_]+(?:/[A-Za-z0-9_+.-]+)*$`)

type Meeting struct {
	ID        string    `json:"id"`
	ClassID   string    `json:"class_id"`
	Title     string    `json:"title"`
	LocalDate string    `json:"local_date"`
	Timezone  string    `json:"timezone"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Version   int64     `json:"version"`
	State     string    `json:"state"`
}

type Calendar struct {
	ClassID         string    `json:"class_id"`
	PlannedMeetings int       `json:"planned_meetings"`
	ActiveMeetings  int       `json:"active_meetings"`
	Meetings        []Meeting `json:"meetings"`
}

type Command struct {
	Action    string
	ClassID   string
	MeetingID string
	Title     string
	LocalDate string
	Timezone  string
	StartsAt  time.Time
	EndsAt    time.Time
	Version   int64
	Reason    string
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
		if c.MeetingID != "" || !safeText(c.Title, 160) || c.Timezone == "Local" || !timezonePattern.MatchString(c.Timezone) || len(c.Timezone) > 64 || c.StartsAt.IsZero() || c.EndsAt.IsZero() {
			return ErrInvalid
		}
		location, err := time.LoadLocation(c.Timezone)
		if err != nil || c.LocalDate != c.StartsAt.In(location).Format("2006-01-02") || c.EndsAt.Sub(c.StartsAt) < time.Minute || c.EndsAt.Sub(c.StartsAt) > 8*time.Hour {
			return ErrInvalid
		}
		if c.Version != 0 || c.Reason != "" {
			return ErrInvalid
		}
	case "cancel", "reinstate":
		if !idPattern.MatchString(c.MeetingID) || c.Version < 1 || !safeText(c.Reason, 240) || c.Title != "" || c.LocalDate != "" || c.Timezone != "" || !c.StartsAt.IsZero() || !c.EndsAt.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func hashCommand(c Command) string {
	data, _ := json.Marshal(c)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Store) authorize(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string, write bool) error {
	var state string
	query := "SELECT state FROM v3.classes WHERE id=$1 FOR SHARE"
	if write {
		query = "SELECT state FROM v3.classes WHERE id=$1 FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, classID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var closed bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", classID).Scan(&closed); err != nil {
		return err
	}
	if actor.Admin {
		if write && (state == "ARCHIVED" || closed) {
			return ErrConflict
		}
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT r.role FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) FOR SHARE OF e,r`, classID, actor.ID)
	if err != nil {
		return err
	}
	teacher, student := false, false
	for rows.Next() {
		var role string
		if rows.Scan(&role) != nil {
			rows.Close()
			return ErrForbidden
		}
		teacher = teacher || role == "teacher"
		student = student || role == "student"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if write {
		if !teacher {
			return ErrForbidden
		}
		if state == "ARCHIVED" || closed {
			return ErrConflict
		}
		return nil
	}
	if !teacher && (!student || state != "PUBLISHED" || closed) {
		return ErrForbidden
	}
	return nil
}

func overlap(ctx context.Context, tx pgx.Tx, classID, excludeID string, start, end time.Time) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.class_meetings m
 LEFT JOIN LATERAL (SELECT kind FROM v3.meeting_exceptions WHERE meeting_id=m.id ORDER BY revision DESC LIMIT 1) x ON true
 WHERE m.class_id=$1 AND m.id<>$2 AND m.starts_at<$4 AND m.ends_at>$3 AND x.kind IS DISTINCT FROM 'CANCELLED')`, classID, excludeID, start, end).Scan(&found)
	return found, err
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

func (s *Store) Execute(ctx context.Context, session, key, requestID string, command Command) (json.RawMessage, bool, error) {
	if s == nil || s.Pool == nil || !keyPattern.MatchString(key) || requestID == "" {
		return nil, false, ErrInvalid
	}
	if err := validate(command); err != nil {
		return nil, false, err
	}
	digest := hashCommand(command)
	var response json.RawMessage
	replayed := false
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.authorize(ctx, tx, actor, command.ClassID, true); err != nil {
			return err
		}
		var oldHash string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.meeting_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&oldHash, &response)
		if err == nil {
			if oldHash != digest {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var meeting Meeting
		switch command.Action {
		case "create":
			found, e := overlap(ctx, tx, command.ClassID, "", command.StartsAt, command.EndsAt)
			if e != nil {
				return e
			}
			if found {
				return ErrConflict
			}
			meeting.ID, e = token()
			if e != nil {
				return e
			}
			meeting.ClassID, meeting.Title, meeting.LocalDate, meeting.Timezone = command.ClassID, command.Title, command.LocalDate, command.Timezone
			meeting.StartsAt, meeting.EndsAt, meeting.Version, meeting.State = command.StartsAt, command.EndsAt, 1, "SCHEDULED"
			_, e = tx.Exec(ctx, `INSERT INTO v3.class_meetings(id,class_id,title,local_date,timezone,starts_at,ends_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, meeting.ID, meeting.ClassID, meeting.Title, meeting.LocalDate, meeting.Timezone, meeting.StartsAt, meeting.EndsAt, actor.ID)
			if e != nil {
				return e
			}
		default:
			err = tx.QueryRow(ctx, `SELECT m.id,m.class_id,m.title,m.local_date::text,m.timezone,m.starts_at,m.ends_at FROM v3.class_meetings m WHERE m.id=$1 AND m.class_id=$2 FOR UPDATE`, command.MeetingID, command.ClassID).Scan(&meeting.ID, &meeting.ClassID, &meeting.Title, &meeting.LocalDate, &meeting.Timezone, &meeting.StartsAt, &meeting.EndsAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			var latest string
			var revision int64
			err = tx.QueryRow(ctx, `SELECT revision,kind FROM v3.meeting_exceptions WHERE meeting_id=$1 ORDER BY revision DESC LIMIT 1`, meeting.ID).Scan(&revision, &latest)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if command.Version != revision+1 {
				return ErrConflict
			}
			kind := "CANCELLED"
			if command.Action == "cancel" {
				if latest == "CANCELLED" {
					return ErrConflict
				}
				meeting.State = "CANCELLED"
			} else {
				if latest != "CANCELLED" {
					return ErrConflict
				}
				found, e := overlap(ctx, tx, command.ClassID, meeting.ID, meeting.StartsAt, meeting.EndsAt)
				if e != nil {
					return e
				}
				if found {
					return ErrConflict
				}
				kind, meeting.State = "REINSTATED", "SCHEDULED"
			}
			id, e := token()
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO v3.meeting_exceptions(id,class_id,meeting_id,revision,kind,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, command.ClassID, meeting.ID, revision+1, kind, command.Reason, actor.ID)
			if e != nil {
				return e
			}
			meeting.Version = revision + 2
		}
		if err = audit(ctx, tx, actor.ID, command.ClassID, "meeting."+command.Action, requestID); err != nil {
			return err
		}
		response, err = json.Marshal(map[string]any{"meeting": meeting})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO v3.meeting_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4)`, actor.ID, key, digest, response)
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

func (s *Store) Read(ctx context.Context, session, classID string) (Calendar, error) {
	result := Calendar{ClassID: classID, Meetings: []Meeting{}}
	if s == nil || s.Pool == nil || !idPattern.MatchString(classID) {
		return Calendar{}, ErrInvalid
	}
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.authorize(ctx, tx, actor, classID, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT m.id,m.class_id,m.title,m.local_date::text,m.timezone,m.starts_at,m.ends_at,1+coalesce(x.revision,0),CASE WHEN x.kind='CANCELLED' THEN 'CANCELLED' ELSE 'SCHEDULED' END
 FROM v3.class_meetings m LEFT JOIN LATERAL (SELECT revision,kind FROM v3.meeting_exceptions WHERE meeting_id=m.id ORDER BY revision DESC LIMIT 1) x ON true
		 WHERE m.class_id=$1 ORDER BY m.starts_at,m.id LIMIT 1001`, classID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var meeting Meeting
			if rows.Scan(&meeting.ID, &meeting.ClassID, &meeting.Title, &meeting.LocalDate, &meeting.Timezone, &meeting.StartsAt, &meeting.EndsAt, &meeting.Version, &meeting.State) != nil {
				rows.Close()
				return ErrConflict
			}
			result.Meetings = append(result.Meetings, meeting)
			result.PlannedMeetings++
			if meeting.State == "SCHEDULED" {
				result.ActiveMeetings++
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if result.PlannedMeetings > 1000 {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return Calendar{}, err
	}
	return result, nil
}
