package classes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/access"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
	"regexp"
	"strings"
)

var ErrForbidden = errors.New("FORBIDDEN")
var ErrConflict = errors.New("CONFLICT")
var ErrInvalid = errors.New("INVALID_INPUT")
var ErrNotFound = errors.New("NOT_FOUND")
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)

type Class struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProgramLabel string `json:"program_label"`
	CohortLabel  string `json:"cohort_label"`
	State        string `json:"state"`
	Version      int64  `json:"version"`
}
type Command struct {
	Action        string
	ClassID       string
	Name          string
	ProgramLabel  string
	CohortLabel   string
	Version       int64
	UserID        string
	Role          string
	UpdateProgram bool
	UpdateCohort  bool
}
type Store struct {
	pool       *pgxpool.Pool
	failAfter  func(string) error
	resolve    func(context.Context, pgx.Tx, string) (identity.Actor, error)
	contextual bool
}

func New(pool *pgxpool.Pool) *Store           { return &Store{pool: pool} }
func NewBridge(pool *pgxpool.Pool) *Store     { return &Store{pool: pool, resolve: identity.ResolveBridge} }
func NewContextual(pool *pgxpool.Pool) *Store { s := NewBridge(pool); s.contextual = true; return s }
func (s *Store) actor(ctx context.Context, tx pgx.Tx, token string) (identity.Actor, error) {
	if s.resolve != nil {
		return s.resolve(ctx, tx, token)
	}
	return identity.Resolve(ctx, tx, token)
}
func (s *Store) load(ctx context.Context, tx pgx.Tx, id string) (Class, error) {
	c, err := load(ctx, tx, id)
	if err != nil || !s.contextual {
		return c, err
	}
	var closed bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", id).Scan(&closed); err != nil {
		return c, err
	}
	if closed && c.State == "PUBLISHED" {
		c.State = "CLOSED"
	}
	return c, nil
}
func (s *Store) allowed(ctx context.Context, tx pgx.Tx, a identity.Actor, c Class, cmd Command) error {
	if !s.contextual {
		return allowed(ctx, tx, a, c, cmd.Action)
	}
	roles := []string{}
	rows, err := tx.Query(ctx, `SELECT r.role FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)`, c.ID, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			rows.Close()
			return err
		}
		roles = append(roles, role)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !access.Allows(a.Admin, roles, c.State, cmd.Action, cmd.Role) {
		return ErrForbidden
	}
	if !a.Admin && (cmd.Action == "add" || cmd.Action == "end" || cmd.Action == "suspend" || cmd.Action == "reactivate") {
		var staff bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id WHERE e.class_id=$1 AND e.user_id=$2 AND r.role='teacher' AND r.revoked_at IS NULL)`, c.ID, cmd.UserID).Scan(&staff); err != nil {
			return err
		}
		if staff {
			return ErrForbidden
		}
	}
	return nil
}
func ID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Store) fail(stage string) error {
	if s.failAfter != nil {
		return s.failAfter(stage)
	}
	return nil
}

func load(ctx context.Context, tx pgx.Tx, id string) (Class, error) {
	var c Class
	err := tx.QueryRow(ctx, "SELECT id,name,program_label,cohort_label,state,version FROM v3.classes WHERE id=$1 FOR UPDATE", id).Scan(&c.ID, &c.Name, &c.ProgramLabel, &c.CohortLabel, &c.State, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}
func allowed(ctx context.Context, tx pgx.Tx, a identity.Actor, c Class, action string) error {
	if a.Admin {
		return nil
	}
	if action != "read" && action != "edit" && action != "publish" {
		return ErrForbidden
	}
	rows, err := tx.Query(ctx, `SELECT r.role FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.revoked_at IS NULL FOR SHARE OF e,r`, c.ID, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	granted := false
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return err
		}
		if role == "teacher" || (action == "read" && role == "student" && c.State != "DRAFT") {
			granted = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !granted {
		return ErrForbidden
	}
	return nil
}
func (s *Store) Read(ctx context.Context, token, id string) (Class, error) {
	var result Class
	if !idPattern.MatchString(id) {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.actor(ctx, tx, token)
		if err != nil {
			return err
		}
		c, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = s.allowed(ctx, tx, a, c, Command{Action: "read"}); err != nil {
			return err
		}
		result = c
		return nil
	})
	return result, err
}
func validate(c Command) error {
	switch c.Action {
	case "create", "edit":
		if strings.TrimSpace(c.Name) == "" || len(c.Name) > 200 || len(c.ProgramLabel) > 200 || len(c.CohortLabel) > 200 {
			return ErrInvalid
		}
	case "add":
		if !idPattern.MatchString(c.UserID) || (c.Role != "teacher" && c.Role != "student") {
			return ErrInvalid
		}
	case "end", "suspend", "reactivate":
		if !idPattern.MatchString(c.UserID) {
			return ErrInvalid
		}
	case "publish", "archive", "close", "reopen":
	default:
		return ErrInvalid
	}
	if c.Action != "create" && !idPattern.MatchString(c.ClassID) {
		return ErrInvalid
	}
	if (c.Action == "edit" || c.Action == "publish" || c.Action == "archive" || c.Action == "close" || c.Action == "reopen") && c.Version < 1 {
		return ErrInvalid
	}
	return nil
}

// Execute serializes a Class command, checks current authority before replay,
// and persists domain/audit/outbox/idempotency together. Commit errors are not
// blindly retried: caller repeats the same key to reconcile uncertain outcomes.
func (s *Store) Execute(ctx context.Context, token, key, requestID string, cmd Command) (json.RawMessage, error) {
	if !keyPattern.MatchString(key) || !idPattern.MatchString(requestID) {
		return nil, ErrInvalid
	}
	if err := validate(cmd); err != nil {
		return nil, err
	}
	if !s.contextual && (cmd.Action == "close" || cmd.Action == "reopen" || cmd.Action == "suspend" || cmd.Action == "reactivate") {
		return nil, ErrInvalid
	}
	raw, _ := json.Marshal(cmd)
	hash := identity.TokenHash(string(raw))
	var response json.RawMessage
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.actor(ctx, tx, token)
		if err != nil {
			return err
		}
		var c Class
		if cmd.Action == "create" {
			if !actor.Admin {
				return ErrForbidden
			}
		} else {
			c, err = s.load(ctx, tx, cmd.ClassID)
			if err != nil {
				return err
			}
			if err = s.allowed(ctx, tx, actor, c, cmd); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+"/"+key); err != nil {
			return err
		}
		var previous string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&previous, &response)
		if err == nil {
			if previous != hash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if cmd.Action != "create" && c.State == "ARCHIVED" {
			return ErrConflict
		}
		if c.State == "CLOSED" && cmd.Action != "reopen" && cmd.Action != "archive" {
			return ErrConflict
		}
		if (cmd.Action == "edit" || cmd.Action == "publish" || cmd.Action == "archive" || cmd.Action == "close" || cmd.Action == "reopen") && c.Version != cmd.Version {
			return ErrConflict
		}
		var result any
		previousState := c.State
		transitionEnrollment := ""
		nextState := ""
		switch cmd.Action {
		case "close", "reopen":
			if (cmd.Action == "close" && c.State != "PUBLISHED") || (cmd.Action == "reopen" && c.State != "CLOSED") {
				return ErrConflict
			}
			if cmd.Action == "reopen" {
				var teacher bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id JOIN v3identity.links l ON l.user_id=e.user_id JOIN v3.users u ON u.id=e.user_id WHERE e.class_id=$1 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND l.active AND u.active AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, c.ID).Scan(&teacher); err != nil {
					return err
				}
				if !teacher {
					return ErrConflict
				}
			}
			q := `INSERT INTO v3.class_access(class_id,closed_at) VALUES($1,clock_timestamp()) ON CONFLICT(class_id) DO UPDATE SET closed_at=excluded.closed_at`
			c.State = "CLOSED"
			if cmd.Action == "reopen" {
				q = "UPDATE v3.class_access SET closed_at=NULL WHERE class_id=$1"
				c.State = "PUBLISHED"
			}
			if _, err = tx.Exec(ctx, q, c.ID); err != nil {
				return err
			}
			c.Version++
			if _, err = tx.Exec(ctx, "UPDATE v3.classes SET version=$2 WHERE id=$1", c.ID, c.Version); err != nil {
				return err
			}
			result = c
			nextState = c.State
		case "create":
			id, err := ID()
			if err != nil {
				return err
			}
			c = Class{id, strings.TrimSpace(cmd.Name), cmd.ProgramLabel, cmd.CohortLabel, "DRAFT", 1}
			_, err = tx.Exec(ctx, "INSERT INTO v3.classes(id,name,program_label,cohort_label) VALUES($1,$2,$3,$4)", c.ID, c.Name, c.ProgramLabel, c.CohortLabel)
			if err != nil {
				return err
			}
			result = c
		case "edit":
			c.Name = strings.TrimSpace(cmd.Name)
			if cmd.UpdateProgram {
				c.ProgramLabel = cmd.ProgramLabel
			}
			if cmd.UpdateCohort {
				c.CohortLabel = cmd.CohortLabel
			}
			c.Version++
			_, err = tx.Exec(ctx, "UPDATE v3.classes SET name=$2,program_label=$3,cohort_label=$4,version=$5 WHERE id=$1", c.ID, c.Name, c.ProgramLabel, c.CohortLabel, c.Version)
			if err != nil {
				return err
			}
			result = c
		case "publish", "archive":
			if cmd.Action == "publish" {
				if c.State != "DRAFT" {
					return ErrConflict
				}
				var teacher bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id JOIN v3.users u ON u.id=e.user_id WHERE e.class_id=$1 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND u.active)`, c.ID).Scan(&teacher)
				if err == nil && s.contextual {
					err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id JOIN v3identity.links l ON l.user_id=e.user_id JOIN v3.users u ON u.id=e.user_id WHERE e.class_id=$1 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND l.active AND u.active AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, c.ID).Scan(&teacher)
				}
				if err != nil {
					return err
				}
				if !teacher {
					return ErrConflict
				}
				c.State = "PUBLISHED"
			} else {
				c.State = "ARCHIVED"
			}
			c.Version++
			_, err = tx.Exec(ctx, "UPDATE v3.classes SET state=$2,version=$3 WHERE id=$1", c.ID, c.State, c.Version)
			if err != nil {
				return err
			}
			result = c
		case "add":
			var active bool
			if err = tx.QueryRow(ctx, "SELECT active FROM v3.users WHERE id=$1", cmd.UserID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalid
			} else if err != nil {
				return err
			}
			if !active {
				return ErrConflict
			}
			if s.contextual {
				if err = tx.QueryRow(ctx, "SELECT active FROM v3identity.links WHERE user_id=$1", cmd.UserID).Scan(&active); err != nil {
					return ErrInvalid
				}
				if !active {
					return ErrConflict
				}
			}
			var enrollment, status string
			err = tx.QueryRow(ctx, "SELECT id,status FROM v3.enrollments WHERE class_id=$1 AND user_id=$2 FOR UPDATE", c.ID, cmd.UserID).Scan(&enrollment, &status)
			fresh := errors.Is(err, pgx.ErrNoRows)
			if err != nil && !fresh {
				return err
			}
			if s.contextual && !fresh {
				var suspended bool
				if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.enrollment_access WHERE enrollment_id=$1 AND suspended_at IS NOT NULL)", enrollment).Scan(&suspended); err != nil {
					return err
				}
				if suspended {
					return ErrConflict
				}
			}
			if fresh {
				enrollment, err = ID()
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, "INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($1,$2,$3,'ACTIVE')", enrollment, c.ID, cmd.UserID)
				if err != nil {
					return err
				}
			}
			if fresh || status == "ENDED" {
				if !fresh {
					if _, err = tx.Exec(ctx, "UPDATE v3.enrollments SET status='ACTIVE' WHERE id=$1", enrollment); err != nil {
						return err
					}
				}
				episode, err := ID()
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, "INSERT INTO v3.episodes(id,enrollment_id) VALUES($1,$2)", episode, enrollment); err != nil {
					return err
				}
			}
			roleID, err := ID()
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($1,$2,$3,$4) ON CONFLICT(enrollment_id,role) WHERE revoked_at IS NULL DO NOTHING", roleID, enrollment, c.ID, cmd.Role)
			if err != nil {
				return err
			}
			result = map[string]string{"class_id": c.ID, "enrollment_id": enrollment, "user_id": cmd.UserID, "role": cmd.Role, "status": "ACTIVE"}
			transitionEnrollment = enrollment
			previousState = status
			if fresh {
				previousState = "ABSENT"
			}
			nextState = "ACTIVE"
		case "suspend", "reactivate":
			var status string
			var suspended bool
			err = tx.QueryRow(ctx, `SELECT e.id,e.status,EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) FROM v3.enrollments e WHERE e.class_id=$1 AND e.user_id=$2 FOR UPDATE`, c.ID, cmd.UserID).Scan(&transitionEnrollment, &status, &suspended)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if status != "ACTIVE" || (cmd.Action == "suspend" && suspended) || (cmd.Action == "reactivate" && !suspended) {
				return ErrConflict
			}
			if cmd.Action == "suspend" {
				if err = s.remainingTeacher(ctx, tx, c, cmd.UserID); err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `INSERT INTO v3.enrollment_access(enrollment_id,suspended_at) VALUES($1,clock_timestamp()) ON CONFLICT(enrollment_id) DO UPDATE SET suspended_at=excluded.suspended_at`, transitionEnrollment)
				previousState = "ACTIVE"
				nextState = "SUSPENDED"
			} else {
				_, err = tx.Exec(ctx, "UPDATE v3.enrollment_access SET suspended_at=NULL WHERE enrollment_id=$1", transitionEnrollment)
				previousState = "SUSPENDED"
				nextState = "ACTIVE"
			}
			if err != nil {
				return err
			}
			result = map[string]string{"enrollment_id": transitionEnrollment, "status": nextState}
		case "end":
			if s.contextual {
				if err = s.remainingTeacher(ctx, tx, c, cmd.UserID); err != nil {
					return err
				}
			}
			if !s.contextual && c.State == "PUBLISHED" {
				var remaining bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id JOIN v3.users u ON u.id=e.user_id WHERE e.class_id=$1 AND e.user_id<>$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND u.active)`, c.ID, cmd.UserID).Scan(&remaining); err != nil {
					return err
				}
				if !remaining {
					return ErrConflict
				}
			}
			var enrollment string
			err = tx.QueryRow(ctx, "SELECT id FROM v3.enrollments WHERE class_id=$1 AND user_id=$2 FOR UPDATE", c.ID, cmd.UserID).Scan(&enrollment)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if s.contextual {
				if err = tx.QueryRow(ctx, `SELECT CASE WHEN EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) THEN 'SUSPENDED' ELSE e.status END FROM v3.enrollments e WHERE e.id=$1`, enrollment).Scan(&previousState); err != nil {
					return err
				}
			}
			for _, q := range []string{"UPDATE v3.enrollments SET status='ENDED' WHERE id=$1", "UPDATE v3.episodes SET ended_at=now() WHERE enrollment_id=$1 AND ended_at IS NULL", "UPDATE v3.roles SET revoked_at=now() WHERE enrollment_id=$1 AND revoked_at IS NULL"} {
				if _, err = tx.Exec(ctx, q, enrollment); err != nil {
					return err
				}
			}
			result = map[string]string{"enrollment_id": enrollment, "status": "ENDED"}
			if s.contextual {
				if _, err = tx.Exec(ctx, "UPDATE v3.enrollment_access SET suspended_at=NULL WHERE enrollment_id=$1", enrollment); err != nil {
					return err
				}
			}
			transitionEnrollment = enrollment
			nextState = "ENDED"
		}
		if s.contextual && nextState != "" {
			id, e := ID()
			if e != nil {
				return e
			}
			var enrollment any
			if transitionEnrollment != "" {
				enrollment = transitionEnrollment
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.transition_events(id,actor_id,class_id,enrollment_id,action,previous_state,next_state) VALUES($1,$2,$3,$4,$5,$6,$7)", id, actor.ID, c.ID, enrollment, cmd.Action, previousState, nextState); err != nil {
				return err
			}
		}
		if err = s.fail("domain"); err != nil {
			return err
		}
		event, err := ID()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", event, actor.ID, c.ID, cmd.Action, requestID); err != nil {
			return err
		}
		if err = s.fail("audit"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", event); err != nil {
			return err
		}
		if err = s.fail("outbox"); err != nil {
			return err
		}
		response, err = json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3.commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb)", actor.ID, key, hash, string(response))
		return err
	})
	return response, err
}

func (s *Store) remainingTeacher(ctx context.Context, tx pgx.Tx, c Class, target string) error {
	if c.State != "PUBLISHED" {
		return nil
	}
	var isTeacher, remaining bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id WHERE e.class_id=$1 AND e.user_id=$2 AND r.role='teacher' AND r.revoked_at IS NULL),EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id JOIN v3identity.links l ON l.user_id=e.user_id JOIN v3.users u ON u.id=e.user_id WHERE e.class_id=$1 AND e.user_id<>$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL AND l.active AND u.active AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, c.ID, target).Scan(&isTeacher, &remaining)
	if err != nil {
		return err
	}
	if isTeacher && !remaining {
		return ErrConflict
	}
	return nil
}
