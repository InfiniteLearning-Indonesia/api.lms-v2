package completion

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

type RuleCommand struct {
	ClassID           string `json:"class_id"`
	ActivityID        string `json:"activity_id"`
	ContentRevisionID string `json:"content_revision_id"`
	ExpectedVersion   int64  `json:"expected_version"`
	Requirement       string `json:"requirement"`
	EvidenceKind      string `json:"evidence_kind"`
}

type Rule struct {
	ID                string    `json:"id"`
	ClassID           string    `json:"class_id"`
	ActivityID        string    `json:"activity_id"`
	ContentRevisionID string    `json:"content_revision_id"`
	Version           int64     `json:"version"`
	Requirement       string    `json:"requirement"`
	EvidenceKind      string    `json:"evidence_kind"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
}

type EvaluationCommand struct {
	ClassID           string `json:"class_id"`
	ActivityID        string `json:"activity_id"`
	ContentRevisionID string `json:"content_revision_id"`
	RuleVersionID     string `json:"rule_version_id"`
	Acknowledge       bool   `json:"acknowledge"`
}

type Outcome struct {
	ID                string    `json:"id"`
	ClassID           string    `json:"class_id"`
	ActivityID        string    `json:"activity_id"`
	ContentRevisionID string    `json:"content_revision_id"`
	RuleVersionID     string    `json:"rule_version_id"`
	EnrollmentID      string    `json:"enrollment_id"`
	EpisodeID         string    `json:"episode_id"`
	LearnerUserID     string    `json:"learner_user_id"`
	EvidenceID        *string   `json:"evidence_id,omitempty"`
	State             string    `json:"state"`
	Reason            string    `json:"reason"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type Store struct {
	pool    *pgxpool.Pool
	resolve func(context.Context, pgx.Tx, string) (identity.Actor, error)
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, resolve: identity.ResolveBridge} }

func token(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func digest(value []byte) string {
	h := sha256.Sum256(value)
	return hex.EncodeToString(h[:])
}

func validIdempotency(key, requestID string) bool {
	if len(key) < 8 || len(key) > 80 || requestID == "" {
		return false
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23505" || pgErr.Code == "23514") {
		return fmt.Errorf("%w: %s", ErrConflict, pgErr.Message)
	}
	return err
}

func teacher(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.classes c JOIN v3.enrollments e ON e.class_id=c.id
	 JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
	 WHERE c.id=$1 AND c.state='PUBLISHED' AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
	 AND NOT EXISTS(SELECT 1 FROM v3.class_access x WHERE x.class_id=c.id AND x.closed_at IS NOT NULL)
	 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, classID, actor.ID).Scan(&allowed)
	return allowed, err
}

func learnerEpisode(ctx context.Context, tx pgx.Tx, actorID, classID string) (string, string, error) {
	var enrollmentID, episodeID string
	err := tx.QueryRow(ctx, `SELECT e.id,ep.id FROM v3.classes c JOIN v3.enrollments e ON e.class_id=c.id
	 JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id AND r.role='student' AND r.revoked_at IS NULL
	 JOIN v3.episodes ep ON ep.enrollment_id=e.id AND ep.ended_at IS NULL
	 WHERE c.id=$1 AND c.state='PUBLISHED' AND e.user_id=$2 AND e.status='ACTIVE'
	 AND NOT EXISTS(SELECT 1 FROM v3.class_access x WHERE x.class_id=c.id AND x.closed_at IS NOT NULL)
	 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)
	 ORDER BY ep.opened_at DESC,ep.id DESC LIMIT 1`, classID, actorID).Scan(&enrollmentID, &episodeID)
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

func normalizeRule(command RuleCommand) (RuleCommand, error) {
	command.Requirement = strings.ToUpper(strings.TrimSpace(command.Requirement))
	command.EvidenceKind = strings.ToUpper(strings.TrimSpace(command.EvidenceKind))
	if command.ClassID == "" || command.ActivityID == "" || command.ContentRevisionID == "" || command.ExpectedVersion < 0 || command.Requirement != "EVIDENCE_PRESENT" || command.EvidenceKind != "ACTIVITY_ACKNOWLEDGED" {
		return command, ErrInvalid
	}
	return command, nil
}

func normalizeEvaluation(command EvaluationCommand) (EvaluationCommand, error) {
	if command.ClassID == "" || command.ActivityID == "" || command.ContentRevisionID == "" || command.RuleVersionID == "" {
		return command, ErrInvalid
	}
	return command, nil
}

func commandReplay(ctx context.Context, tx pgx.Tx, actorID, key, requestHash, kind string) (json.RawMessage, bool, error) {
	var priorHash, priorKind string
	var response json.RawMessage
	err := tx.QueryRow(ctx, "SELECT request_hash,kind,response FROM v3.completion_commands WHERE actor_id=$1 AND key=$2", actorID, key).Scan(&priorHash, &priorKind, &response)
	if err == nil {
		if priorHash != requestHash || priorKind != kind {
			return nil, false, ErrConflict
		}
		return response, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	return nil, false, nil
}

func saveCommand(ctx context.Context, tx pgx.Tx, actorID, key, kind, requestHash string, response json.RawMessage) error {
	return tx.QueryRow(ctx, `INSERT INTO v3.completion_commands(actor_id,key,kind,request_hash,response)
	 VALUES($1,$2,$3,$4,$5::jsonb) RETURNING response`, actorID, key, kind, requestHash, string(response)).Scan(&response)
}

func (s *Store) CreateRule(ctx context.Context, session, key, requestID string, command RuleCommand) (Rule, bool, error) {
	var result Rule
	if !validIdempotency(key, requestID) {
		return result, false, ErrInvalid
	}
	command, err := normalizeRule(command)
	if err != nil {
		return result, false, err
	}
	request, _ := json.Marshal(command)
	requestHash := digest(request)
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, command.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "completion-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := commandReplay(ctx, tx, actor.ID, key, requestHash, "CREATE_RULE")
		if e != nil {
			return e
		}
		if found {
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "completion-rule:"+command.ActivityID); e != nil {
			return e
		}
		var current int64
		if e = tx.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM v3.completion_rule_versions WHERE activity_id=$1", command.ActivityID).Scan(&current); e != nil {
			return e
		}
		if current != command.ExpectedVersion {
			return ErrConflict
		}
		id, e := token(16)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO v3.completion_rule_versions(id,class_id,activity_id,content_revision_id,version,requirement,evidence_kind,created_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,class_id,activity_id,content_revision_id,version,requirement,evidence_kind,created_by,created_at`,
			id, command.ClassID, command.ActivityID, command.ContentRevisionID, current+1, command.Requirement, command.EvidenceKind, actor.ID).
			Scan(&result.ID, &result.ClassID, &result.ActivityID, &result.ContentRevisionID, &result.Version, &result.Requirement, &result.EvidenceKind, &result.CreatedBy, &result.CreatedAt)
		if e != nil {
			return mapWriteError(e)
		}
		response, _ := json.Marshal(result)
		if e = audit(ctx, tx, actor.ID, command.ClassID, "completion.rule.create", requestID); e != nil {
			return e
		}
		return saveCommand(ctx, tx, actor.ID, key, "CREATE_RULE", requestHash, response)
	})
	return result, replayed, mapWriteError(err)
}

func (s *Store) Evaluate(ctx context.Context, session, key, requestID string, command EvaluationCommand) (Outcome, bool, error) {
	var result Outcome
	if !validIdempotency(key, requestID) {
		return result, false, ErrInvalid
	}
	command, err := normalizeEvaluation(command)
	if err != nil {
		return result, false, err
	}
	request, _ := json.Marshal(command)
	requestHash := digest(request)
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		enrollmentID, episodeID, e := learnerEpisode(ctx, tx, actor.ID, command.ClassID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "completion-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := commandReplay(ctx, tx, actor.ID, key, requestHash, "EVALUATE")
		if e != nil {
			return e
		}
		if found {
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "completion-episode:"+command.ClassID+":"+episodeID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "completion-outcome:"+command.RuleVersionID+":"+episodeID); e != nil {
			return e
		}
		// CP01 can run on its original schema. Once CP02 is installed, an
		// override is authoritative until an explicit REOPEN; a reopened
		// activity can be evaluated again without mutating earlier outcomes.
		var correctionInstalled bool
		if e = tx.QueryRow(ctx, "SELECT to_regclass('v3.completion_corrections') IS NOT NULL").Scan(&correctionInstalled); e != nil {
			return e
		}
		if correctionInstalled {
			var action *string
			var base *int64
			var count int64
			e = tx.QueryRow(ctx, `SELECT
			 (SELECT c.action FROM v3.completion_corrections c WHERE c.rule_version_id=$1 AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1),
			 (SELECT c.base_outcome_count FROM v3.completion_corrections c WHERE c.rule_version_id=$1 AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1),
			 (SELECT count(*) FROM v3.activity_completion_outcomes o WHERE o.rule_version_id=$1 AND o.episode_id=$2)`,
				command.RuleVersionID, episodeID).Scan(&action, &base, &count)
			if e != nil {
				return e
			}
			if action != nil && base != nil && *base == count && *action != "REOPEN" {
				return ErrConflict
			}
		}
		var requirement, evidenceKind string
		var version, latest int64
		e = tx.QueryRow(ctx, `SELECT r.requirement,r.evidence_kind,r.version,
		 (SELECT max(x.version) FROM v3.completion_rule_versions x WHERE x.activity_id=r.activity_id)
		 FROM v3.completion_rule_versions r WHERE r.id=$1 AND r.class_id=$2 AND r.activity_id=$3 AND r.content_revision_id=$4`,
			command.RuleVersionID, command.ClassID, command.ActivityID, command.ContentRevisionID).Scan(&requirement, &evidenceKind, &version, &latest)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if version != latest || requirement != "EVIDENCE_PRESENT" || evidenceKind != "ACTIVITY_ACKNOWLEDGED" {
			return ErrConflict
		}
		var evidenceID *string
		state, reason := "INCOMPLETE", "EVIDENCE_MISSING"
		if command.Acknowledge {
			id, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			_, e = tx.Exec(ctx, `INSERT INTO v3.learning_evidence(id,class_id,activity_id,content_revision_id,rule_version_id,enrollment_id,episode_id,learner_user_id,evidence_kind,source_ref,recorded_by)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$8)`, id, command.ClassID, command.ActivityID, command.ContentRevisionID,
				command.RuleVersionID, enrollmentID, episodeID, actor.ID, evidenceKind, "command:"+digest([]byte(actor.ID+":"+key)))
			if e != nil {
				return mapWriteError(e)
			}
			evidenceID, state, reason = &id, "COMPLETED", "EVIDENCE_SATISFIED"
		}
		outcomeID, e := token(16)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO v3.activity_completion_outcomes(id,class_id,activity_id,content_revision_id,rule_version_id,enrollment_id,episode_id,learner_user_id,evidence_id,state,reason,evaluated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$8)
		 RETURNING id,class_id,activity_id,content_revision_id,rule_version_id,enrollment_id,episode_id,learner_user_id,evidence_id,state,reason,evaluated_at`,
			outcomeID, command.ClassID, command.ActivityID, command.ContentRevisionID, command.RuleVersionID, enrollmentID, episodeID, actor.ID, evidenceID, state, reason).
			Scan(&result.ID, &result.ClassID, &result.ActivityID, &result.ContentRevisionID, &result.RuleVersionID, &result.EnrollmentID, &result.EpisodeID, &result.LearnerUserID, &result.EvidenceID, &result.State, &result.Reason, &result.EvaluatedAt)
		if e != nil {
			return mapWriteError(e)
		}
		response, _ := json.Marshal(result)
		if e = audit(ctx, tx, actor.ID, command.ClassID, "completion.evaluate", requestID); e != nil {
			return e
		}
		return saveCommand(ctx, tx, actor.ID, key, "EVALUATE", requestHash, response)
	})
	return result, replayed, mapWriteError(err)
}
