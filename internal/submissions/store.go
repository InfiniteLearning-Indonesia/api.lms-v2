package submissions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
var ErrDeadline = errors.New("DEADLINE_PASSED")

type Aggregate struct {
	ID                   string    `json:"id"`
	ClassID              string    `json:"class_id"`
	ActivityID           string    `json:"activity_id"`
	AssignmentRevisionID string    `json:"assignment_revision_id"`
	EnrollmentID         string    `json:"enrollment_id"`
	EpisodeID            string    `json:"episode_id"`
	OwnerUserID          string    `json:"owner_user_id"`
	CurrentAttemptNo     int       `json:"current_attempt_no"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	Attempts             []Attempt `json:"attempts,omitempty"`
}

type Attempt struct {
	ID                   string     `json:"id"`
	AggregateID          string     `json:"aggregate_id"`
	ClassID              string     `json:"class_id"`
	ActivityID           string     `json:"activity_id"`
	AssignmentRevisionID string     `json:"assignment_revision_id"`
	AttemptNo            int        `json:"attempt_no"`
	State                string     `json:"state"`
	Version              int64      `json:"version"`
	LockedAt             *time.Time `json:"locked_at,omitempty"`
	ReturnedAt           *time.Time `json:"returned_at,omitempty"`
	ReturnedBy           *string    `json:"returned_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Revisions            []Revision `json:"revisions,omitempty"`
	Receipt              *Receipt   `json:"receipt,omitempty"`
}

type Receipt struct {
	ID             string     `json:"id"`
	ClassID        string     `json:"class_id"`
	AggregateID    string     `json:"aggregate_id"`
	AttemptID      string     `json:"attempt_id"`
	RevisionID     string     `json:"revision_id"`
	SubmittedBy    string     `json:"submitted_by"`
	SubmittedAt    time.Time  `json:"submitted_at"`
	SubmittedLocal string     `json:"submitted_local"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	CutoffAt       *time.Time `json:"cutoff_at,omitempty"`
	Timezone       string     `json:"timezone"`
	Late           bool       `json:"late"`
	LatePolicy     string     `json:"late_policy"`
}

type Revision struct {
	ID          string       `json:"id"`
	ClassID     string       `json:"class_id"`
	AggregateID string       `json:"aggregate_id"`
	AttemptID   string       `json:"attempt_id"`
	RevisionNo  int          `json:"revision_no"`
	Body        string       `json:"body"`
	BodySHA256  string       `json:"body_sha256"`
	CreatedAt   time.Time    `json:"created_at"`
	Files       []Attachment `json:"files"`
}

type Attachment struct {
	AssetID  string `json:"asset_id"`
	Position int    `json:"position"`
}

type Command struct {
	Action               string   `json:"action"`
	ClassID              string   `json:"class_id"`
	ActivityID           string   `json:"activity_id,omitempty"`
	AssignmentRevisionID string   `json:"assignment_revision_id,omitempty"`
	AggregateID          string   `json:"aggregate_id,omitempty"`
	Version              int64    `json:"version,omitempty"`
	AttemptVersion       int64    `json:"attempt_version,omitempty"`
	Body                 string   `json:"body,omitempty"`
	FileAssetIDs         []string `json:"file_asset_ids,omitempty"`
}

type Store struct {
	pool    *pgxpool.Pool
	resolve func(context.Context, pgx.Tx, string) (identity.Actor, error)
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, resolve: identity.ResolveBridge}
}

func token(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func normalize(command Command) (Command, error) {
	command.Action = strings.ToLower(strings.TrimSpace(command.Action))
	command.Body = strings.ReplaceAll(strings.ReplaceAll(command.Body, "\r\n", "\n"), "\r", "\n")
	if command.ClassID == "" || !utf8.ValidString(command.Body) || len(command.Body) > 65536 || len(command.FileAssetIDs) > 20 {
		return command, ErrInvalid
	}
	for _, character := range command.Body {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return command, ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, assetID := range command.FileAssetIDs {
		if assetID == "" || seen[assetID] {
			return command, ErrInvalid
		}
		seen[assetID] = true
	}
	switch command.Action {
	case "create":
		if command.ActivityID == "" || command.AssignmentRevisionID == "" || command.AggregateID != "" || command.Version != 0 || command.AttemptVersion != 0 || command.Body != "" || len(command.FileAssetIDs) != 0 {
			return command, ErrInvalid
		}
	case "append_revision":
		if command.AggregateID == "" || command.ActivityID != "" || command.AssignmentRevisionID != "" || command.Version < 1 || command.AttemptVersion != 0 {
			return command, ErrInvalid
		}
	case "lock", "return":
		if command.AggregateID == "" || command.AttemptVersion < 1 || command.ActivityID != "" || command.AssignmentRevisionID != "" || command.Version != 0 || command.Body != "" || len(command.FileAssetIDs) != 0 {
			return command, ErrInvalid
		}
	case "resubmit":
		if command.AggregateID == "" || command.Version < 1 || command.AttemptVersion < 1 || command.ActivityID != "" || command.AssignmentRevisionID != "" || command.Body != "" || len(command.FileAssetIDs) != 0 {
			return command, ErrInvalid
		}
	case "handin":
		if command.AggregateID == "" || command.Version < 1 || command.AttemptVersion < 1 || command.ActivityID != "" || command.AssignmentRevisionID != "" || command.Body != "" || len(command.FileAssetIDs) != 0 {
			return command, ErrInvalid
		}
	default:
		return command, ErrInvalid
	}
	return command, nil
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Message == "SUBMISSION_CUTOFF_PASSED" {
		return ErrDeadline
	}
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514") {
		return ErrConflict
	}
	return err
}

func classTeacher(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	var teacher bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
		WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, classID, actor.ID).Scan(&teacher)
	return teacher, err
}

func activeStudentContext(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (string, string, error) {
	var enrollmentID, episodeID string
	err := tx.QueryRow(ctx, `SELECT e.id,ep.id FROM v3.classes c JOIN v3.enrollments e ON e.class_id=c.id
		JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id JOIN v3.episodes ep ON ep.enrollment_id=e.id
		WHERE c.id=$1 AND c.state='PUBLISHED' AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='student' AND r.revoked_at IS NULL AND ep.ended_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM v3.class_access ca WHERE ca.class_id=c.id AND ca.closed_at IS NOT NULL)
		AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access ea WHERE ea.enrollment_id=e.id AND ea.suspended_at IS NOT NULL)`, classID, actor.ID).Scan(&enrollmentID, &episodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrForbidden
	}
	return enrollmentID, episodeID, err
}

func audit(ctx context.Context, tx pgx.Tx, actor, classID, action, requestID string) error {
	id, err := token(16)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", id, actor, classID, action, requestID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", id)
	return err
}

const aggregateFields = `id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,current_attempt_no,version,created_at,updated_at`

func scanAggregate(row pgx.Row, value *Aggregate) error {
	return row.Scan(&value.ID, &value.ClassID, &value.ActivityID, &value.AssignmentRevisionID, &value.EnrollmentID, &value.EpisodeID, &value.OwnerUserID, &value.CurrentAttemptNo, &value.Version, &value.CreatedAt, &value.UpdatedAt)
}

const receiptFields = `id,class_id,aggregate_id,attempt_id,revision_id,submitted_by,submitted_at,due_at,cutoff_at,timezone,late,late_policy`

func scanReceipt(row pgx.Row, value *Receipt) error {
	if err := row.Scan(&value.ID, &value.ClassID, &value.AggregateID, &value.AttemptID, &value.RevisionID, &value.SubmittedBy, &value.SubmittedAt, &value.DueAt, &value.CutoffAt, &value.Timezone, &value.Late, &value.LatePolicy); err != nil {
		return err
	}
	zone, err := time.LoadLocation(value.Timezone)
	if err != nil {
		return fmt.Errorf("invalid stored receipt timezone: %w", err)
	}
	value.SubmittedLocal = value.SubmittedAt.In(zone).Format(time.RFC3339Nano)
	return nil
}

func receiptsInstalled(ctx context.Context, tx pgx.Tx) (bool, error) {
	var installed bool
	err := tx.QueryRow(ctx, "SELECT to_regclass('v3.submission_receipts') IS NOT NULL").Scan(&installed)
	return installed, err
}

func (s *Store) Execute(ctx context.Context, session, key, requestID string, command Command) (json.RawMessage, bool, error) {
	if len(key) < 8 || len(key) > 80 || requestID == "" {
		return nil, false, ErrInvalid
	}
	for _, character := range key {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return nil, false, ErrInvalid
		}
	}
	command, err := normalize(command)
	if err != nil {
		return nil, false, err
	}
	request, _ := json.Marshal(command)
	requestHash := digest(request)
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return fmt.Errorf("submission identity resolution: %w", err)
		}
		// Serialize retries for the same actor/key before inspecting the durable
		// command result. A different key remains a distinct command.
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":"+key); err != nil {
			return fmt.Errorf("submission retry lock: %w", err)
		}
		var priorHash string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.submission_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&priorHash, &response)
		if err == nil {
			if priorHash != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("submission idempotency lookup: %w", err)
		}
		// ErrNoRows means this is a new command, not a transaction failure.
		// Clear it before action-local declarations can shadow this variable.
		err = nil
		var value any
		switch command.Action {
		case "create":
			if actor.Admin {
				return ErrForbidden
			}
			enrollmentID, episodeID, err := activeStudentContext(ctx, tx, actor, command.ClassID)
			if err != nil {
				return fmt.Errorf("submission active participation: %w", err)
			}
			var available bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.content_revisions cr JOIN v3.assignment_revision_rules ar ON ar.revision_id=cr.id AND ar.class_id=cr.class_id AND ar.activity_id=cr.activity_id
				WHERE cr.id=$1 AND cr.class_id=$2 AND cr.activity_id=$3 AND cr.state='PUBLISHED' AND (cr.available_from IS NULL OR cr.available_from<=clock_timestamp()))`, command.AssignmentRevisionID, command.ClassID, command.ActivityID).Scan(&available)
			if err != nil || !available {
				if err == nil {
					return ErrNotFound
				}
				return fmt.Errorf("submission contract resolution: %w", err)
			}
			aggregateID, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			attemptID, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			var aggregate Aggregate
			err = scanAggregate(tx.QueryRow(ctx, `INSERT INTO v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,created_by)
				VALUES($1,$2,$3,$4,$5,$6,$7,$7) RETURNING `+aggregateFields, aggregateID, command.ClassID, command.ActivityID, command.AssignmentRevisionID, enrollmentID, episodeID, actor.ID), &aggregate)
			if err != nil {
				return fmt.Errorf("submission aggregate insert: %w", err)
			}
			var attempt Attempt
			err = tx.QueryRow(ctx, `INSERT INTO v3.submission_attempts(id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,created_by)
				VALUES($1,$2,$3,$4,$5,1,$6) RETURNING id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,created_at,updated_at`, attemptID, aggregate.ID, aggregate.ClassID, aggregate.ActivityID, aggregate.AssignmentRevisionID, actor.ID).Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.CreatedAt, &attempt.UpdatedAt)
			if err != nil {
				return fmt.Errorf("submission attempt insert: %w", err)
			}
			aggregate.Attempts = []Attempt{attempt}
			value = aggregate
		case "append_revision":
			var aggregate Aggregate
			err = scanAggregate(tx.QueryRow(ctx, "SELECT "+aggregateFields+" FROM v3.submission_aggregates WHERE id=$1 AND class_id=$2 FOR UPDATE", command.AggregateID, command.ClassID), &aggregate)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if aggregate.OwnerUserID != actor.ID || aggregate.Version != command.Version {
				return ErrConflict
			}
			if _, _, err = activeStudentContext(ctx, tx, actor, command.ClassID); err != nil {
				return err
			}
			var attempt Attempt
			err = tx.QueryRow(ctx, `SELECT id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,locked_at,returned_at,returned_by,created_at,updated_at
				FROM v3.submission_attempts WHERE aggregate_id=$1 AND attempt_no=$2 FOR UPDATE`, aggregate.ID, aggregate.CurrentAttemptNo).Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.LockedAt, &attempt.ReturnedAt, &attempt.ReturnedBy, &attempt.CreatedAt, &attempt.UpdatedAt)
			if err != nil {
				return err
			}
			if attempt.State != "DRAFT" {
				return ErrConflict
			}
			var mode string
			var maxFiles int
			var maxFileBytes int64
			err = tx.QueryRow(ctx, "SELECT submission_mode,max_files,max_file_bytes FROM v3.assignment_revision_rules WHERE revision_id=$1 AND class_id=$2 AND activity_id=$3", aggregate.AssignmentRevisionID, aggregate.ClassID, aggregate.ActivityID).Scan(&mode, &maxFiles, &maxFileBytes)
			if err != nil {
				return err
			}
			if (mode == "TEXT" && (strings.TrimSpace(command.Body) == "" || len(command.FileAssetIDs) > 0)) || (mode == "FILES" && (command.Body != "" || len(command.FileAssetIDs) < 1)) || (mode == "TEXT_AND_FILES" && strings.TrimSpace(command.Body) == "" && len(command.FileAssetIDs) < 1) || len(command.FileAssetIDs) > maxFiles {
				return ErrInvalid
			}
			for _, assetID := range command.FileAssetIDs {
				var valid bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.file_assets WHERE id=$1 AND class_id=$2 AND owner_user_id=$3 AND purpose='EVIDENCE' AND state='AVAILABLE' AND scan_status='CLEAN' AND byte_size<=$4)`, assetID, aggregate.ClassID, actor.ID, maxFileBytes).Scan(&valid); err != nil {
					return err
				}
				if !valid {
					return ErrForbidden
				}
			}
			var revisionNo int
			if err = tx.QueryRow(ctx, "SELECT COALESCE(max(revision_no),0)+1 FROM v3.submission_revisions WHERE attempt_id=$1", attempt.ID).Scan(&revisionNo); err != nil {
				return err
			}
			revisionID, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			var revision Revision
			err = tx.QueryRow(ctx, `INSERT INTO v3.submission_revisions(id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_by)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_at`, revisionID, aggregate.ClassID, aggregate.ID, attempt.ID, revisionNo, command.Body, digest([]byte(command.Body)), actor.ID).Scan(&revision.ID, &revision.ClassID, &revision.AggregateID, &revision.AttemptID, &revision.RevisionNo, &revision.Body, &revision.BodySHA256, &revision.CreatedAt)
			if err != nil {
				return err
			}
			revision.Files = []Attachment{}
			for index, assetID := range command.FileAssetIDs {
				if _, err = tx.Exec(ctx, "INSERT INTO v3.submission_revision_files(revision_id,class_id,file_asset_id,position) VALUES($1,$2,$3,$4)", revision.ID, aggregate.ClassID, assetID, index+1); err != nil {
					return err
				}
				revision.Files = append(revision.Files, Attachment{AssetID: assetID, Position: index + 1})
			}
			value = revision
		case "lock", "return":
			var owner string
			var current int
			if err = tx.QueryRow(ctx, "SELECT owner_user_id,current_attempt_no FROM v3.submission_aggregates WHERE id=$1 AND class_id=$2", command.AggregateID, command.ClassID).Scan(&owner, &current); errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			next, expected := "LOCKED", "DRAFT"
			if command.Action == "lock" {
				if owner != actor.ID {
					return ErrForbidden
				}
				if _, _, err = activeStudentContext(ctx, tx, actor, command.ClassID); err != nil {
					return err
				}
			} else {
				next, expected = "RETURNED", "LOCKED"
				teacher, roleErr := classTeacher(ctx, tx, actor, command.ClassID)
				if roleErr != nil {
					return roleErr
				}
				if !teacher {
					return ErrForbidden
				}
				installed, installErr := receiptsInstalled(ctx, tx)
				if installErr != nil {
					return installErr
				}
				if installed {
					var submitted bool
					if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.submission_receipts r JOIN v3.submission_attempts a ON a.id=r.attempt_id WHERE a.aggregate_id=$1 AND a.attempt_no=$2)`, command.AggregateID, current).Scan(&submitted); err != nil {
						return err
					}
					if !submitted {
						return ErrConflict
					}
				}
			}
			query := `UPDATE v3.submission_attempts SET state=$4,version=version+1,updated_at=clock_timestamp(),locked_at=CASE WHEN $4='LOCKED' THEN clock_timestamp() ELSE locked_at END,returned_at=CASE WHEN $4='RETURNED' THEN clock_timestamp() ELSE NULL END,returned_by=CASE WHEN $4='RETURNED' THEN $5 ELSE NULL END WHERE aggregate_id=$1 AND attempt_no=$2 AND version=$3 AND state=$6 RETURNING id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,locked_at,returned_at,returned_by,created_at,updated_at`
			var attempt Attempt
			err = tx.QueryRow(ctx, query, command.AggregateID, current, command.AttemptVersion, next, actor.ID, expected).Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.LockedAt, &attempt.ReturnedAt, &attempt.ReturnedBy, &attempt.CreatedAt, &attempt.UpdatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			value = attempt
		case "resubmit":
			if actor.Admin {
				return ErrForbidden
			}
			var aggregate Aggregate
			err = scanAggregate(tx.QueryRow(ctx, "SELECT "+aggregateFields+" FROM v3.submission_aggregates WHERE id=$1 AND class_id=$2 FOR UPDATE", command.AggregateID, command.ClassID), &aggregate)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if aggregate.OwnerUserID != actor.ID || aggregate.Version != command.Version {
				return ErrConflict
			}
			if _, _, err = activeStudentContext(ctx, tx, actor, command.ClassID); err != nil {
				return err
			}
			var state string
			var attemptVersion int64
			if err = tx.QueryRow(ctx, "SELECT state,version FROM v3.submission_attempts WHERE aggregate_id=$1 AND attempt_no=$2 FOR UPDATE", aggregate.ID, aggregate.CurrentAttemptNo).Scan(&state, &attemptVersion); err != nil {
				return err
			}
			if state != "RETURNED" || attemptVersion != command.AttemptVersion {
				return ErrConflict
			}
			var maxAttempts int
			if err = tx.QueryRow(ctx, "SELECT max_attempts FROM v3.assignment_revision_rules WHERE revision_id=$1", aggregate.AssignmentRevisionID).Scan(&maxAttempts); err != nil {
				return err
			}
			if aggregate.CurrentAttemptNo >= maxAttempts {
				return ErrConflict
			}
			newNo := aggregate.CurrentAttemptNo + 1
			attemptID, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			var attempt Attempt
			err = tx.QueryRow(ctx, `INSERT INTO v3.submission_attempts(id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,created_at,updated_at`, attemptID, aggregate.ID, aggregate.ClassID, aggregate.ActivityID, aggregate.AssignmentRevisionID, newNo, actor.ID).Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.CreatedAt, &attempt.UpdatedAt)
			if err != nil {
				return err
			}
			if err = scanAggregate(tx.QueryRow(ctx, `UPDATE v3.submission_aggregates SET current_attempt_no=$3,version=version+1,updated_at=clock_timestamp() WHERE id=$1 AND class_id=$2 AND version=$4 RETURNING `+aggregateFields, aggregate.ID, aggregate.ClassID, newNo, command.Version), &aggregate); err != nil {
				return err
			}
			aggregate.Attempts = []Attempt{attempt}
			value = aggregate
		case "handin":
			var aggregate Aggregate
			err = scanAggregate(tx.QueryRow(ctx, "SELECT "+aggregateFields+" FROM v3.submission_aggregates WHERE id=$1 AND class_id=$2 FOR UPDATE", command.AggregateID, command.ClassID), &aggregate)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if aggregate.OwnerUserID != actor.ID {
				return ErrForbidden
			}
			if aggregate.Version != command.Version {
				return ErrConflict
			}
			if _, _, err = activeStudentContext(ctx, tx, actor, command.ClassID); err != nil {
				return err
			}
			var attempt Attempt
			err = tx.QueryRow(ctx, `SELECT id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,locked_at,returned_at,returned_by,created_at,updated_at FROM v3.submission_attempts WHERE aggregate_id=$1 AND attempt_no=$2 FOR UPDATE`, aggregate.ID, aggregate.CurrentAttemptNo).Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.LockedAt, &attempt.ReturnedAt, &attempt.ReturnedBy, &attempt.CreatedAt, &attempt.UpdatedAt)
			if err != nil {
				return err
			}
			if attempt.Version != command.AttemptVersion || (attempt.State != "DRAFT" && attempt.State != "LOCKED") {
				return ErrConflict
			}
			var revisionID string
			if err = tx.QueryRow(ctx, "SELECT id FROM v3.submission_revisions WHERE attempt_id=$1 ORDER BY revision_no DESC LIMIT 1", attempt.ID).Scan(&revisionID); errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			if err != nil {
				return err
			}
			if attempt.State == "DRAFT" {
				_, err = tx.Exec(ctx, `UPDATE v3.submission_attempts SET state='LOCKED',version=version+1,updated_at=clock_timestamp(),locked_at=clock_timestamp() WHERE id=$1 AND state='DRAFT' AND version=$2`, attempt.ID, attempt.Version)
				if err != nil {
					return err
				}
			}
			receiptID, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			var receipt Receipt
			err = scanReceipt(tx.QueryRow(ctx, `INSERT INTO v3.submission_receipts(id,class_id,aggregate_id,attempt_id,revision_id,submitted_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+receiptFields, receiptID, aggregate.ClassID, aggregate.ID, attempt.ID, revisionID, actor.ID), &receipt)
			if err != nil {
				return err
			}
			value = receipt
		}
		if err != nil {
			return mapWriteError(err)
		}
		response, err = json.Marshal(value)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, actor.ID, command.ClassID, "submission."+command.Action, requestID); err != nil {
			return fmt.Errorf("submission audit: %w", err)
		}
		err = tx.QueryRow(ctx, `INSERT INTO v3.submission_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb) RETURNING response`, actor.ID, key, requestHash, string(response)).Scan(&response)
		if err != nil {
			return fmt.Errorf("submission command insert: %w", err)
		}
		return err
	})
	if err != nil {
		return response, replayed, fmt.Errorf("submission transaction: %w", mapWriteError(err))
	}
	return response, replayed, nil
}

func (s *Store) Read(ctx context.Context, session, classID, aggregateID string) (Aggregate, error) {
	var result Aggregate
	if classID == "" || aggregateID == "" {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = scanAggregate(tx.QueryRow(ctx, "SELECT "+aggregateFields+" FROM v3.submission_aggregates WHERE id=$1 AND class_id=$2", aggregateID, classID), &result); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		teacher, err := classTeacher(ctx, tx, actor, classID)
		if err != nil {
			return err
		}
		if actor.ID != result.OwnerUserID && !teacher {
			return ErrForbidden
		}
		result.Attempts = []Attempt{}
		receiptReady, err := receiptsInstalled(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,state,version,locked_at,returned_at,returned_by,created_at,updated_at FROM v3.submission_attempts WHERE aggregate_id=$1 ORDER BY attempt_no`, result.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var attempt Attempt
			if err = rows.Scan(&attempt.ID, &attempt.AggregateID, &attempt.ClassID, &attempt.ActivityID, &attempt.AssignmentRevisionID, &attempt.AttemptNo, &attempt.State, &attempt.Version, &attempt.LockedAt, &attempt.ReturnedAt, &attempt.ReturnedBy, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
				rows.Close()
				return err
			}
			attempt.Revisions = []Revision{}
			result.Attempts = append(result.Attempts, attempt)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for i := range result.Attempts {
			if receiptReady {
				var receipt Receipt
				err = scanReceipt(tx.QueryRow(ctx, "SELECT "+receiptFields+" FROM v3.submission_receipts WHERE attempt_id=$1", result.Attempts[i].ID), &receipt)
				if err == nil {
					result.Attempts[i].Receipt = &receipt
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			revisions, queryErr := tx.Query(ctx, `SELECT id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_at FROM v3.submission_revisions WHERE attempt_id=$1 ORDER BY revision_no`, result.Attempts[i].ID)
			if queryErr != nil {
				return queryErr
			}
			for revisions.Next() {
				var revision Revision
				if err = revisions.Scan(&revision.ID, &revision.ClassID, &revision.AggregateID, &revision.AttemptID, &revision.RevisionNo, &revision.Body, &revision.BodySHA256, &revision.CreatedAt); err != nil {
					revisions.Close()
					return err
				}
				revision.Files = []Attachment{}
				result.Attempts[i].Revisions = append(result.Attempts[i].Revisions, revision)
			}
			if err = revisions.Err(); err != nil {
				revisions.Close()
				return err
			}
			revisions.Close()
			for j := range result.Attempts[i].Revisions {
				files, fileErr := tx.Query(ctx, "SELECT file_asset_id,position FROM v3.submission_revision_files WHERE revision_id=$1 ORDER BY position", result.Attempts[i].Revisions[j].ID)
				if fileErr != nil {
					return fileErr
				}
				for files.Next() {
					var attachment Attachment
					if err = files.Scan(&attachment.AssetID, &attachment.Position); err != nil {
						files.Close()
						return err
					}
					result.Attempts[i].Revisions[j].Files = append(result.Attempts[i].Revisions[j].Files, attachment)
				}
				if err = files.Err(); err != nil {
					files.Close()
					return err
				}
				files.Close()
			}
		}
		return nil
	})
	return result, err
}
