package completion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

type IssueCommand struct {
	ClassID       string `json:"class_id"`
	LearnerUserID string `json:"learner_user_id"`
	TranscriptID  string `json:"transcript_id"`
	Privacy       string `json:"privacy"`
}

type TransitionCommand struct {
	CredentialID          string `json:"credential_id"`
	ExpectedVersion       int64  `json:"expected_version"`
	Reason                string `json:"reason"`
	SuccessorTranscriptID string `json:"successor_transcript_id,omitempty"`
	Privacy               string `json:"privacy,omitempty"`
}

type Credential struct {
	ID            string `json:"id"`
	ClassID       string `json:"class_id"`
	LearnerUserID string `json:"learner_user_id"`
	TranscriptID  string `json:"transcript_id"`
	Privacy       string `json:"privacy"`
	Status        string `json:"status"`
	Version       int64  `json:"version"`
	SuccessorID   string `json:"successor_id,omitempty"`
}

type Verification struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func validPrivacy(value string) bool { return value == "PUBLIC_MINIMAL" || value == "PRIVATE" }

func eligibility(ctx context.Context, tx pgx.Tx, classID, learnerID, transcriptID string) (string, error) {
	enrollmentID, episodeID, err := learnerEpisode(ctx, tx, learnerID, classID)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "completion-episode:"+classID+":"+episodeID); err != nil {
		return "", err
	}
	var selectedID, sourceHash string
	err = tx.QueryRow(ctx, `SELECT id,grade_source_hash FROM v3.transcript_releases
	 WHERE class_id=$1 AND learner_user_id=$2 AND enrollment_id=$3 AND episode_id=$4 ORDER BY version DESC LIMIT 1`,
		classID, learnerID, enrollmentID, episodeID).Scan(&selectedID, &sourceHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConflict
	}
	if err != nil {
		return "", err
	}
	if selectedID != transcriptID {
		return "", ErrConflict
	}
	items, currentHash, err := gradeSnapshot(ctx, tx, classID, learnerID, episodeID)
	if err != nil {
		return "", err
	}
	if len(items) == 0 || currentHash != sourceHash {
		return "", ErrConflict
	}
	progress, err := readProjection(ctx, tx, classID, learnerID, enrollmentID, episodeID)
	if err != nil {
		return "", err
	}
	if progress.Total == 0 || progress.Completed != progress.Total {
		return "", ErrConflict
	}
	return episodeID, nil
}

func insertIssued(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID, learnerID, transcriptID, privacy string) (Credential, error) {
	var result Credential
	id, err := token(16)
	if err != nil {
		return result, err
	}
	eventID, err := token(16)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v3.credential_records(id,class_id,learner_user_id,transcript_id,privacy,issued_by)
	 VALUES($1,$2,$3,$4,$5,$6)`, id, classID, learnerID, transcriptID, privacy, actor.ID)
	if err != nil {
		return result, mapWriteError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO v3.credential_events(id,credential_id,action,version,actor_id)
	 VALUES($1,$2,'ISSUED',1,$3)`, eventID, id, actor.ID)
	if err != nil {
		return result, mapWriteError(err)
	}
	return Credential{ID: id, ClassID: classID, LearnerUserID: learnerID, TranscriptID: transcriptID, Privacy: privacy, Status: "VALID", Version: 1}, nil
}

func (s *Store) Issue(ctx context.Context, session, key, requestID string, c IssueCommand) (Credential, bool, error) {
	var result Credential
	if !validIdempotency(key, requestID) || c.ClassID == "" || c.LearnerUserID == "" || c.TranscriptID == "" || !validPrivacy(c.Privacy) {
		return result, false, ErrInvalid
	}
	raw, _ := json.Marshal(c)
	hash := digest(raw)
	replayed := false
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, c.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "credential-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := credentialReplay(ctx, tx, actor.ID, key, "ISSUE", hash)
		if e != nil {
			return e
		}
		if found {
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if _, e = eligibility(ctx, tx, c.ClassID, c.LearnerUserID, c.TranscriptID); e != nil {
			return e
		}
		result, e = insertIssued(ctx, tx, actor, c.ClassID, c.LearnerUserID, c.TranscriptID, c.Privacy)
		if e != nil {
			return e
		}
		if e = audit(ctx, tx, actor.ID, c.ClassID, "credential.issue", requestID); e != nil {
			return e
		}
		return saveCredentialCommand(ctx, tx, actor.ID, key, "ISSUE", hash, result)
	})
	return result, replayed, mapWriteError(err)
}

func loadCredential(ctx context.Context, tx pgx.Tx, id string) (Credential, error) {
	var result Credential
	var action string
	err := tx.QueryRow(ctx, `SELECT c.id,c.class_id,c.learner_user_id,c.transcript_id,c.privacy,e.action,e.version
	 FROM v3.credential_records c JOIN LATERAL(SELECT action,version FROM v3.credential_events WHERE credential_id=c.id ORDER BY version DESC LIMIT 1)e ON true
	 WHERE c.id=$1`, id).Scan(&result.ID, &result.ClassID, &result.LearnerUserID, &result.TranscriptID, &result.Privacy, &action, &result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	switch action {
	case "ISSUED":
		result.Status = "VALID"
	case "REVOKED":
		result.Status = "REVOKED"
	case "SUPERSEDED":
		result.Status = "SUPERSEDED"
	}
	return result, nil
}

func normalizeTransition(c TransitionCommand, kind string) (TransitionCommand, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.CredentialID == "" || c.ExpectedVersion < 1 || len(c.Reason) < 8 || len(c.Reason) > 500 {
		return c, ErrInvalid
	}
	if kind == "REVOKE" && (c.SuccessorTranscriptID != "" || c.Privacy != "") {
		return c, ErrInvalid
	}
	if kind == "SUPERSEDE" && (c.SuccessorTranscriptID == "" || !validPrivacy(c.Privacy)) {
		return c, ErrInvalid
	}
	return c, nil
}

func (s *Store) transition(ctx context.Context, session, key, requestID, kind string, c TransitionCommand) (Credential, bool, error) {
	var result Credential
	if !validIdempotency(key, requestID) {
		return result, false, ErrInvalid
	}
	c, err := normalizeTransition(c, kind)
	if err != nil {
		return result, false, err
	}
	raw, _ := json.Marshal(c)
	hash := digest(raw)
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		preflight, e := loadCredential(ctx, tx, c.CredentialID)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, preflight.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "credential-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := credentialReplay(ctx, tx, actor.ID, key, kind, hash)
		if e != nil {
			return e
		}
		if found {
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "credential:"+c.CredentialID); e != nil {
			return e
		}
		current, e := loadCredential(ctx, tx, c.CredentialID)
		if e != nil {
			return e
		}
		allowed, e = teacher(ctx, tx, actor, current.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if current.Status != "VALID" || current.Version != c.ExpectedVersion {
			return ErrConflict
		}
		var successorID *string
		if kind == "SUPERSEDE" {
			if c.SuccessorTranscriptID == current.TranscriptID {
				return ErrConflict
			}
			if _, e = eligibility(ctx, tx, current.ClassID, current.LearnerUserID, c.SuccessorTranscriptID); e != nil {
				return e
			}
			successor, issueErr := insertIssued(ctx, tx, actor, current.ClassID, current.LearnerUserID, c.SuccessorTranscriptID, c.Privacy)
			if issueErr != nil {
				return issueErr
			}
			successorID = &successor.ID
			result.SuccessorID = successor.ID
		}
		id, e := token(16)
		if e != nil {
			return e
		}
		action := "REVOKED"
		if kind == "SUPERSEDE" {
			action = "SUPERSEDED"
		}
		_, e = tx.Exec(ctx, `INSERT INTO v3.credential_events(id,credential_id,action,version,actor_id,reason,successor_id)
		 VALUES($1,$2,$3,$4,$5,$6,$7)`, id, current.ID, action, current.Version+1, actor.ID, c.Reason, successorID)
		if e != nil {
			return mapWriteError(e)
		}
		result.ID, result.ClassID, result.LearnerUserID, result.TranscriptID, result.Privacy = current.ID, current.ClassID, current.LearnerUserID, current.TranscriptID, current.Privacy
		result.Status, result.Version = action, current.Version+1
		if e = audit(ctx, tx, actor.ID, current.ClassID, "credential."+strings.ToLower(kind), requestID); e != nil {
			return e
		}
		return saveCredentialCommand(ctx, tx, actor.ID, key, kind, hash, result)
	})
	return result, replayed, mapWriteError(err)
}

func (s *Store) Revoke(ctx context.Context, session, key, requestID string, c TransitionCommand) (Credential, bool, error) {
	return s.transition(ctx, session, key, requestID, "REVOKE", c)
}
func (s *Store) Supersede(ctx context.Context, session, key, requestID string, c TransitionCommand) (Credential, bool, error) {
	return s.transition(ctx, session, key, requestID, "SUPERSEDE", c)
}

// Public verification is intentionally non-identifying; private and unknown
// records share the same NOT_FOUND result.
func (s *Store) Verify(ctx context.Context, id string) (Verification, error) {
	var result Verification
	if len(id) != 32 {
		return result, ErrNotFound
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return result, ErrNotFound
		}
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		current, e := loadCredential(ctx, tx, id)
		if e != nil {
			return e
		}
		if current.Privacy != "PUBLIC_MINIMAL" {
			return ErrNotFound
		}
		result = Verification{ID: id, Status: current.Status}
		return nil
	})
	return result, err
}
