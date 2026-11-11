//go:build qaintegration

package gradebook

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type historicalFixture struct {
	f          *gradeFixture
	work       gradeContext
	definition Definition
}

func ptrInt64(value int64) *int64    { return &value }
func ptrString(value string) *string { return &value }

func openHistoricalFixture(t *testing.T, suffix string, release bool) historicalFixture {
	t.Helper()
	f := openGradeFixture(t, "m09-cp04")
	unique, _ := token(4)
	work := f.submission(t, suffix+unique, "student", false)
	definition := createPublishedDefinition(t, f, work)
	grade := GradeCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, SubmissionRevisionID: work.revisionID, ExpectedVersion: 0, ValueMilli: 80000}
	if _, _, err := f.store.Grade(context.Background(), f.tokens["teacher"], "hist-grade-"+unique, "hist-grade-request-"+unique, grade); err != nil {
		t.Fatal(err)
	}
	if release {
		if _, _, err := f.store.ReleaseGrade(context.Background(), f.tokens["teacher"], "hist-release-"+unique, "hist-release-request-"+unique, ReleaseCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
	}
	return historicalFixture{f: f, work: work, definition: definition}
}

func mappedHistorical(h historicalFixture, sourceRow string) HistoricalRow {
	return HistoricalRow{SourceTable: "submissions", SourceRowID: sourceRow, SourceScope: "CLASS", RawState: "VALUE", RawValueText: ptrString("80"), RawScoreMilli: ptrInt64(80000), ObservedScoreMilli: ptrInt64(80000), Origin: "RECORDED", CandidateClassID: h.work.classID, CandidateGradeItemID: h.definition.GradeItemID, CandidateAggregateID: h.work.aggregateID}
}

func reconcile(t *testing.T, h historicalFixture, subject, key string, rows []HistoricalRow) (ReconciliationRun, bool, error) {
	t.Helper()
	raw, replayed, err := h.f.store.ReconcileHistorical(context.Background(), h.f.tokens[subject], key, "request-"+key, ReconciliationCommand{SourceSnapshotSHA256: digest([]byte("snapshot-" + key)), PolicyReference: "SYNTHETIC-M09-CP04-V1", Rows: rows})
	var run ReconciliationRun
	if err == nil && json.Unmarshal(raw, &run) != nil {
		t.Fatal("decode reconciliation")
	}
	return run, replayed, err
}

func TestSyntheticHistoricalSemantics(t *testing.T) {
	h := openHistoricalFixture(t, "hs", true)
	rows := []HistoricalRow{
		{SourceTable: "transcript_projection", SourceRowID: "missing", SourceScope: "CLASS", RawState: "MISSING_ROW", ObservedScoreMilli: ptrInt64(65000), Origin: "LEGACY_MINIMUM_65", CandidateClassID: h.work.classID, CandidateGradeItemID: h.definition.GradeItemID, CandidateAggregateID: h.work.aggregateID},
		{SourceTable: "transcript_projection", SourceRowID: "null", SourceScope: "CLASS", RawState: "EXPLICIT_NULL", RawValueText: ptrString("null"), ObservedScoreMilli: ptrInt64(65000), Origin: "LEGACY_MINIMUM_65", CandidateClassID: h.work.classID, CandidateGradeItemID: h.definition.GradeItemID, CandidateAggregateID: h.work.aggregateID},
		{SourceTable: "submissions", SourceRowID: "zero", SourceScope: "CLASS", RawState: "VALUE", RawValueText: ptrString("0"), RawScoreMilli: ptrInt64(0), ObservedScoreMilli: ptrInt64(0), Origin: "RECORDED", CandidateClassID: h.work.classID, CandidateGradeItemID: h.definition.GradeItemID, CandidateAggregateID: h.work.aggregateID},
		{SourceTable: "ai_evaluation", SourceRowID: "fallback75", SourceScope: "CLASS", RawState: "INVALID", RawValueText: ptrString("provider_error"), ObservedScoreMilli: ptrInt64(75000), Origin: "AI_FALLBACK_75", CandidateClassID: h.work.classID, CandidateGradeItemID: h.definition.GradeItemID, CandidateAggregateID: h.work.aggregateID},
		{SourceTable: "competency_scores", SourceRowID: "global", SourceScope: "PROGRAM_GLOBAL", RawState: "VALUE", RawValueText: ptrString("90"), RawScoreMilli: ptrInt64(90000), ObservedScoreMilli: ptrInt64(90000), Origin: "RECORDED"},
	}
	run, replayed, err := reconcile(t, h, "admin", "historical-semantics", rows)
	if err != nil || replayed || len(run.Observations) != 5 {
		t.Fatal(run, replayed, err)
	}
	want := []string{"POLICY_DERIVED_HOLD", "POLICY_DERIVED_HOLD", "DIFFERENCE_HOLD", "AI_UNREVIEWED_HOLD", "AMBIGUOUS_HOLD"}
	for i, value := range want {
		if run.Observations[i].Outcome != value {
			t.Fatal(i, run.Observations[i])
		}
	}
	var missing, nulls, zero, ai, global int
	err = h.f.owner.QueryRow(context.Background(), `SELECT
	 count(*) FILTER(WHERE raw_state='MISSING_ROW' AND raw_score_milli IS NULL),count(*) FILTER(WHERE raw_state='EXPLICIT_NULL' AND raw_value_text='null' AND raw_score_milli IS NULL),
	 count(*) FILTER(WHERE raw_state='VALUE' AND raw_value_text='0' AND raw_score_milli=0),count(*) FILTER(WHERE origin='AI_FALLBACK_75' AND observed_score_milli=75000),
	 count(*) FILTER(WHERE source_scope='PROGRAM_GLOBAL' AND candidate_class_id IS NULL AND outcome='AMBIGUOUS_HOLD') FROM v3.legacy_grade_observations WHERE run_id=$1`, run.ID).Scan(&missing, &nulls, &zero, &ai, &global)
	if err != nil || missing != 1 || nulls != 1 || zero != 1 || ai != 1 || global != 1 {
		t.Fatal(missing, nulls, zero, ai, global, err)
	}
}

func TestSyntheticHistoricalScopedMatch(t *testing.T) {
	h := openHistoricalFixture(t, "hm", true)
	match := mappedHistorical(h, "match")
	difference := mappedHistorical(h, "approved")
	difference.RawValueText = ptrString("70")
	difference.RawScoreMilli = ptrInt64(70000)
	difference.ObservedScoreMilli = ptrInt64(70000)
	difference.Resolution = "APPROVED_DIFFERENCE"
	difference.Reason = "Academic review accepted preserved legacy outcome"
	run, replayed, err := reconcile(t, h, "admin", "historical-match", []HistoricalRow{match, difference})
	if err != nil || replayed || run.Observations[0].Outcome != "MATCHED" || run.Observations[1].Outcome != "APPROVED_DIFFERENCE" || run.Observations[0].CanonicalScoreMilli == nil || *run.Observations[0].CanonicalScoreMilli != 80000 {
		t.Fatal(run, replayed, err)
	}
	again, replayed, err := reconcile(t, h, "admin", "historical-match", []HistoricalRow{match, difference})
	if err != nil || !replayed || again.ID != run.ID {
		t.Fatal(again, replayed, err)
	}
	unreleased := openHistoricalFixture(t, "hu", false)
	row := mappedHistorical(unreleased, "unreleased")
	unreleasedRun, _, err := reconcile(t, unreleased, "admin", "historical-unreleased", []HistoricalRow{row})
	if err != nil || unreleasedRun.Observations[0].Outcome != "NO_CANONICAL_HOLD" || unreleasedRun.Observations[0].CanonicalReleaseID != "" {
		t.Fatal(unreleasedRun, err)
	}
	var grades, releases int
	err = h.f.owner.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM v3.grade_revisions WHERE class_id=$1),(SELECT count(*) FROM v3.grade_releases WHERE class_id=$1)", h.work.classID).Scan(&grades, &releases)
	if err != nil || grades != 1 || releases != 1 {
		t.Fatal("canonical mutation", grades, releases, err)
	}
}

func TestSyntheticHistoricalBoundary(t *testing.T) {
	h := openHistoricalFixture(t, "hb", true)
	valid := mappedHistorical(h, "valid")
	if _, _, err := reconcile(t, h, "teacher", "historical-teacher", []HistoricalRow{valid}); !errors.Is(err, ErrForbidden) {
		t.Fatal("teacher reconciliation", err)
	}
	duplicate := valid
	duplicate.SourceRowID = valid.SourceRowID
	if _, _, err := reconcile(t, h, "admin", "historical-duplicate", []HistoricalRow{valid, duplicate}); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate source", err)
	}
	global := HistoricalRow{SourceTable: "competency_scores", SourceRowID: "global-approval", SourceScope: "PROGRAM_GLOBAL", RawState: "VALUE", RawValueText: ptrString("90"), RawScoreMilli: ptrInt64(90000), ObservedScoreMilli: ptrInt64(90000), Origin: "RECORDED", Resolution: "APPROVED_DIFFERENCE", Reason: "Forged approval for ambiguous global row"}
	if _, _, err := reconcile(t, h, "admin", "historical-global", []HistoricalRow{global}); !errors.Is(err, ErrInvalid) {
		t.Fatal("global approval", err)
	}
	fallback := valid
	fallback.SourceRowID = "fallback-approval"
	fallback.RawState = "INVALID"
	fallback.RawValueText = ptrString("provider_error")
	fallback.RawScoreMilli = nil
	fallback.ObservedScoreMilli = ptrInt64(75000)
	fallback.Origin = "AI_FALLBACK_75"
	fallback.Resolution = "APPROVED_DIFFERENCE"
	fallback.Reason = "Attempted fallback approval must be rejected"
	if _, _, err := reconcile(t, h, "admin", "historical-fallback", []HistoricalRow{fallback}); !errors.Is(err, ErrInvalid) {
		t.Fatal("fallback approval", err)
	}
	foreign := valid
	foreign.SourceRowID = "foreign"
	foreign.CandidateAggregateID = "missing-aggregate"
	if _, _, err := reconcile(t, h, "admin", "historical-foreign", []HistoricalRow{valid, foreign}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign ancestry", err)
	}
	var partial int
	if err := h.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM v3.legacy_grade_reconciliation_runs WHERE key='historical-foreign'").Scan(&partial); err != nil || partial != 0 {
		t.Fatal("partial run", partial, err)
	}
	run, _, err := reconcile(t, h, "admin", "historical-stable", []HistoricalRow{valid})
	if err != nil {
		t.Fatal(err)
	}
	changed := valid
	changed.ObservedScoreMilli = ptrInt64(79000)
	if _, _, err = reconcile(t, h, "admin", "historical-stable", []HistoricalRow{changed}); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
	if _, err = h.f.owner.Exec(context.Background(), "UPDATE v3.legacy_grade_observations SET reason='changed' WHERE run_id=$1", run.ID); err == nil {
		t.Fatal("immutable ledger updated")
	}
}
