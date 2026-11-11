package completion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

type ActivityProgress struct {
	ActivityID    string `json:"activity_id"`
	RuleVersionID string `json:"rule_version_id"`
	State         string `json:"state"`
	Version       int64  `json:"version"`
}

type Progress struct {
	ClassID       string             `json:"class_id"`
	LearnerUserID string             `json:"learner_user_id"`
	EnrollmentID  string             `json:"enrollment_id"`
	EpisodeID     string             `json:"episode_id"`
	Completed     int                `json:"completed"`
	Total         int                `json:"total"`
	Activities    []ActivityProgress `json:"activities"`
}

type CorrectionCommand struct {
	ClassID         string `json:"class_id"`
	ActivityID      string `json:"activity_id"`
	RuleVersionID   string `json:"rule_version_id"`
	LearnerUserID   string `json:"learner_user_id"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

type Correction struct {
	ID            string `json:"id"`
	ClassID       string `json:"class_id"`
	ActivityID    string `json:"activity_id"`
	RuleVersionID string `json:"rule_version_id"`
	LearnerUserID string `json:"learner_user_id"`
	EpisodeID     string `json:"episode_id"`
	Action        string `json:"action"`
	State         string `json:"state"`
	Reason        string `json:"reason"`
	Version       int64  `json:"version"`
}

// projectionState is deterministic even if a correction is followed by a new
// learner evaluation after REOPEN. No stored projection can drift.
func projectionState(outcomes, base int64, outcomeState, correctionState *string) string {
	if correctionState != nil && outcomes == base {
		return *correctionState
	}
	if outcomeState != nil {
		return *outcomeState
	}
	return "INCOMPLETE"
}

func readProjection(ctx context.Context, tx pgx.Tx, classID, userID, enrollmentID, episodeID string) (Progress, error) {
	p := Progress{ClassID: classID, LearnerUserID: userID, EnrollmentID: enrollmentID, EpisodeID: episodeID, Activities: []ActivityProgress{}}
	rows, err := tx.Query(ctx, `SELECT r.activity_id,r.id,
	  (SELECT count(*) FROM v3.activity_completion_outcomes o WHERE o.rule_version_id=r.id AND o.episode_id=$2),
	  (SELECT count(*) FROM v3.completion_corrections c WHERE c.rule_version_id=r.id AND c.episode_id=$2),
	  (SELECT o.state FROM v3.activity_completion_outcomes o WHERE o.rule_version_id=r.id AND o.episode_id=$2 ORDER BY o.evaluated_at DESC,o.id DESC LIMIT 1),
	  (SELECT c.state FROM v3.completion_corrections c WHERE c.rule_version_id=r.id AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1),
	  (SELECT c.base_outcome_count FROM v3.completion_corrections c WHERE c.rule_version_id=r.id AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1)
	 FROM v3.completion_rule_versions r JOIN v3.content_revisions cr ON cr.id=r.content_revision_id AND cr.class_id=r.class_id AND cr.activity_id=r.activity_id
	 WHERE r.class_id=$1 AND cr.state='PUBLISHED' AND r.version=(SELECT max(x.version) FROM v3.completion_rule_versions x WHERE x.activity_id=r.activity_id)
	 ORDER BY r.activity_id`, classID, episodeID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ActivityProgress
		var outcomes, corrections int64
		var outcomeState, correctionState *string
		var base *int64
		if err = rows.Scan(&item.ActivityID, &item.RuleVersionID, &outcomes, &corrections, &outcomeState, &correctionState, &base); err != nil {
			return p, err
		}
		baseCount := int64(-1)
		if base != nil {
			baseCount = *base
		}
		item.State = projectionState(outcomes, baseCount, outcomeState, correctionState)
		item.Version = outcomes + corrections
		p.Activities = append(p.Activities, item)
		if item.State == "COMPLETED" {
			p.Completed++
		}
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	p.Total = len(p.Activities)
	return p, nil
}

// Progress always rebuilds from immutable history. GET has no repair side effect.
func (s *Store) Progress(ctx context.Context, session, classID, targetUserID string) (Progress, error) {
	var result Progress
	if classID == "" {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		if targetUserID == "" {
			targetUserID = actor.ID
		}
		if actor.ID != targetUserID {
			allowed, e := teacher(ctx, tx, actor, classID)
			if e != nil {
				return e
			}
			if !allowed {
				return ErrForbidden
			}
		}
		enrollmentID, episodeID, e := learnerEpisode(ctx, tx, targetUserID, classID)
		if e != nil {
			return e
		}
		result, e = readProjection(ctx, tx, classID, targetUserID, enrollmentID, episodeID)
		return e
	})
	return result, err
}

func normalizeCorrection(c CorrectionCommand) (CorrectionCommand, error) {
	c.Action = strings.ToUpper(strings.TrimSpace(c.Action))
	c.Reason = strings.TrimSpace(c.Reason)
	if c.ClassID == "" || c.ActivityID == "" || c.RuleVersionID == "" || c.LearnerUserID == "" ||
		c.ExpectedVersion < 0 || len(c.Reason) < 8 || len(c.Reason) > 500 ||
		(c.Action != "OVERRIDE_COMPLETE" && c.Action != "OVERRIDE_INCOMPLETE" && c.Action != "REOPEN") {
		return c, ErrInvalid
	}
	return c, nil
}

func correctionReplay(ctx context.Context, tx pgx.Tx, actor, key, hash string) (Correction, bool, error) {
	var result Correction
	var priorHash string
	var response json.RawMessage
	err := tx.QueryRow(ctx, `SELECT request_hash,response FROM v3.completion_correction_commands WHERE actor_id=$1 AND key=$2`, actor, key).Scan(&priorHash, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if priorHash != hash {
		return result, false, ErrConflict
	}
	err = json.Unmarshal(response, &result)
	return result, true, err
}

func (s *Store) Correct(ctx context.Context, session, key, requestID string, command CorrectionCommand) (Correction, bool, error) {
	var result Correction
	if !validIdempotency(key, requestID) {
		return result, false, ErrInvalid
	}
	command, err := normalizeCorrection(command)
	if err != nil {
		return result, false, err
	}
	request, _ := json.Marshal(command)
	hash := digest(request)
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
		enrollmentID, episodeID, e := learnerEpisode(ctx, tx, command.LearnerUserID, command.ClassID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "completion-correction-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := correctionReplay(ctx, tx, actor.ID, key, hash)
		if e != nil {
			return e
		}
		if found {
			replayed = true
			result = prior
			return nil
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "completion-episode:"+command.ClassID+":"+episodeID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "completion-outcome:"+command.RuleVersionID+":"+episodeID); e != nil {
			return e
		}
		var revisionID string
		e = tx.QueryRow(ctx, `SELECT r.content_revision_id FROM v3.completion_rule_versions r
		 WHERE r.id=$1 AND r.class_id=$2 AND r.activity_id=$3
		 AND r.version=(SELECT max(x.version) FROM v3.completion_rule_versions x WHERE x.activity_id=r.activity_id)`,
			command.RuleVersionID, command.ClassID, command.ActivityID).Scan(&revisionID)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		var outcomes, corrections int64
		var outcomeState, correctionState *string
		var base *int64
		e = tx.QueryRow(ctx, `SELECT
		 (SELECT count(*) FROM v3.activity_completion_outcomes o WHERE o.rule_version_id=$1 AND o.episode_id=$2),
		 (SELECT count(*) FROM v3.completion_corrections c WHERE c.rule_version_id=$1 AND c.episode_id=$2),
		 (SELECT o.state FROM v3.activity_completion_outcomes o WHERE o.rule_version_id=$1 AND o.episode_id=$2 ORDER BY o.evaluated_at DESC,o.id DESC LIMIT 1),
		 (SELECT c.state FROM v3.completion_corrections c WHERE c.rule_version_id=$1 AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1),
		 (SELECT c.base_outcome_count FROM v3.completion_corrections c WHERE c.rule_version_id=$1 AND c.episode_id=$2 ORDER BY c.version DESC LIMIT 1)`, command.RuleVersionID, episodeID).Scan(&outcomes, &corrections, &outcomeState, &correctionState, &base)
		if e != nil {
			return e
		}
		if command.ExpectedVersion != outcomes+corrections {
			return ErrConflict
		}
		baseCount := int64(-1)
		if base != nil {
			baseCount = *base
		}
		if command.Action == "REOPEN" && projectionState(outcomes, baseCount, outcomeState, correctionState) != "COMPLETED" {
			return ErrConflict
		}
		state := "INCOMPLETE"
		if command.Action == "OVERRIDE_COMPLETE" {
			state = "COMPLETED"
		}
		id, e := token(16)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO v3.completion_corrections(id,class_id,activity_id,content_revision_id,rule_version_id,enrollment_id,episode_id,learner_user_id,actor_id,action,state,reason,version,base_outcome_count)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING id,class_id,activity_id,rule_version_id,learner_user_id,episode_id,action,state,reason,version`,
			id, command.ClassID, command.ActivityID, revisionID, command.RuleVersionID, enrollmentID, episodeID, command.LearnerUserID, actor.ID,
			command.Action, state, command.Reason, outcomes+corrections+1, outcomes).
			Scan(&result.ID, &result.ClassID, &result.ActivityID, &result.RuleVersionID, &result.LearnerUserID, &result.EpisodeID,
				&result.Action, &result.State, &result.Reason, &result.Version)
		if e != nil {
			return mapWriteError(e)
		}
		if e = audit(ctx, tx, actor.ID, command.ClassID, "completion.correct", requestID); e != nil {
			return e
		}
		response, _ := json.Marshal(result)
		_, e = tx.Exec(ctx, `INSERT INTO v3.completion_correction_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb)`,
			actor.ID, key, hash, string(response))
		return e
	})
	return result, replayed, mapWriteError(err)
}
