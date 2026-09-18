package logbook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

type EntryCommand struct {
	Action          string
	ClassID         string
	EntryID         string
	PeriodID        string
	RevisionID      string
	Body            string
	FileAssetIDs    []string
	ExpectedVersion int64
}

type EntryView struct {
	ID            string    `json:"id"`
	ClassID       string    `json:"class_id"`
	PeriodID      string    `json:"period_id"`
	RevisionID    string    `json:"revision_id"`
	RevisionNo    int       `json:"revision_no"`
	RevisionCount int       `json:"revision_count"`
	State         string    `json:"state"`
	Version       int64     `json:"version"`
	Body          string    `json:"body"`
	FileAssetIDs  []string  `json:"file_asset_ids"`
	Feedback      string    `json:"feedback,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

func validIDs(values []string) bool {
	if len(values) > 10 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !idPattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validateEntry(c EntryCommand) error {
	if !idPattern.MatchString(c.ClassID) {
		return ErrInvalid
	}
	switch c.Action {
	case "save":
		if c.EntryID != "" || !idPattern.MatchString(c.PeriodID) || c.RevisionID != "" || c.ExpectedVersion != 0 || len(c.Body) > 65536 || (strings.TrimSpace(c.Body) == "" && len(c.FileAssetIDs) == 0) || !validIDs(c.FileAssetIDs) {
			return ErrInvalid
		}
	case "submit":
		if !idPattern.MatchString(c.EntryID) || !idPattern.MatchString(c.RevisionID) || c.PeriodID != "" || c.Body != "" || len(c.FileAssetIDs) != 0 || c.ExpectedVersion < 1 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func entryDigest(c EntryCommand) string {
	data, _ := json.Marshal(c)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Store) activeStudent(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (string, string, error) {
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM v3.classes WHERE id=$1 FOR SHARE", classID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	} else if err != nil {
		return "", "", err
	}
	var closed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", classID).Scan(&closed); err != nil {
		return "", "", err
	}
	if state != "PUBLISHED" || closed || actor.Admin {
		return "", "", ErrForbidden
	}
	var enrollment, episode string
	err := tx.QueryRow(ctx, `SELECT e.id,ep.id FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id JOIN v3.episodes ep ON ep.enrollment_id=e.id AND ep.ended_at IS NULL
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='student' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) FOR SHARE OF e,r,ep`, classID, actor.ID).Scan(&enrollment, &episode)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrForbidden
	}
	return enrollment, episode, err
}

func ensureOpenPeriod(ctx context.Context, tx pgx.Tx, classID, periodID string) error {
	var open bool
	err := tx.QueryRow(ctx, `SELECT p.starts_at<=clock_timestamp() AND p.due_at>=clock_timestamp() FROM v3.logbook_periods p JOIN v3.logbook_period_publications pub ON pub.set_id=p.set_id AND pub.class_id=p.class_id
 WHERE p.id=$1 AND p.class_id=$2 AND pub.revision=(SELECT max(x.revision) FROM v3.logbook_period_publications x WHERE x.class_id=$2)`, periodID, classID).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !open {
		return ErrConflict
	}
	return nil
}

func mapEntryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514") {
		return ErrConflict
	}
	return err
}

func (s *Store) ExecuteEntry(ctx context.Context, session, key, requestID string, c EntryCommand) (json.RawMessage, bool, error) {
	if s == nil || s.Pool == nil || !keyPattern.MatchString(key) || requestID == "" {
		return nil, false, ErrInvalid
	}
	if err := validateEntry(c); err != nil {
		return nil, false, err
	}
	hash := entryDigest(c)
	var response json.RawMessage
	replayed := false
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		enrollment, episode, err := s.activeStudent(ctx, tx, actor, c.ClassID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+"/"+key); err != nil {
			return err
		}
		var prior string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.logbook_entry_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
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
		var entryID, periodID string
		if c.Action == "save" {
			if err = ensureOpenPeriod(ctx, tx, c.ClassID, c.PeriodID); err != nil {
				return err
			}
			periodID = c.PeriodID
			err = tx.QueryRow(ctx, "SELECT id FROM v3.logbook_entries WHERE period_id=$1 AND episode_id=$2 FOR UPDATE", c.PeriodID, episode).Scan(&entryID)
			if errors.Is(err, pgx.ErrNoRows) {
				entryID, err = token()
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entries(id,class_id,period_id,enrollment_id,episode_id,owner_user_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$6)`, entryID, c.ClassID, periodID, enrollment, episode, actor.ID)
				if err != nil {
					return mapEntryError(err)
				}
			} else if err != nil {
				return err
			}
			var revisionNo int
			if err = tx.QueryRow(ctx, "SELECT coalesce(max(revision_no),0)+1 FROM v3.logbook_entry_revisions WHERE entry_id=$1", entryID).Scan(&revisionNo); err != nil {
				return err
			}
			revisionID, e := token()
			if e != nil {
				return e
			}
			bodyHash := sha256.Sum256([]byte(c.Body))
			if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entry_revisions(id,entry_id,class_id,revision_no,body,body_sha256,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, revisionID, entryID, c.ClassID, revisionNo, c.Body, hex.EncodeToString(bodyHash[:]), actor.ID); err != nil {
				return mapEntryError(err)
			}
			for i, assetID := range c.FileAssetIDs {
				if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_revision_files(revision_id,entry_id,class_id,file_asset_id,position) VALUES($1,$2,$3,$4,$5)`, revisionID, entryID, c.ClassID, assetID, i+1); err != nil {
					return mapEntryError(err)
				}
			}
			var version int64
			if err = tx.QueryRow(ctx, "SELECT coalesce(max(version),0)+1 FROM v3.logbook_entry_events WHERE entry_id=$1", entryID).Scan(&version); err != nil {
				return err
			}
			eventID, e := token()
			if e != nil {
				return e
			}
			if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entry_events(id,entry_id,class_id,revision_id,version,state,actor_id) VALUES($1,$2,$3,$4,$5,'DRAFT',$6)`, eventID, entryID, c.ClassID, revisionID, version, actor.ID); err != nil {
				return mapEntryError(err)
			}
		} else {
			entryID = c.EntryID
			var owner string
			err = tx.QueryRow(ctx, "SELECT period_id,owner_user_id FROM v3.logbook_entries WHERE id=$1 AND class_id=$2 AND enrollment_id=$3 AND episode_id=$4 FOR UPDATE", entryID, c.ClassID, enrollment, episode).Scan(&periodID, &owner)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != actor.ID) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if err = ensureOpenPeriod(ctx, tx, c.ClassID, periodID); err != nil {
				return err
			}
			var state, revision string
			var version int64
			if err = tx.QueryRow(ctx, "SELECT state,revision_id,version FROM v3.logbook_entry_events WHERE entry_id=$1 ORDER BY version DESC LIMIT 1", entryID).Scan(&state, &revision, &version); err != nil {
				return err
			}
			if state != "DRAFT" || revision != c.RevisionID || version != c.ExpectedVersion {
				return ErrConflict
			}
			eventID, e := token()
			if e != nil {
				return e
			}
			if _, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entry_events(id,entry_id,class_id,revision_id,version,state,actor_id) VALUES($1,$2,$3,$4,$5,'SUBMITTED',$6)`, eventID, entryID, c.ClassID, revision, version+1, actor.ID); err != nil {
				return mapEntryError(err)
			}
		}
		view, err := loadEntry(ctx, tx, entryID)
		if err != nil {
			return err
		}
		if err = audit(ctx, tx, actor.ID, c.ClassID, "logbook.entry."+c.Action, requestID); err != nil {
			return err
		}
		response, err = json.Marshal(map[string]any{"entry": view})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO v3.logbook_entry_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4)`, actor.ID, key, hash, response)
		return err
	})
	if err != nil {
		return nil, false, mapEntryError(err)
	}
	return response, replayed, nil
}

func loadEntry(ctx context.Context, tx pgx.Tx, entryID string) (EntryView, error) {
	view := EntryView{FileAssetIDs: []string{}}
	err := tx.QueryRow(ctx, `SELECT e.id,e.class_id,e.period_id,ev.revision_id,r.revision_no,(SELECT count(*) FROM v3.logbook_entry_revisions x WHERE x.entry_id=e.id),ev.state,ev.version,r.body,ev.feedback,ev.occurred_at
 FROM v3.logbook_entries e JOIN LATERAL (SELECT * FROM v3.logbook_entry_events WHERE entry_id=e.id ORDER BY version DESC LIMIT 1) ev ON true JOIN v3.logbook_entry_revisions r ON r.id=ev.revision_id AND r.entry_id=e.id WHERE e.id=$1`, entryID).Scan(&view.ID, &view.ClassID, &view.PeriodID, &view.RevisionID, &view.RevisionNo, &view.RevisionCount, &view.State, &view.Version, &view.Body, &view.Feedback, &view.OccurredAt)
	if err != nil {
		return EntryView{}, err
	}
	rows, err := tx.Query(ctx, "SELECT file_asset_id FROM v3.logbook_revision_files WHERE revision_id=$1 ORDER BY position", view.RevisionID)
	if err != nil {
		return EntryView{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return EntryView{}, err
		}
		view.FileAssetIDs = append(view.FileAssetIDs, id)
	}
	return view, rows.Err()
}

func (s *Store) authorizeEntryRead(ctx context.Context, tx pgx.Tx, actor identity.Actor, entryID, classID string) error {
	var owner, studentEnrollment string
	err := tx.QueryRow(ctx, "SELECT owner_user_id,enrollment_id FROM v3.logbook_entries WHERE id=$1 AND class_id=$2", entryID, classID).Scan(&owner, &studentEnrollment)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if actor.Admin {
		return nil
	}
	if actor.ID == owner {
		var active bool
		if err = tx.QueryRow(ctx, `SELECT e.status='ACTIVE' AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL) FROM v3.enrollments e WHERE e.id=$1`, studentEnrollment).Scan(&active); err != nil || !active {
			return ErrForbidden
		}
		return nil
	}
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.mentor_assignments m JOIN v3.enrollments me ON me.id=m.mentor_enrollment_id AND me.class_id=m.class_id JOIN v3.roles r ON r.enrollment_id=me.id AND r.class_id=me.class_id
 WHERE m.student_enrollment_id=$1 AND m.class_id=$2 AND m.ended_at IS NULL AND me.user_id=$3 AND me.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=me.id AND x.suspended_at IS NOT NULL))`, studentEnrollment, classID, actor.ID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Store) ReadEntry(ctx context.Context, session, classID, entryID string) (EntryView, error) {
	if s == nil || s.Pool == nil || !idPattern.MatchString(classID) || !idPattern.MatchString(entryID) {
		return EntryView{}, ErrInvalid
	}
	var result EntryView
	err := transaction.Within(ctx, s.Pool, func(tx pgx.Tx) error {
		actor, err := identity.ResolveBridge(ctx, tx, session)
		if err != nil {
			return err
		}
		if err = s.authorizeEntryRead(ctx, tx, actor, entryID, classID); err != nil {
			return err
		}
		result, err = loadEntry(ctx, tx, entryID)
		return err
	})
	if err != nil {
		return EntryView{}, err
	}
	return result, nil
}
