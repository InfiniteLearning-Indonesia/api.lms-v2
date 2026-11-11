package completion

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

const transcriptPolicy = "SYNTHETIC_M10_CP03_V1"

type TranscriptItem struct {
	ID                  string `json:"id"`
	GradeItemID         string `json:"grade_item_id"`
	GradeReleaseID      string `json:"grade_release_id"`
	GradeRevisionID     string `json:"grade_revision_id"`
	DefinitionVersionID string `json:"definition_version_id"`
	ScaleID             string `json:"scale_id"`
	RawValueMilli       int64  `json:"raw_value_milli"`
	ScaleMinimumMilli   int64  `json:"scale_minimum_milli"`
	ScaleMaximumMilli   int64  `json:"scale_maximum_milli"`
	WeightBps           int    `json:"weight_bps"`
}

type Transcript struct {
	ID              string           `json:"id"`
	ClassID         string           `json:"class_id"`
	LearnerUserID   string           `json:"learner_user_id"`
	EnrollmentID    string           `json:"enrollment_id"`
	EpisodeID       string           `json:"episode_id"`
	Version         int64            `json:"version"`
	PolicyRef       string           `json:"policy_ref"`
	GradeSourceHash string           `json:"grade_source_hash"`
	Items           []TranscriptItem `json:"items"`
}

type TranscriptCommand struct {
	ClassID       string `json:"class_id"`
	LearnerUserID string `json:"learner_user_id"`
}

func gradeSnapshot(ctx context.Context, tx pgx.Tx, classID, learnerID, episodeID string) ([]TranscriptItem, string, error) {
	items := []TranscriptItem{}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (l.grade_item_id,l.aggregate_id)
	 l.grade_item_id,l.id,r.id,d.id,s.id,r.value_milli,s.minimum_milli,s.maximum_milli,d.weight_bps
	 FROM v3.grade_releases l JOIN v3.grade_revisions r ON r.id=l.revision_id AND r.class_id=l.class_id
	 JOIN v3.submission_aggregates a ON a.id=l.aggregate_id AND a.class_id=l.class_id AND a.episode_id=$3
	 JOIN v3.grade_definition_versions d ON d.id=r.definition_version_id AND d.class_id=l.class_id AND d.grade_item_id=l.grade_item_id
	 JOIN v3.grade_scales s ON s.id=d.scale_id AND s.class_id=l.class_id
	 WHERE l.class_id=$1 AND l.learner_user_id=$2
	 ORDER BY l.grade_item_id,l.aggregate_id,l.grade_version DESC,l.id DESC`, classID, learnerID, episodeID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var item TranscriptItem
		if err = rows.Scan(&item.GradeItemID, &item.GradeReleaseID, &item.GradeRevisionID, &item.DefinitionVersionID,
			&item.ScaleID, &item.RawValueMilli, &item.ScaleMinimumMilli, &item.ScaleMaximumMilli, &item.WeightBps); err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	source, _ := json.Marshal(items)
	return items, digest(source), nil
}

func credentialReplay(ctx context.Context, tx pgx.Tx, actor, key, kind, hash string) (json.RawMessage, bool, error) {
	var priorKind, priorHash string
	var response json.RawMessage
	err := tx.QueryRow(ctx, `SELECT kind,request_hash,response FROM v3.credential_commands WHERE actor_id=$1 AND key=$2`, actor, key).Scan(&priorKind, &priorHash, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if priorKind != kind || priorHash != hash {
		return nil, false, ErrConflict
	}
	return response, true, nil
}

func saveCredentialCommand(ctx context.Context, tx pgx.Tx, actor, key, kind, hash string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v3.credential_commands(actor_id,key,kind,request_hash,response)
	 VALUES($1,$2,$3,$4,$5::jsonb)`, actor, key, kind, hash, string(raw))
	return err
}

func (s *Store) ReleaseTranscript(ctx context.Context, session, key, requestID string, c TranscriptCommand) (Transcript, bool, error) {
	var result Transcript
	if !validIdempotency(key, requestID) || c.ClassID == "" || c.LearnerUserID == "" {
		return result, false, ErrInvalid
	}
	request, _ := json.Marshal(c)
	hash := digest(request)
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
		enrollmentID, episodeID, e := learnerEpisode(ctx, tx, c.LearnerUserID, c.ClassID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "credential-command:"+actor.ID+":"+key); e != nil {
			return e
		}
		prior, found, e := credentialReplay(ctx, tx, actor.ID, key, "RELEASE_TRANSCRIPT", hash)
		if e != nil {
			return e
		}
		if found {
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "transcript:"+c.ClassID+":"+episodeID); e != nil {
			return e
		}
		items, sourceHash, e := gradeSnapshot(ctx, tx, c.ClassID, c.LearnerUserID, episodeID)
		if e != nil {
			return e
		}
		if len(items) == 0 {
			return ErrConflict
		}
		var version int64
		if e = tx.QueryRow(ctx, `SELECT coalesce(max(version),0)+1 FROM v3.transcript_releases WHERE class_id=$1 AND episode_id=$2`, c.ClassID, episodeID).Scan(&version); e != nil {
			return e
		}
		id, e := token(16)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO v3.transcript_releases(id,class_id,enrollment_id,episode_id,learner_user_id,version,policy_ref,grade_source_hash,released_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.ClassID, enrollmentID, episodeID, c.LearnerUserID, version, transcriptPolicy, sourceHash, actor.ID)
		if e != nil {
			return mapWriteError(e)
		}
		for i := range items {
			items[i].ID, e = token(16)
			if e != nil {
				return e
			}
			item := items[i]
			_, e = tx.Exec(ctx, `INSERT INTO v3.transcript_grade_items(id,transcript_id,class_id,learner_user_id,grade_item_id,grade_release_id,
			 grade_revision_id,definition_version_id,scale_id,raw_value_milli,scale_minimum_milli,scale_maximum_milli,weight_bps)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
				item.ID, id, c.ClassID, c.LearnerUserID, item.GradeItemID, item.GradeReleaseID, item.GradeRevisionID,
				item.DefinitionVersionID, item.ScaleID, item.RawValueMilli, item.ScaleMinimumMilli, item.ScaleMaximumMilli, item.WeightBps)
			if e != nil {
				return mapWriteError(e)
			}
		}
		result = Transcript{ID: id, ClassID: c.ClassID, LearnerUserID: c.LearnerUserID, EnrollmentID: enrollmentID, EpisodeID: episodeID,
			Version: version, PolicyRef: transcriptPolicy, GradeSourceHash: sourceHash, Items: items}
		if e = audit(ctx, tx, actor.ID, c.ClassID, "transcript.release", requestID); e != nil {
			return e
		}
		return saveCredentialCommand(ctx, tx, actor.ID, key, "RELEASE_TRANSCRIPT", hash, result)
	})
	return result, replayed, mapWriteError(err)
}

func (s *Store) ReadTranscript(ctx context.Context, session, classID, learnerID string) (Transcript, error) {
	var result Transcript
	if classID == "" || learnerID == "" {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		if actor.ID != learnerID {
			allowed, authErr := teacher(ctx, tx, actor, classID)
			if authErr != nil {
				return authErr
			}
			if !allowed {
				return ErrForbidden
			}
		}
		_, episodeID, e := learnerEpisode(ctx, tx, learnerID, classID)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT id,class_id,learner_user_id,enrollment_id,episode_id,version,policy_ref,grade_source_hash
		 FROM v3.transcript_releases WHERE class_id=$1 AND episode_id=$2 ORDER BY version DESC LIMIT 1`, classID, episodeID).
			Scan(&result.ID, &result.ClassID, &result.LearnerUserID, &result.EnrollmentID, &result.EpisodeID, &result.Version, &result.PolicyRef, &result.GradeSourceHash)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id,grade_item_id,grade_release_id,grade_revision_id,definition_version_id,scale_id,
		 raw_value_milli,scale_minimum_milli,scale_maximum_milli,weight_bps FROM v3.transcript_grade_items WHERE transcript_id=$1 ORDER BY grade_item_id,grade_release_id`, result.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		result.Items = []TranscriptItem{}
		for rows.Next() {
			var item TranscriptItem
			if e = rows.Scan(&item.ID, &item.GradeItemID, &item.GradeReleaseID, &item.GradeRevisionID, &item.DefinitionVersionID,
				&item.ScaleID, &item.RawValueMilli, &item.ScaleMinimumMilli, &item.ScaleMaximumMilli, &item.WeightBps); e != nil {
				return e
			}
			result.Items = append(result.Items, item)
		}
		return rows.Err()
	})
	return result, err
}
