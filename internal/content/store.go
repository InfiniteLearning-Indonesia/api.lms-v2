package content

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"strings"
	"time"
	_ "time/tzdata"
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

type Section struct {
	ID        string    `json:"id"`
	ClassID   string    `json:"class_id"`
	Title     string    `json:"title"`
	Position  int       `json:"position"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Activity struct {
	ID           string    `json:"id"`
	ClassID      string    `json:"class_id"`
	SectionID    string    `json:"section_id"`
	ActivityType string    `json:"activity_type"`
	Title        string    `json:"title"`
	Position     int       `json:"position"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Revision struct {
	ID            string          `json:"id"`
	ClassID       string          `json:"class_id"`
	ActivityID    string          `json:"activity_id"`
	RevisionNo    int             `json:"revision_no"`
	State         string          `json:"state"`
	CanonicalBody string          `json:"canonical_body"`
	ContentFormat string          `json:"content_format"`
	Timezone      string          `json:"timezone"`
	AvailableFrom *time.Time      `json:"available_from,omitempty"`
	DueAt         *time.Time      `json:"due_at,omitempty"`
	CutoffAt      *time.Time      `json:"cutoff_at,omitempty"`
	Version       int64           `json:"version"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Assignment    *AssignmentRule `json:"assignment,omitempty"`
}

type AssignmentRule struct {
	RevisionID      string    `json:"revision_id"`
	ClassID         string    `json:"class_id"`
	ActivityID      string    `json:"activity_id"`
	ContractVersion int16     `json:"contract_version"`
	SubmissionMode  string    `json:"submission_mode"`
	MaxAttempts     int       `json:"max_attempts"`
	MaxFiles        int       `json:"max_files"`
	MaxFileBytes    int64     `json:"max_file_bytes"`
	LatePolicy      string    `json:"late_policy"`
	Version         int64     `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AssignmentRuleInput struct {
	SubmissionMode string `json:"submission_mode"`
	MaxAttempts    int    `json:"max_attempts"`
	MaxFiles       int    `json:"max_files"`
	MaxFileBytes   int64  `json:"max_file_bytes"`
	LatePolicy     string `json:"late_policy"`
}

type AssignmentContract struct {
	Revision Revision       `json:"revision"`
	Rule     AssignmentRule `json:"rule"`
}

type Snapshot struct {
	ClassID         string           `json:"class_id"`
	Sections        []Section        `json:"sections"`
	Activities      []Activity       `json:"activities"`
	Revisions       []Revision       `json:"revisions"`
	AssignmentRules []AssignmentRule `json:"assignment_rules"`
}

type Command struct {
	Action        string               `json:"action"`
	ClassID       string               `json:"class_id"`
	SectionID     string               `json:"section_id,omitempty"`
	ActivityID    string               `json:"activity_id,omitempty"`
	RevisionID    string               `json:"revision_id,omitempty"`
	ActivityType  string               `json:"activity_type,omitempty"`
	Title         string               `json:"title"`
	Position      int                  `json:"position"`
	Version       int64                `json:"version,omitempty"`
	RawBody       string               `json:"raw_body,omitempty"`
	Timezone      string               `json:"timezone,omitempty"`
	AvailableFrom *time.Time           `json:"available_from,omitempty"`
	DueAt         *time.Time           `json:"due_at,omitempty"`
	CutoffAt      *time.Time           `json:"cutoff_at,omitempty"`
	Assignment    *AssignmentRuleInput `json:"assignment,omitempty"`
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

func hash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func cleanTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 160 {
		return "", ErrInvalid
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", ErrInvalid
		}
	}
	return value, nil
}

func canonicalize(value string) (string, string, error) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	if !utf8.ValidString(value) || len(value) < 1 || len(value) > 65536 || strings.TrimSpace(value) == "" {
		return "", "", ErrInvalid
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return "", "", ErrInvalid
		}
	}
	canonical := strings.ReplaceAll(html.EscapeString(value), "\n", "<br>")
	return canonical, hash([]byte(value)), nil
}

func validSchedule(command Command) bool {
	if command.Timezone == "" || len(command.Timezone) > 64 {
		return false
	}
	if _, err := time.LoadLocation(command.Timezone); err != nil {
		return false
	}
	if command.AvailableFrom != nil && command.DueAt != nil && command.DueAt.Before(*command.AvailableFrom) {
		return false
	}
	if command.DueAt != nil && command.CutoffAt != nil && command.CutoffAt.Before(*command.DueAt) {
		return false
	}
	return command.AvailableFrom == nil || command.CutoffAt == nil || !command.CutoffAt.Before(*command.AvailableFrom)
}

func normalizeAssignment(value *AssignmentRuleInput) (*AssignmentRuleInput, error) {
	if value == nil {
		return nil, nil
	}
	normalized := *value
	normalized.SubmissionMode = strings.ToUpper(strings.TrimSpace(normalized.SubmissionMode))
	normalized.LatePolicy = strings.ToUpper(strings.TrimSpace(normalized.LatePolicy))
	if normalized.MaxAttempts < 1 || normalized.MaxAttempts > 100 {
		return nil, ErrInvalid
	}
	if normalized.LatePolicy != "REJECT_AFTER_CUTOFF" && normalized.LatePolicy != "ALLOW_MARKED_LATE" {
		return nil, ErrInvalid
	}
	switch normalized.SubmissionMode {
	case "TEXT":
		if normalized.MaxFiles != 0 || normalized.MaxFileBytes != 0 {
			return nil, ErrInvalid
		}
	case "FILES", "TEXT_AND_FILES":
		if normalized.MaxFiles < 1 || normalized.MaxFiles > 20 || normalized.MaxFileBytes < 1 || normalized.MaxFileBytes > 2<<20 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	return &normalized, nil
}

func normalize(command Command) (Command, error) {
	command.Action = strings.ToLower(strings.TrimSpace(command.Action))
	command.ActivityType = strings.ToUpper(strings.TrimSpace(command.ActivityType))
	command.Timezone = strings.TrimSpace(command.Timezone)
	if command.ClassID == "" {
		return command, ErrInvalid
	}
	switch command.Action {
	case "create_section", "update_section", "create_activity", "update_activity":
		var err error
		command.Title, err = cleanTitle(command.Title)
		if err != nil || command.Position < 1 || command.Position > 10000 || command.RevisionID != "" || command.RawBody != "" || command.Timezone != "" || command.AvailableFrom != nil || command.DueAt != nil || command.CutoffAt != nil || command.Assignment != nil {
			return command, ErrInvalid
		}
	}
	switch command.Action {
	case "create_section":
		if command.SectionID != "" || command.ActivityID != "" || command.Version != 0 || command.ActivityType != "" {
			return command, ErrInvalid
		}
	case "update_section":
		if command.SectionID == "" || command.ActivityID != "" || command.Version < 1 || command.ActivityType != "" {
			return command, ErrInvalid
		}
	case "create_activity":
		if command.SectionID == "" || command.ActivityID != "" || command.Version != 0 || !validType(command.ActivityType) {
			return command, ErrInvalid
		}
	case "update_activity":
		if command.SectionID != "" || command.ActivityID == "" || command.Version < 1 || command.ActivityType != "" {
			return command, ErrInvalid
		}
	case "create_revision", "update_revision":
		command.RawBody = strings.ReplaceAll(strings.ReplaceAll(command.RawBody, "\r\n", "\n"), "\r", "\n")
		if command.SectionID != "" || command.ActivityID == "" || !validSchedule(command) || command.Title != "" || command.Position != 0 || command.ActivityType != "" {
			return command, ErrInvalid
		}
		if _, _, err := canonicalize(command.RawBody); err != nil {
			return command, err
		}
		assignment, assignmentErr := normalizeAssignment(command.Assignment)
		if assignmentErr != nil {
			return command, assignmentErr
		}
		command.Assignment = assignment
		if command.Action == "create_revision" && (command.RevisionID != "" || command.Version != 0) {
			return command, ErrInvalid
		}
		if command.Action == "update_revision" && (command.RevisionID == "" || command.Version < 1) {
			return command, ErrInvalid
		}
	case "publish_revision", "withdraw_revision":
		if command.SectionID != "" || command.ActivityID == "" || command.RevisionID == "" || command.Version < 1 || command.Title != "" || command.Position != 0 || command.ActivityType != "" || command.RawBody != "" || command.Timezone != "" || command.AvailableFrom != nil || command.DueAt != nil || command.CutoffAt != nil || command.Assignment != nil {
			return command, ErrInvalid
		}
	default:
		return command, ErrInvalid
	}
	return command, nil
}

func validType(value string) bool {
	return value == "RESOURCE" || value == "ASSIGNMENT" || value == "QUIZ"
}

func (s *Store) context(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string, write bool) error {
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM v3.classes WHERE id=$1 FOR SHARE", classID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var closed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", classID).Scan(&closed); err != nil {
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
	 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)`, classID, actor.ID)
	if err != nil {
		return err
	}
	teacher, participant := false, false
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			rows.Close()
			return err
		}
		teacher = teacher || role == "teacher"
		participant = participant || role == "teacher" || role == "student"
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
	if !participant || (state == "DRAFT" && !teacher) {
		return ErrForbidden
	}
	return nil
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

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514") {
		return ErrConflict
	}
	return err
}

func scanRevision(row pgx.Row, revision *Revision) error {
	return row.Scan(&revision.ID, &revision.ClassID, &revision.ActivityID, &revision.RevisionNo, &revision.State,
		&revision.CanonicalBody, &revision.ContentFormat, &revision.Timezone, &revision.AvailableFrom,
		&revision.DueAt, &revision.CutoffAt, &revision.Version, &revision.CreatedAt, &revision.UpdatedAt)
}

const revisionFields = `id,class_id,activity_id,revision_no,state,canonical_body,content_format,timezone,available_from,due_at,cutoff_at,version,created_at,updated_at`

const assignmentRuleFields = `revision_id,class_id,activity_id,contract_version,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,version,created_at,updated_at`

func qualifyFields(alias, fields string) string {
	return alias + "." + strings.ReplaceAll(fields, ",", ","+alias+".")
}

func scanAssignmentRule(row pgx.Row, rule *AssignmentRule) error {
	return row.Scan(&rule.RevisionID, &rule.ClassID, &rule.ActivityID, &rule.ContractVersion,
		&rule.SubmissionMode, &rule.MaxAttempts, &rule.MaxFiles, &rule.MaxFileBytes,
		&rule.LatePolicy, &rule.Version, &rule.CreatedAt, &rule.UpdatedAt)
}

func assignmentContractsInstalled(ctx context.Context, tx pgx.Tx) (bool, error) {
	var installed bool
	err := tx.QueryRow(ctx, "SELECT to_regclass('v3.assignment_revision_rules') IS NOT NULL").Scan(&installed)
	return installed, err
}

func actorIsAuthor(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	var author bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
		WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, classID, actor.ID).Scan(&author)
	return author, err
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
	requestHash := hash(request)
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.context(ctx, tx, actor, command.ClassID, true); err != nil {
			return err
		}
		var priorHash string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.content_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&priorHash, &response)
		if err == nil {
			if priorHash != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var value any
		switch command.Action {
		case "create_section":
			var id string
			id, err = token(16)
			if err != nil {
				return err
			}
			var section Section
			err = tx.QueryRow(ctx, `INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($1,$2,$3,$4,$5)
			 RETURNING id,class_id,title,position,version,created_at,updated_at`, id, command.ClassID, command.Title, command.Position, actor.ID).Scan(&section.ID, &section.ClassID, &section.Title, &section.Position, &section.Version, &section.CreatedAt, &section.UpdatedAt)
			value = section
		case "update_section":
			var section Section
			err = tx.QueryRow(ctx, `UPDATE v3.content_sections SET title=$3,position=$4,version=version+1,updated_at=clock_timestamp()
			 WHERE id=$1 AND class_id=$2 AND version=$5 RETURNING id,class_id,title,position,version,created_at,updated_at`, command.SectionID, command.ClassID, command.Title, command.Position, command.Version).Scan(&section.ID, &section.ClassID, &section.Title, &section.Position, &section.Version, &section.CreatedAt, &section.UpdatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			value = section
		case "create_activity":
			var exists bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.content_sections WHERE id=$1 AND class_id=$2)", command.SectionID, command.ClassID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return ErrNotFound
			}
			var id string
			id, err = token(16)
			if err != nil {
				return err
			}
			var activity Activity
			err = tx.QueryRow(ctx, `INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)
			 RETURNING id,class_id,section_id,activity_type,title,position,version,created_at,updated_at`, id, command.ClassID, command.SectionID, command.ActivityType, command.Title, command.Position, actor.ID).Scan(&activity.ID, &activity.ClassID, &activity.SectionID, &activity.ActivityType, &activity.Title, &activity.Position, &activity.Version, &activity.CreatedAt, &activity.UpdatedAt)
			value = activity
		case "update_activity":
			var activity Activity
			err = tx.QueryRow(ctx, `UPDATE v3.content_activities SET title=$3,position=$4,version=version+1,updated_at=clock_timestamp()
			 WHERE id=$1 AND class_id=$2 AND version=$5 RETURNING id,class_id,section_id,activity_type,title,position,version,created_at,updated_at`, command.ActivityID, command.ClassID, command.Title, command.Position, command.Version).Scan(&activity.ID, &activity.ClassID, &activity.SectionID, &activity.ActivityType, &activity.Title, &activity.Position, &activity.Version, &activity.CreatedAt, &activity.UpdatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			value = activity
		case "create_revision":
			var activityType string
			if err = tx.QueryRow(ctx, "SELECT activity_type FROM v3.content_activities WHERE id=$1 AND class_id=$2 FOR UPDATE", command.ActivityID, command.ClassID).Scan(&activityType); errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
			if activityType != "ASSIGNMENT" && (command.DueAt != nil || command.CutoffAt != nil) {
				return ErrInvalid
			}
			contractsInstalled, contractErr := assignmentContractsInstalled(ctx, tx)
			if contractErr != nil {
				return contractErr
			}
			if (activityType == "ASSIGNMENT" && contractsInstalled) != (command.Assignment != nil) {
				return ErrInvalid
			}
			canonical, rawHash, canonicalErr := canonicalize(command.RawBody)
			if canonicalErr != nil {
				return canonicalErr
			}
			var next int
			if err = tx.QueryRow(ctx, "SELECT COALESCE(MAX(revision_no),0)+1 FROM v3.content_revisions WHERE activity_id=$1", command.ActivityID).Scan(&next); err != nil {
				return err
			}
			var id string
			if id, err = token(16); err != nil {
				return err
			}
			var revision Revision
			err = scanRevision(tx.QueryRow(ctx, `INSERT INTO v3.content_revisions(id,class_id,activity_id,revision_no,canonical_body,content_format,timezone,available_from,due_at,cutoff_at,created_by)
				VALUES($1,$2,$3,$4,$5,'SAFE_HTML',$6,$7,$8,$9,$10) RETURNING `+revisionFields,
				id, command.ClassID, command.ActivityID, next, canonical, command.Timezone, command.AvailableFrom, command.DueAt, command.CutoffAt, actor.ID), &revision)
			if err == nil {
				_, err = tx.Exec(ctx, "INSERT INTO v3.content_revision_sources(revision_id,raw_body,raw_sha256) VALUES($1,$2,$3)", id, command.RawBody, rawHash)
			}
			if err == nil && command.Assignment != nil {
				var rule AssignmentRule
				err = scanAssignmentRule(tx.QueryRow(ctx, `INSERT INTO v3.assignment_revision_rules(revision_id,class_id,activity_id,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,created_by)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+assignmentRuleFields,
					id, command.ClassID, command.ActivityID, command.Assignment.SubmissionMode, command.Assignment.MaxAttempts,
					command.Assignment.MaxFiles, command.Assignment.MaxFileBytes, command.Assignment.LatePolicy, actor.ID), &rule)
				if err == nil {
					revision.Assignment = &rule
				}
			}
			value = revision
		case "update_revision":
			var activityType string
			if err = tx.QueryRow(ctx, "SELECT activity_type FROM v3.content_activities WHERE id=$1 AND class_id=$2", command.ActivityID, command.ClassID).Scan(&activityType); errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
			if activityType != "ASSIGNMENT" && (command.DueAt != nil || command.CutoffAt != nil) {
				return ErrInvalid
			}
			contractsInstalled, contractErr := assignmentContractsInstalled(ctx, tx)
			if contractErr != nil {
				return contractErr
			}
			if (activityType == "ASSIGNMENT" && contractsInstalled) != (command.Assignment != nil) {
				return ErrInvalid
			}
			canonical, rawHash, canonicalErr := canonicalize(command.RawBody)
			if canonicalErr != nil {
				return canonicalErr
			}
			var revision Revision
			err = scanRevision(tx.QueryRow(ctx, `UPDATE v3.content_revisions SET canonical_body=$5,timezone=$6,available_from=$7,due_at=$8,cutoff_at=$9,version=version+1,updated_at=clock_timestamp()
				WHERE id=$1 AND class_id=$2 AND activity_id=$3 AND version=$4 AND state='DRAFT' RETURNING `+revisionFields,
				command.RevisionID, command.ClassID, command.ActivityID, command.Version, canonical, command.Timezone, command.AvailableFrom, command.DueAt, command.CutoffAt), &revision)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			if err == nil {
				result, updateErr := tx.Exec(ctx, "UPDATE v3.content_revision_sources SET raw_body=$2,raw_sha256=$3,updated_at=clock_timestamp() WHERE revision_id=$1", command.RevisionID, command.RawBody, rawHash)
				if updateErr != nil || result.RowsAffected() != 1 {
					if updateErr != nil {
						return updateErr
					}
					return ErrConflict
				}
			}
			if err == nil && command.Assignment != nil {
				var rule AssignmentRule
				err = scanAssignmentRule(tx.QueryRow(ctx, `UPDATE v3.assignment_revision_rules SET submission_mode=$4,max_attempts=$5,max_files=$6,max_file_bytes=$7,late_policy=$8,version=version+1,updated_at=clock_timestamp()
					WHERE revision_id=$1 AND class_id=$2 AND activity_id=$3 AND version=$9 RETURNING `+assignmentRuleFields,
					command.RevisionID, command.ClassID, command.ActivityID, command.Assignment.SubmissionMode, command.Assignment.MaxAttempts,
					command.Assignment.MaxFiles, command.Assignment.MaxFileBytes, command.Assignment.LatePolicy, command.Version), &rule)
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrConflict
				}
				if err == nil {
					revision.Assignment = &rule
				}
			}
			value = revision
		case "publish_revision", "withdraw_revision":
			expected, next := "DRAFT", "PUBLISHED"
			if command.Action == "withdraw_revision" {
				expected, next = "PUBLISHED", "WITHDRAWN"
			}
			var revision Revision
			err = scanRevision(tx.QueryRow(ctx, `UPDATE v3.content_revisions SET state=$5,version=version+1,updated_at=clock_timestamp()
				WHERE id=$1 AND class_id=$2 AND activity_id=$3 AND version=$4 AND state=$6 RETURNING `+revisionFields,
				command.RevisionID, command.ClassID, command.ActivityID, command.Version, next, expected), &revision)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			value = revision
		}
		if err != nil {
			return mapWriteError(err)
		}
		response, err = json.Marshal(value)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, actor.ID, command.ClassID, "content."+command.Action, requestID); err != nil {
			return err
		}
		// Return PostgreSQL's canonical JSONB representation on the first call
		// as well as replays, so one idempotency key has byte-stable output.
		err = tx.QueryRow(ctx, `INSERT INTO v3.content_commands(actor_id,key,request_hash,response)
			VALUES($1,$2,$3,$4::jsonb) RETURNING response`, actor.ID, key, requestHash, string(response)).Scan(&response)
		return err
	})
	return response, replayed, err
}

func (s *Store) Read(ctx context.Context, session, classID string) (Snapshot, error) {
	result := Snapshot{ClassID: classID, Sections: []Section{}, Activities: []Activity{}, Revisions: []Revision{}, AssignmentRules: []AssignmentRule{}}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.context(ctx, tx, actor, classID, false); err != nil {
			return err
		}
		author, err := actorIsAuthor(ctx, tx, actor, classID)
		if err != nil {
			return err
		}
		sections, err := tx.Query(ctx, `SELECT id,class_id,title,position,version,created_at,updated_at FROM v3.content_sections WHERE class_id=$1 ORDER BY position,id`, classID)
		if err != nil {
			return err
		}
		for sections.Next() {
			var section Section
			if err = sections.Scan(&section.ID, &section.ClassID, &section.Title, &section.Position, &section.Version, &section.CreatedAt, &section.UpdatedAt); err != nil {
				sections.Close()
				return err
			}
			result.Sections = append(result.Sections, section)
		}
		err = sections.Err()
		sections.Close()
		if err != nil {
			return err
		}
		activities, err := tx.Query(ctx, `SELECT id,class_id,section_id,activity_type,title,position,version,created_at,updated_at FROM v3.content_activities WHERE class_id=$1 ORDER BY section_id,position,id`, classID)
		if err != nil {
			return err
		}
		for activities.Next() {
			var activity Activity
			if err = activities.Scan(&activity.ID, &activity.ClassID, &activity.SectionID, &activity.ActivityType, &activity.Title, &activity.Position, &activity.Version, &activity.CreatedAt, &activity.UpdatedAt); err != nil {
				activities.Close()
				return err
			}
			result.Activities = append(result.Activities, activity)
		}
		err = activities.Err()
		activities.Close()
		if err != nil {
			return err
		}
		var revisionsInstalled bool
		if err = tx.QueryRow(ctx, "SELECT to_regclass('v3.content_revisions') IS NOT NULL").Scan(&revisionsInstalled); err != nil {
			return err
		}
		if !revisionsInstalled {
			return nil
		}
		query := `SELECT ` + revisionFields + ` FROM v3.content_revisions WHERE class_id=$1`
		if !author {
			query += " AND state='PUBLISHED' AND (available_from IS NULL OR available_from<=clock_timestamp())"
		}
		query += " ORDER BY activity_id,revision_no,id"
		revisions, err := tx.Query(ctx, query, classID)
		if err != nil {
			return err
		}
		for revisions.Next() {
			var revision Revision
			if err = scanRevision(revisions, &revision); err != nil {
				revisions.Close()
				return err
			}
			result.Revisions = append(result.Revisions, revision)
		}
		err = revisions.Err()
		revisions.Close()
		if err != nil {
			return err
		}
		contractsInstalled, err := assignmentContractsInstalled(ctx, tx)
		if err != nil || !contractsInstalled {
			return err
		}
		ruleQuery := `SELECT ` + qualifyFields("ar", assignmentRuleFields) + ` FROM v3.assignment_revision_rules ar JOIN v3.content_revisions cr ON cr.id=ar.revision_id AND cr.class_id=ar.class_id AND cr.activity_id=ar.activity_id WHERE ar.class_id=$1`
		if !author {
			ruleQuery += " AND cr.state='PUBLISHED' AND (cr.available_from IS NULL OR cr.available_from<=clock_timestamp())"
		}
		ruleQuery += " ORDER BY ar.activity_id,cr.revision_no,ar.revision_id"
		rules, err := tx.Query(ctx, ruleQuery, classID)
		if err != nil {
			return err
		}
		for rules.Next() {
			var rule AssignmentRule
			if err = scanAssignmentRule(rules, &rule); err != nil {
				rules.Close()
				return err
			}
			result.AssignmentRules = append(result.AssignmentRules, rule)
		}
		err = rules.Err()
		rules.Close()
		return err
	})
	return result, err
}

// ResolveAssignmentContract returns an exact immutable producer contract by
// revision ID. M08 may reference this ID; it must never resolve through a
// mutable "latest revision" pointer.
func (s *Store) ResolveAssignmentContract(ctx context.Context, session, classID, activityID, revisionID string) (AssignmentContract, error) {
	var result AssignmentContract
	if classID == "" || activityID == "" || revisionID == "" {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.context(ctx, tx, actor, classID, false); err != nil {
			return err
		}
		installed, err := assignmentContractsInstalled(ctx, tx)
		if err != nil {
			return err
		}
		if !installed {
			return ErrNotFound
		}
		author, err := actorIsAuthor(ctx, tx, actor, classID)
		if err != nil {
			return err
		}
		query := `SELECT ` + qualifyFields("cr", revisionFields) + `,` + qualifyFields("ar", assignmentRuleFields) + `
			FROM v3.content_revisions cr JOIN v3.assignment_revision_rules ar ON ar.revision_id=cr.id AND ar.class_id=cr.class_id AND ar.activity_id=cr.activity_id
			WHERE cr.id=$1 AND cr.class_id=$2 AND cr.activity_id=$3`
		if !author {
			query += " AND cr.state='PUBLISHED' AND (cr.available_from IS NULL OR cr.available_from<=clock_timestamp())"
		}
		row := tx.QueryRow(ctx, query, revisionID, classID, activityID)
		err = row.Scan(&result.Revision.ID, &result.Revision.ClassID, &result.Revision.ActivityID, &result.Revision.RevisionNo,
			&result.Revision.State, &result.Revision.CanonicalBody, &result.Revision.ContentFormat, &result.Revision.Timezone,
			&result.Revision.AvailableFrom, &result.Revision.DueAt, &result.Revision.CutoffAt, &result.Revision.Version,
			&result.Revision.CreatedAt, &result.Revision.UpdatedAt,
			&result.Rule.RevisionID, &result.Rule.ClassID, &result.Rule.ActivityID, &result.Rule.ContractVersion,
			&result.Rule.SubmissionMode, &result.Rule.MaxAttempts, &result.Rule.MaxFiles, &result.Rule.MaxFileBytes,
			&result.Rule.LatePolicy, &result.Rule.Version, &result.Rule.CreatedAt, &result.Rule.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err == nil {
			result.Revision.Assignment = &result.Rule
		}
		return err
	})
	return result, err
}
