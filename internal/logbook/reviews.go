package logbook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

type ReviewCommand struct {
	ClassID         string
	EntryID         string
	RevisionID      string
	ExpectedVersion int64
	Decision        string
	Feedback        string
}

func validateReview(c ReviewCommand) error {
	if !idPattern.MatchString(c.ClassID) || !idPattern.MatchString(c.EntryID) || !idPattern.MatchString(c.RevisionID) || c.ExpectedVersion < 1 || (c.Decision != "REVISION_REQUIRED" && c.Decision != "ACCEPTED") || !safeText(c.Feedback, 8000) {
		return ErrInvalid
	}
	return nil
}

func reviewDigest(c ReviewCommand) string {
	data, _ := json.Marshal(c)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Store) Review(ctx context.Context, session, key, requestID string, c ReviewCommand) (json.RawMessage, bool, error) {
	if s == nil || s.Pool == nil || !keyPattern.MatchString(key) || strings.TrimSpace(requestID) == "" {
		return nil, false, ErrInvalid
	}
	if err := validateReview(c); err != nil {
		return nil, false, err
	}
	hash := reviewDigest(c)
	var response json.RawMessage
	replayed := false
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+"/"+key); err != nil {
			return err
		}
		var prior string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.logbook_review_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
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
		var owner, studentEnrollment, state, revision string
		var version int64
		err = tx.QueryRow(ctx, `SELECT e.owner_user_id,e.enrollment_id,ev.state,ev.revision_id,ev.version FROM v3.logbook_entries e JOIN LATERAL (SELECT state,revision_id,version FROM v3.logbook_entry_events WHERE entry_id=e.id ORDER BY version DESC LIMIT 1) ev ON true WHERE e.id=$1 AND e.class_id=$2 FOR UPDATE OF e`, c.EntryID, c.ClassID).Scan(&owner, &studentEnrollment, &state, &revision, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if actor.ID == owner {
			return ErrForbidden
		}
		if state != "SUBMITTED" || revision != c.RevisionID || version != c.ExpectedVersion {
			return ErrConflict
		}
		var assignment any
		if !actor.Admin {
			var assignmentID string
			err = tx.QueryRow(ctx, `SELECT m.id FROM v3.mentor_assignments m JOIN v3.enrollments me ON me.id=m.mentor_enrollment_id AND me.class_id=m.class_id JOIN v3.roles r ON r.enrollment_id=me.id AND r.class_id=me.class_id
 WHERE m.student_enrollment_id=$1 AND m.class_id=$2 AND m.ended_at IS NULL AND me.user_id=$3 AND me.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=me.id AND x.suspended_at IS NOT NULL) FOR SHARE OF m,me,r`, studentEnrollment, c.ClassID, actor.ID).Scan(&assignmentID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrForbidden
			}
			if err != nil {
				return err
			}
			assignment = assignmentID
		}
		eventID, err := token()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entry_events(id,entry_id,class_id,revision_id,version,state,actor_id,mentor_assignment_id,feedback) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, eventID, c.EntryID, c.ClassID, revision, version+1, c.Decision, actor.ID, assignment, c.Feedback); err != nil {
			return mapEntryError(err)
		}
		view, err := loadEntry(ctx, tx, c.EntryID)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, actor.ID, c.ClassID, "logbook.entry.review."+strings.ToLower(c.Decision), requestID); err != nil {
			return err
		}
		response, err = json.Marshal(map[string]any{"entry": view})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO v3.logbook_review_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4)`, actor.ID, key, hash, response)
		return err
	})
	if err != nil {
		return nil, false, mapEntryError(err)
	}
	return response, replayed, nil
}
