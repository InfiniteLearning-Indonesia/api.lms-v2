package gradebook

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lmsv3/internal/platform/transaction"
)

type ImportEntry struct {
	AggregateID          string `json:"aggregate_id"`
	SubmissionRevisionID string `json:"submission_revision_id"`
	ExpectedVersion      int64  `json:"expected_version"`
	ValueMilli           int64  `json:"value_milli"`
	Reason               string `json:"reason,omitempty"`
}

type ImportCommand struct {
	ClassID     string        `json:"class_id"`
	GradeItemID string        `json:"grade_item_id"`
	Entries     []ImportEntry `json:"entries"`
}

type ImportPreview struct {
	ID                  string        `json:"id"`
	ClassID             string        `json:"class_id"`
	GradeItemID         string        `json:"grade_item_id"`
	DefinitionVersionID string        `json:"definition_version_id"`
	PublicationRevision int64         `json:"publication_revision"`
	InputHash           string        `json:"input_hash"`
	Entries             []ImportEntry `json:"entries"`
	ExpiresAt           time.Time     `json:"expires_at"`
}

type ConfirmImportCommand struct {
	ClassID     string        `json:"class_id"`
	GradeItemID string        `json:"grade_item_id"`
	PreviewID   string        `json:"preview_id"`
	Entries     []ImportEntry `json:"entries"`
}

type ReleaseCommand struct {
	ClassID         string `json:"class_id"`
	GradeItemID     string `json:"grade_item_id"`
	AggregateID     string `json:"aggregate_id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type Release struct {
	ID            string    `json:"id"`
	ClassID       string    `json:"class_id"`
	GradeItemID   string    `json:"grade_item_id"`
	AggregateID   string    `json:"aggregate_id"`
	RevisionID    string    `json:"revision_id"`
	GradeVersion  int64     `json:"grade_version"`
	LearnerUserID string    `json:"learner_user_id"`
	ReleasedAt    time.Time `json:"released_at"`
}

func normalizeImport(c ImportCommand) (ImportCommand, error) {
	if c.ClassID == "" || c.GradeItemID == "" || len(c.Entries) < 1 || len(c.Entries) > 50 {
		return c, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range c.Entries {
		e := &c.Entries[i]
		e.Reason = strings.TrimSpace(e.Reason)
		if e.AggregateID == "" || e.SubmissionRevisionID == "" || seen[e.AggregateID] || e.ExpectedVersion < 0 || len(e.Reason) > 1000 || !utf8.ValidString(e.Reason) || (e.ExpectedVersion > 0 && e.Reason == "") {
			return c, ErrInvalid
		}
		seen[e.AggregateID] = true
		if e.ExpectedVersion == 0 {
			e.Reason = "IMPORT_INITIAL"
		}
	}
	return c, nil
}

func lockGradeItem(ctx context.Context, tx pgx.Tx, item string) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", item)
	return err
}

func validateImportEntry(ctx context.Context, tx pgx.Tx, actor, classID, itemID, definitionID string, publicationRevision int64, e ImportEntry) (string, string, error) {
	var attempt, learner, definition string
	var revision, minimum, maximum int64
	err := tx.QueryRow(ctx, `SELECT sr.attempt_id,sa.owner_user_id,p.definition_version_id,p.revision,sc.minimum_milli,sc.maximum_milli
	 FROM v3.submission_revisions sr JOIN v3.submission_receipts receipt ON receipt.revision_id=sr.id AND receipt.class_id=sr.class_id AND receipt.attempt_id=sr.attempt_id
	 JOIN v3.submission_aggregates sa ON sa.id=sr.aggregate_id AND sa.class_id=sr.class_id
	 JOIN v3.grade_items gi ON gi.class_id=sa.class_id AND gi.activity_id=sa.activity_id
	 JOIN v3.grade_definition_publications p ON p.grade_item_id=gi.id AND p.revision=(SELECT max(x.revision) FROM v3.grade_definition_publications x WHERE x.grade_item_id=gi.id)
	 JOIN v3.grade_definition_versions d ON d.id=p.definition_version_id AND d.grade_item_id=p.grade_item_id AND d.class_id=p.class_id
	 JOIN v3.grade_scales sc ON sc.id=d.scale_id AND sc.class_id=d.class_id
	 WHERE sr.id=$1 AND sr.aggregate_id=$2 AND sr.class_id=$3 AND gi.id=$4`, e.SubmissionRevisionID, e.AggregateID, classID, itemID).Scan(&attempt, &learner, &definition, &revision, &minimum, &maximum)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if learner == actor {
		return "", "", ErrForbidden
	}
	if definition != definitionID || revision != publicationRevision {
		return "", "", ErrConflict
	}
	if e.ValueMilli < minimum || e.ValueMilli > maximum {
		return "", "", ErrInvalid
	}
	var current int64
	err = tx.QueryRow(ctx, "SELECT version FROM v3.grade_current WHERE grade_item_id=$1 AND aggregate_id=$2", itemID, e.AggregateID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		current, err = 0, nil
	}
	if err != nil {
		return "", "", err
	}
	if current != e.ExpectedVersion {
		return "", "", ErrConflict
	}
	return attempt, learner, nil
}

func currentPublication(ctx context.Context, tx pgx.Tx, classID, itemID string) (string, int64, error) {
	var definition string
	var revision int64
	err := tx.QueryRow(ctx, `SELECT p.definition_version_id,p.revision FROM v3.grade_definition_publications p
	 JOIN v3.grade_items i ON i.id=p.grade_item_id AND i.class_id=p.class_id
	 WHERE p.grade_item_id=$1 AND p.class_id=$2 ORDER BY p.revision DESC LIMIT 1`, itemID, classID).Scan(&definition, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return definition, revision, err
}

func (s *Store) PreviewImport(ctx context.Context, session, key, requestID string, command ImportCommand) (ImportPreview, bool, error) {
	var preview ImportPreview
	if !validIdempotency(key, requestID) {
		return preview, false, ErrInvalid
	}
	c, err := normalizeImport(command)
	if err != nil {
		return preview, false, err
	}
	input, _ := json.Marshal(c)
	inputHash := digest(input)
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
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
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":preview:"+key); e != nil {
			return e
		}
		var stored []byte
		e = tx.QueryRow(ctx, `SELECT id,class_id,grade_item_id,definition_version_id,publication_revision,input_hash,entries,expires_at FROM v3.grade_import_previews WHERE actor_id=$1 AND key=$2`, actor.ID, key).Scan(&preview.ID, &preview.ClassID, &preview.GradeItemID, &preview.DefinitionVersionID, &preview.PublicationRevision, &preview.InputHash, &stored, &preview.ExpiresAt)
		if e == nil {
			if preview.InputHash != inputHash {
				return ErrConflict
			}
			if e = json.Unmarshal(stored, &preview.Entries); e != nil {
				return e
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = lockGradeItem(ctx, tx, c.GradeItemID); e != nil {
			return e
		}
		definition, revision, e := currentPublication(ctx, tx, c.ClassID, c.GradeItemID)
		if e != nil {
			return e
		}
		for _, entry := range c.Entries {
			if _, _, e = validateImportEntry(ctx, tx, actor.ID, c.ClassID, c.GradeItemID, definition, revision, entry); e != nil {
				return e
			}
		}
		preview.ID, e = token(16)
		if e != nil {
			return e
		}
		preview.ClassID, preview.GradeItemID, preview.DefinitionVersionID, preview.PublicationRevision, preview.InputHash, preview.Entries = c.ClassID, c.GradeItemID, definition, revision, inputHash, c.Entries
		e = tx.QueryRow(ctx, `INSERT INTO v3.grade_import_previews(id,actor_id,key,class_id,grade_item_id,definition_version_id,publication_revision,input_hash,entries)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) RETURNING expires_at`, preview.ID, actor.ID, key, c.ClassID, c.GradeItemID, definition, revision, inputHash, string(mustJSON(c.Entries))).Scan(&preview.ExpiresAt)
		if e != nil {
			return mapWriteError(e)
		}
		return audit(ctx, tx, actor.ID, c.ClassID, "gradebook.import.preview", requestID)
	})
	return preview, replayed, mapWriteError(err)
}

func mustJSON(value any) []byte { b, _ := json.Marshal(value); return b }

func (s *Store) ConfirmImport(ctx context.Context, session, key, requestID string, command ConfirmImportCommand) (json.RawMessage, bool, error) {
	if !validIdempotency(key, requestID) || command.PreviewID == "" {
		return nil, false, ErrInvalid
	}
	c, err := normalizeImport(ImportCommand{ClassID: command.ClassID, GradeItemID: command.GradeItemID, Entries: command.Entries})
	if err != nil {
		return nil, false, err
	}
	command.Entries = c.Entries
	inputHash := digest(mustJSON(c))
	requestHash := digest(mustJSON(command))
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
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
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":confirm:"+key); e != nil {
			return e
		}
		var prior string
		e = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.grade_import_confirms WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
		if e == nil {
			if prior != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", command.PreviewID); e != nil {
			return e
		}
		var definition, storedHash string
		var revision int64
		var unexpired bool
		e = tx.QueryRow(ctx, `SELECT definition_version_id,publication_revision,input_hash,expires_at>clock_timestamp() FROM v3.grade_import_previews
		 WHERE id=$1 AND actor_id=$2 AND class_id=$3 AND grade_item_id=$4`, command.PreviewID, actor.ID, c.ClassID, c.GradeItemID).Scan(&definition, &revision, &storedHash, &unexpired)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if storedHash != inputHash || !unexpired {
			return ErrConflict
		}
		var existing bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.grade_import_confirms WHERE preview_id=$1)", command.PreviewID).Scan(&existing); e != nil {
			return e
		}
		if existing {
			return ErrConflict
		}
		if e = lockGradeItem(ctx, tx, c.GradeItemID); e != nil {
			return e
		}
		active, activeRevision, e := currentPublication(ctx, tx, c.ClassID, c.GradeItemID)
		if e != nil {
			return e
		}
		if active != definition || activeRevision != revision {
			return ErrConflict
		}
		entries := append([]ImportEntry(nil), c.Entries...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].AggregateID < entries[j].AggregateID })
		grades := make([]Grade, 0, len(entries))
		for _, entry := range entries {
			if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", c.GradeItemID+":"+entry.AggregateID); e != nil {
				return e
			}
			attempt, learner, checkErr := validateImportEntry(ctx, tx, actor.ID, c.ClassID, c.GradeItemID, definition, revision, entry)
			if checkErr != nil {
				return checkErr
			}
			gradeID, genErr := token(16)
			if genErr != nil {
				return genErr
			}
			var grade Grade
			e = scanGrade(tx.QueryRow(ctx, `INSERT INTO v3.grade_revisions(id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,learner_user_id,grader_user_id,version,value_milli,reason,source_preview_id)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,learner_user_id,grader_user_id,version,value_milli,reason,created_at`, gradeID, c.ClassID, c.GradeItemID, definition, entry.AggregateID, attempt, entry.SubmissionRevisionID, learner, actor.ID, entry.ExpectedVersion+1, entry.ValueMilli, entry.Reason, command.PreviewID), &grade)
			if e != nil {
				return mapWriteError(e)
			}
			if entry.ExpectedVersion == 0 {
				_, e = tx.Exec(ctx, "INSERT INTO v3.grade_current(grade_item_id,aggregate_id,class_id,revision_id,version) VALUES($1,$2,$3,$4,1)", c.GradeItemID, entry.AggregateID, c.ClassID, gradeID)
			} else {
				var tag pgconn.CommandTag
				tag, e = tx.Exec(ctx, "UPDATE v3.grade_current SET revision_id=$4,version=version+1,updated_at=clock_timestamp() WHERE grade_item_id=$1 AND aggregate_id=$2 AND class_id=$3 AND version=$5", c.GradeItemID, entry.AggregateID, c.ClassID, gradeID, entry.ExpectedVersion)
				if e == nil && tag.RowsAffected() != 1 {
					return ErrConflict
				}
			}
			if e != nil {
				return mapWriteError(e)
			}
			grades = append(grades, grade)
		}
		response = mustJSON(grades)
		if e = audit(ctx, tx, actor.ID, c.ClassID, "gradebook.import.confirm", requestID); e != nil {
			return e
		}
		return tx.QueryRow(ctx, "INSERT INTO v3.grade_import_confirms(preview_id,actor_id,key,request_hash,response) VALUES($1,$2,$3,$4,$5::jsonb) RETURNING response", command.PreviewID, actor.ID, key, requestHash, string(response)).Scan(&response)
	})
	return response, replayed, mapWriteError(err)
}

func (s *Store) ReleaseGrade(ctx context.Context, session, key, requestID string, c ReleaseCommand) (Release, bool, error) {
	var release Release
	if !validIdempotency(key, requestID) || c.ClassID == "" || c.GradeItemID == "" || c.AggregateID == "" || c.ExpectedVersion < 1 {
		return release, false, ErrInvalid
	}
	requestHash := digest(mustJSON(c))
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
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":release:"+key); e != nil {
			return e
		}
		var prior string
		e = tx.QueryRow(ctx, `SELECT id,class_id,grade_item_id,aggregate_id,revision_id,grade_version,learner_user_id,released_at,request_hash
		 FROM v3.grade_releases WHERE actor_id=$1 AND key=$2`, actor.ID, key).Scan(&release.ID, &release.ClassID, &release.GradeItemID, &release.AggregateID, &release.RevisionID, &release.GradeVersion, &release.LearnerUserID, &release.ReleasedAt, &prior)
		if e == nil {
			if prior != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", c.GradeItemID+":"+c.AggregateID); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT r.id,c.version,r.learner_user_id FROM v3.grade_current c JOIN v3.grade_revisions r ON r.id=c.revision_id
		 WHERE c.class_id=$1 AND c.grade_item_id=$2 AND c.aggregate_id=$3 FOR UPDATE OF c`, c.ClassID, c.GradeItemID, c.AggregateID).Scan(&release.RevisionID, &release.GradeVersion, &release.LearnerUserID)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if release.GradeVersion != c.ExpectedVersion {
			return ErrConflict
		}
		if actor.ID == release.LearnerUserID {
			return ErrForbidden
		}
		release.ID, e = token(16)
		if e != nil {
			return e
		}
		release.ClassID, release.GradeItemID, release.AggregateID = c.ClassID, c.GradeItemID, c.AggregateID
		e = tx.QueryRow(ctx, `INSERT INTO v3.grade_releases(id,class_id,grade_item_id,aggregate_id,revision_id,grade_version,learner_user_id,actor_id,key,request_hash)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING released_at`, release.ID, c.ClassID, c.GradeItemID, c.AggregateID, release.RevisionID, release.GradeVersion, release.LearnerUserID, actor.ID, key, requestHash).Scan(&release.ReleasedAt)
		if e != nil {
			return mapWriteError(e)
		}
		return audit(ctx, tx, actor.ID, c.ClassID, "gradebook.grade.release", requestID)
	})
	return release, replayed, mapWriteError(err)
}
