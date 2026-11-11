package completion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

// These are untrusted observations of legacy output, never commands to issue
// canonical completion, transcripts, credentials, or outbound messages.
type LegacyOutputRow struct {
	SourceTable            string          `json:"source_table"`
	SourcePK               string          `json:"source_pk"`
	SourceVersion          string          `json:"source_version"`
	RawPayload             json.RawMessage `json:"raw_payload"`
	ObservedState          string          `json:"observed_state"`
	CandidateClassID       string          `json:"candidate_class_id,omitempty"`
	CandidateLearnerUserID string          `json:"candidate_learner_user_id,omitempty"`
}

type LegacyOutputCommand struct {
	SourceInstanceID     string            `json:"source_instance_id"`
	SourceSnapshotSHA256 string            `json:"source_snapshot_sha256"`
	Rows                 []LegacyOutputRow `json:"rows"`
}

type LegacyOutputObservation struct {
	ID                     string `json:"id"`
	SourceTable            string `json:"source_table"`
	SourcePK               string `json:"source_pk"`
	SourceVersion          string `json:"source_version"`
	RawSHA256              string `json:"raw_sha256"`
	ObservedState          string `json:"observed_state"`
	Disposition            string `json:"disposition"`
	CandidateClassID       string `json:"candidate_class_id,omitempty"`
	CandidateLearnerUserID string `json:"candidate_learner_user_id,omitempty"`
}

type LegacyOutputRun struct {
	ID                   string                    `json:"id"`
	SourceInstanceID     string                    `json:"source_instance_id"`
	SourceSnapshotSHA256 string                    `json:"source_snapshot_sha256"`
	Observations         []LegacyOutputObservation `json:"observations"`
	CreatedAt            time.Time                 `json:"created_at"`
}

func validLegacyOutputText(value string, min, max int) bool {
	return len(value) >= min && len(value) <= max && strings.TrimSpace(value) == value
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func normalizeLegacyOutput(command LegacyOutputCommand) (LegacyOutputCommand, error) {
	if !validLegacyOutputText(command.SourceInstanceID, 1, 128) || !validSHA256(command.SourceSnapshotSHA256) || len(command.Rows) < 1 || len(command.Rows) > 100 {
		return command, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range command.Rows {
		r := &command.Rows[i]
		r.SourceTable = strings.ToLower(strings.TrimSpace(r.SourceTable))
		r.ObservedState = strings.ToUpper(strings.TrimSpace(r.ObservedState))
		if r.SourceTable != "completion_projection" && r.SourceTable != "transcript_projection" && r.SourceTable != "certificate_projection" {
			return command, ErrInvalid
		}
		if !validLegacyOutputText(r.SourcePK, 1, 160) || !validLegacyOutputText(r.SourceVersion, 1, 128) ||
			(r.ObservedState != "TRUE" && r.ObservedState != "FALSE" && r.ObservedState != "UNKNOWN" && r.ObservedState != "INVALID") ||
			(r.CandidateClassID == "") != (r.CandidateLearnerUserID == "") || len(r.RawPayload) == 0 || len(r.RawPayload) > 8192 || !json.Valid(r.RawPayload) {
			return command, ErrInvalid
		}
		identity := r.SourceTable + "\x00" + r.SourcePK + "\x00" + r.SourceVersion
		if seen[identity] {
			return command, ErrInvalid
		}
		seen[identity] = true
		var compact bytes.Buffer
		if err := json.Compact(&compact, r.RawPayload); err != nil {
			return command, ErrInvalid
		}
		r.RawPayload = json.RawMessage(compact.Bytes())
	}
	return command, nil
}

func (s *Store) ObserveLegacyOutput(ctx context.Context, session, key, requestID string, command LegacyOutputCommand) (LegacyOutputRun, bool, error) {
	var result LegacyOutputRun
	if !validIdempotency(key, requestID) {
		return result, false, ErrInvalid
	}
	command, err := normalizeLegacyOutput(command)
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
		if !actor.Admin {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "legacy-completion:"+actor.ID+":"+key); e != nil {
			return e
		}
		var priorHash string
		var prior json.RawMessage
		e = tx.QueryRow(ctx, `SELECT request_hash,response FROM v3.legacy_completion_runs WHERE actor_id=$1 AND key=$2`, actor.ID, key).Scan(&priorHash, &prior)
		if e == nil {
			if priorHash != requestHash {
				return ErrConflict
			}
			replayed = true
			return json.Unmarshal(prior, &result)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		result.ID, e = token(16)
		if e != nil {
			return e
		}
		result.SourceInstanceID, result.SourceSnapshotSHA256 = command.SourceInstanceID, command.SourceSnapshotSHA256
		result.CreatedAt = time.Now().UTC()
		result.Observations = make([]LegacyOutputObservation, 0, len(command.Rows))
		for _, row := range command.Rows {
			if row.CandidateClassID != "" {
				var exists bool
				e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments WHERE class_id=$1 AND user_id=$2)`, row.CandidateClassID, row.CandidateLearnerUserID).Scan(&exists)
				if e != nil {
					return e
				}
				if !exists {
					return ErrConflict
				}
			}
			id, tokenErr := token(16)
			if tokenErr != nil {
				return tokenErr
			}
			disposition := "OBSERVED_UNVERIFIED"
			if row.ObservedState == "UNKNOWN" || row.ObservedState == "INVALID" {
				disposition = "UNKNOWN_HOLD"
			}
			result.Observations = append(result.Observations, LegacyOutputObservation{
				ID: id, SourceTable: row.SourceTable, SourcePK: row.SourcePK, SourceVersion: row.SourceVersion,
				RawSHA256: digest(row.RawPayload), ObservedState: row.ObservedState, Disposition: disposition,
				CandidateClassID: row.CandidateClassID, CandidateLearnerUserID: row.CandidateLearnerUserID,
			})
		}
		response, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		_, e = tx.Exec(ctx, `INSERT INTO v3.legacy_completion_runs(id,actor_id,key,request_hash,source_instance_id,source_snapshot_sha256,response)
		 VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, result.ID, actor.ID, key, requestHash, command.SourceInstanceID, command.SourceSnapshotSHA256, string(response))
		if e != nil {
			return mapWriteError(e)
		}
		for i, row := range command.Rows {
			observation := result.Observations[i]
			_, e = tx.Exec(ctx, `INSERT INTO v3.legacy_completion_observations(id,run_id,ordinal,source_instance_id,source_table,source_pk,source_version,
			 raw_payload,raw_sha256,observed_state,disposition,candidate_class_id,candidate_learner_user_id)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,NULLIF($12,''),NULLIF($13,''))`,
				observation.ID, result.ID, i+1, command.SourceInstanceID, row.SourceTable, row.SourcePK, row.SourceVersion,
				string(row.RawPayload), observation.RawSHA256, row.ObservedState, observation.Disposition, row.CandidateClassID, row.CandidateLearnerUserID)
			if e != nil {
				return mapWriteError(e)
			}
		}
		return nil
	})
	return result, replayed, mapWriteError(err)
}
