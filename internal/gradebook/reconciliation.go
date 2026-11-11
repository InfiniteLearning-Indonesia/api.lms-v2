package gradebook

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/platform/transaction"
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type HistoricalRow struct {
	SourceTable          string  `json:"source_table"`
	SourceRowID          string  `json:"source_row_id"`
	SourceScope          string  `json:"source_scope"`
	RawState             string  `json:"raw_state"`
	RawValueText         *string `json:"raw_value_text,omitempty"`
	RawScoreMilli        *int64  `json:"raw_score_milli,omitempty"`
	ObservedScoreMilli   *int64  `json:"observed_score_milli,omitempty"`
	Origin               string  `json:"origin"`
	CandidateClassID     string  `json:"candidate_class_id,omitempty"`
	CandidateGradeItemID string  `json:"candidate_grade_item_id,omitempty"`
	CandidateAggregateID string  `json:"candidate_aggregate_id,omitempty"`
	Resolution           string  `json:"resolution,omitempty"`
	Reason               string  `json:"reason,omitempty"`
}

type ReconciliationCommand struct {
	SourceSnapshotSHA256 string          `json:"source_snapshot_sha256"`
	PolicyReference      string          `json:"policy_reference"`
	Rows                 []HistoricalRow `json:"rows"`
}

type ReconciliationObservation struct {
	ID      string `json:"id"`
	Ordinal int    `json:"ordinal"`
	HistoricalRow
	CanonicalReleaseID  string `json:"canonical_release_id,omitempty"`
	CanonicalScoreMilli *int64 `json:"canonical_score_milli,omitempty"`
	Outcome             string `json:"outcome"`
	EffectiveReason     string `json:"effective_reason"`
}

type ReconciliationRun struct {
	ID                   string                      `json:"id"`
	SourceSnapshotSHA256 string                      `json:"source_snapshot_sha256"`
	PolicyReference      string                      `json:"policy_reference"`
	Observations         []ReconciliationObservation `json:"observations"`
	CreatedAt            time.Time                   `json:"created_at"`
}

func historicalAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeHistorical(c ReconciliationCommand) (ReconciliationCommand, error) {
	c.SourceSnapshotSHA256 = strings.ToLower(strings.TrimSpace(c.SourceSnapshotSHA256))
	c.PolicyReference = strings.TrimSpace(c.PolicyReference)
	if !sha256Pattern.MatchString(c.SourceSnapshotSHA256) || len(c.PolicyReference) < 3 || len(c.PolicyReference) > 160 || len(c.Rows) < 1 || len(c.Rows) > 100 {
		return c, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range c.Rows {
		r := &c.Rows[i]
		r.SourceTable = strings.ToLower(strings.TrimSpace(r.SourceTable))
		r.SourceRowID = strings.TrimSpace(r.SourceRowID)
		r.SourceScope = strings.ToUpper(strings.TrimSpace(r.SourceScope))
		r.RawState = strings.ToUpper(strings.TrimSpace(r.RawState))
		r.Origin = strings.ToUpper(strings.TrimSpace(r.Origin))
		r.Resolution = strings.ToUpper(strings.TrimSpace(r.Resolution))
		r.Reason = strings.TrimSpace(r.Reason)
		identity := r.SourceTable + ":" + r.SourceRowID
		if !historicalAllowed(r.SourceTable, "submissions", "competency_scores", "rubrik_assessment_scores", "transcript_projection", "ai_evaluation", "attendance_projection") || len(r.SourceRowID) < 1 || len(r.SourceRowID) > 160 || seen[identity] || !historicalAllowed(r.SourceScope, "CLASS", "PROGRAM_GLOBAL") || !historicalAllowed(r.RawState, "MISSING_ROW", "EXPLICIT_NULL", "VALUE", "INVALID") || !historicalAllowed(r.Origin, "RECORDED", "COMPUTED", "LEGACY_MINIMUM_65", "AI_RETURNED", "AI_FALLBACK_75") || !historicalAllowed(r.Resolution, "", "HOLD", "APPROVED_DIFFERENCE") || len(r.Reason) > 1000 || !utf8.ValidString(r.Reason) {
			return c, ErrInvalid
		}
		seen[identity] = true
		mapped := r.CandidateClassID != "" || r.CandidateGradeItemID != "" || r.CandidateAggregateID != ""
		if (r.SourceScope == "PROGRAM_GLOBAL" && mapped) || (r.SourceScope == "CLASS" && (r.CandidateClassID == "" || r.CandidateGradeItemID == "" || r.CandidateAggregateID == "")) {
			return c, ErrInvalid
		}
		if r.RawValueText != nil && (!utf8.ValidString(*r.RawValueText) || len(*r.RawValueText) > 128) {
			return c, ErrInvalid
		}
		switch r.RawState {
		case "MISSING_ROW":
			if r.RawValueText != nil || r.RawScoreMilli != nil {
				return c, ErrInvalid
			}
		case "EXPLICIT_NULL":
			if r.RawValueText == nil || *r.RawValueText != "null" || r.RawScoreMilli != nil {
				return c, ErrInvalid
			}
		case "VALUE":
			if r.RawValueText == nil || r.RawScoreMilli == nil {
				return c, ErrInvalid
			}
		case "INVALID":
			if r.RawValueText == nil || r.RawScoreMilli != nil {
				return c, ErrInvalid
			}
		}
		for _, score := range []*int64{r.RawScoreMilli, r.ObservedScoreMilli} {
			if score != nil && (*score < -1000000000 || *score > 1000000000) {
				return c, ErrInvalid
			}
		}
		if r.Resolution == "APPROVED_DIFFERENCE" && len(r.Reason) < 8 {
			return c, ErrInvalid
		}
	}
	return c, nil
}

func (s *Store) ReconcileHistorical(ctx context.Context, session, key, requestID string, command ReconciliationCommand) (json.RawMessage, bool, error) {
	if !validIdempotency(key, requestID) {
		return nil, false, ErrInvalid
	}
	c, err := normalizeHistorical(command)
	if err != nil {
		return nil, false, err
	}
	requestHash := digest(mustJSON(c))
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		if !actor.Admin {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":historical:"+key); e != nil {
			return e
		}
		var prior string
		e = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.legacy_grade_reconciliation_runs WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
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
		locks := map[string]bool{}
		for _, row := range c.Rows {
			if row.SourceScope == "CLASS" {
				locks[row.CandidateGradeItemID+":"+row.CandidateAggregateID] = true
			}
		}
		ordered := make([]string, 0, len(locks))
		for value := range locks {
			ordered = append(ordered, value)
		}
		sort.Strings(ordered)
		for _, value := range ordered {
			if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", value); e != nil {
				return e
			}
		}
		runID, genErr := token(16)
		if genErr != nil {
			return genErr
		}
		run := ReconciliationRun{ID: runID, SourceSnapshotSHA256: c.SourceSnapshotSHA256, PolicyReference: c.PolicyReference, Observations: make([]ReconciliationObservation, 0, len(c.Rows)), CreatedAt: time.Now().UTC()}
		for index, row := range c.Rows {
			observation := ReconciliationObservation{Ordinal: index + 1, HistoricalRow: row}
			observation.ID, genErr = token(16)
			if genErr != nil {
				return genErr
			}
			if row.SourceScope == "PROGRAM_GLOBAL" {
				observation.Outcome = "AMBIGUOUS_HOLD"
				observation.EffectiveReason = "PROGRAM_GLOBAL_WITHOUT_CLASS_IDENTITY"
			} else {
				var releaseID *string
				var canonical *int64
				e = tx.QueryRow(ctx, `SELECT latest.id,latest.value_milli FROM v3.grade_items gi JOIN v3.submission_aggregates sa ON sa.class_id=gi.class_id AND sa.activity_id=gi.activity_id
				 LEFT JOIN LATERAL (SELECT l.id,r.value_milli FROM v3.grade_releases l JOIN v3.grade_revisions r ON r.id=l.revision_id WHERE l.class_id=gi.class_id AND l.grade_item_id=gi.id AND l.aggregate_id=sa.id ORDER BY l.grade_version DESC LIMIT 1) latest ON true
				 WHERE gi.id=$1 AND gi.class_id=$2 AND sa.id=$3`, row.CandidateGradeItemID, row.CandidateClassID, row.CandidateAggregateID).Scan(&releaseID, &canonical)
				if errors.Is(e, pgx.ErrNoRows) {
					return ErrNotFound
				}
				if e != nil {
					return e
				}
				if releaseID != nil {
					observation.CanonicalReleaseID = *releaseID
				}
				observation.CanonicalScoreMilli = canonical
				switch {
				case row.Origin == "LEGACY_MINIMUM_65":
					if row.ObservedScoreMilli == nil || *row.ObservedScoreMilli != 65000 {
						return ErrInvalid
					}
					observation.Outcome = "POLICY_DERIVED_HOLD"
					observation.EffectiveReason = "LEGACY_MINIMUM_65_IS_NOT_RAW_GRADE"
				case row.Origin == "AI_FALLBACK_75":
					if row.ObservedScoreMilli == nil || *row.ObservedScoreMilli != 75000 {
						return ErrInvalid
					}
					observation.Outcome = "AI_UNREVIEWED_HOLD"
					observation.EffectiveReason = "AI_FALLBACK_75_REQUIRES_HUMAN_REVIEW"
				case releaseID == nil:
					observation.Outcome = "NO_CANONICAL_HOLD"
					observation.EffectiveReason = "NO_RELEASED_CANONICAL_GRADE"
				case row.RawState == "VALUE" && row.RawScoreMilli != nil && row.ObservedScoreMilli != nil && *row.RawScoreMilli == *canonical && *row.ObservedScoreMilli == *canonical:
					observation.Outcome = "MATCHED"
					observation.EffectiveReason = "EXACT_RELEASED_MATCH"
				case row.Resolution == "APPROVED_DIFFERENCE":
					observation.Outcome = "APPROVED_DIFFERENCE"
					observation.EffectiveReason = row.Reason
				default:
					observation.Outcome = "DIFFERENCE_HOLD"
					observation.EffectiveReason = "DIFFERENCE_REQUIRES_EXPLICIT_APPROVAL"
				}
			}
			if row.Resolution == "APPROVED_DIFFERENCE" && observation.Outcome != "APPROVED_DIFFERENCE" {
				return ErrInvalid
			}
			run.Observations = append(run.Observations, observation)
		}
		response = mustJSON(run)
		if _, e = tx.Exec(ctx, "INSERT INTO v3.legacy_grade_reconciliation_runs(id,actor_id,key,request_hash,source_snapshot_sha256,policy_reference,response) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)", run.ID, actor.ID, key, requestHash, c.SourceSnapshotSHA256, c.PolicyReference, string(response)); e != nil {
			return mapWriteError(e)
		}
		for _, observation := range run.Observations {
			var release any = nil
			if observation.CanonicalReleaseID != "" {
				release = observation.CanonicalReleaseID
			}
			_, e = tx.Exec(ctx, `INSERT INTO v3.legacy_grade_observations(id,run_id,ordinal,source_table,source_row_id,source_scope,raw_state,raw_value_text,raw_score_milli,observed_score_milli,origin,candidate_class_id,candidate_grade_item_id,candidate_aggregate_id,canonical_release_id,canonical_score_milli,outcome,reason)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),$15,$16,$17,$18)`, observation.ID, run.ID, observation.Ordinal, observation.SourceTable, observation.SourceRowID, observation.SourceScope, observation.RawState, observation.RawValueText, observation.RawScoreMilli, observation.ObservedScoreMilli, observation.Origin, observation.CandidateClassID, observation.CandidateGradeItemID, observation.CandidateAggregateID, release, observation.CanonicalScoreMilli, observation.Outcome, observation.EffectiveReason)
			if e != nil {
				return mapWriteError(e)
			}
		}
		auditID, genErr := token(16)
		if genErr != nil {
			return genErr
		}
		if _, e = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,NULL,'gradebook.historical.reconcile',$3)", auditID, actor.ID, requestID); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", auditID)
		return e
	})
	return response, replayed, mapWriteError(err)
}
