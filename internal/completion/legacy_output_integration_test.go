//go:build qaintegration

package completion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"lmsv3/internal/labdb"
)

func openLegacyOutputFixture(t *testing.T) *completionFixture {
	t.Helper()
	f := openCredentialFixture(t)
	ctx := context.Background()
	if err := labdb.InstallLegacyCompletionObservation(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallLegacyCompletionObservation(ctx, f.owner); err != nil {
		t.Fatal("legacy output migration replay", err)
	}
	if _, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.legacy_completion_runs,v3.legacy_completion_observations TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
	return f
}

func legacyCommand(instance string, v completionContext, learner string) LegacyOutputCommand {
	return LegacyOutputCommand{
		SourceInstanceID: instance, SourceSnapshotSHA256: strings.Repeat("a", 64),
		Rows: []LegacyOutputRow{{
			SourceTable: "certificate_projection", SourcePK: "legacy-cert-1", SourceVersion: "1",
			RawPayload: json.RawMessage(`{"certificate_visible":true}`), ObservedState: "TRUE",
			CandidateClassID: v.classID, CandidateLearnerUserID: learner,
		}},
	}
}

func TestSyntheticLegacyOutputObservedOnly(t *testing.T) {
	f := openLegacyOutputFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "legacyobserved"+unique, true)
	ctx := context.Background()
	var beforeRecords, beforeTranscripts, beforeOutcomes, beforeOutbox int
	if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.credential_records),
	 (SELECT count(*) FROM v3.transcript_releases),(SELECT count(*) FROM v3.activity_completion_outcomes),(SELECT count(*) FROM v3.outbox)`).
		Scan(&beforeRecords, &beforeTranscripts, &beforeOutcomes, &beforeOutbox); err != nil {
		t.Fatal(err)
	}
	run, replayed, err := f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "observed"+unique, "observed-request", legacyCommand("source"+unique, v, f.users["student"]))
	if err != nil || replayed || len(run.Observations) != 1 || run.Observations[0].Disposition != "OBSERVED_UNVERIFIED" {
		t.Fatal(run, replayed, err)
	}
	var raw json.RawMessage
	var records, transcripts, outcomes, afterOutbox int
	if err = f.owner.QueryRow(ctx, `SELECT raw_payload FROM v3.legacy_completion_observations WHERE id=$1`, run.Observations[0].ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"certificate_visible": true}` && string(raw) != `{"certificate_visible":true}` {
		t.Fatal("source payload not retained", string(raw))
	}
	if err = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.credential_records),
	 (SELECT count(*) FROM v3.transcript_releases),(SELECT count(*) FROM v3.activity_completion_outcomes),(SELECT count(*) FROM v3.outbox)`).
		Scan(&records, &transcripts, &outcomes, &afterOutbox); err != nil || records != beforeRecords || transcripts != beforeTranscripts || outcomes != beforeOutcomes || afterOutbox != beforeOutbox {
		t.Fatal("historical observation caused canonical or outbound mutation", records, transcripts, outcomes, afterOutbox, err)
	}
}

func TestSyntheticLegacyOutputIdempotencyBoundary(t *testing.T) {
	f := openLegacyOutputFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "legacyretry"+unique, false)
	command := legacyCommand("source"+unique, v, f.users["student"])
	command.Rows = append(command.Rows, LegacyOutputRow{
		SourceTable: "completion_projection", SourcePK: "unknown-row", SourceVersion: "1",
		RawPayload: json.RawMessage(`{"legacy_value":null}`), ObservedState: "UNKNOWN",
	})
	ctx := context.Background()
	first, replayed, err := f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "legacy-key"+unique, "legacy-request", command)
	if err != nil || replayed || len(first.Observations) != 2 || first.Observations[1].Disposition != "UNKNOWN_HOLD" {
		t.Fatal(first, replayed, err)
	}
	second, replayed, err := f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "legacy-key"+unique, "retry-request", command)
	if err != nil || !replayed || first.ID != second.ID {
		t.Fatal(second, replayed, err)
	}
	changed := command
	changed.Rows = append([]LegacyOutputRow(nil), command.Rows...)
	changed.Rows[0].ObservedState = "FALSE"
	if _, _, err = f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "legacy-key"+unique, "changed-request", changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	if _, _, err = f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "other-key"+unique, "duplicate-source", command); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate source accepted", err)
	}
	if _, _, err = f.store.ObserveLegacyOutput(ctx, f.tokens["student"], "student-key"+unique, "student-request", command); !errors.Is(err, ErrForbidden) {
		t.Fatal("non-admin observed legacy state", err)
	}
	foreign := f.learningContext(t, "legacyforeign"+unique, false)
	wrong := legacyCommand("wrong"+unique, foreign, f.users["outsider"])
	if _, _, err = f.store.ObserveLegacyOutput(ctx, f.tokens["admin"], "wrong-key"+unique, "wrong-request", wrong); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign candidate accepted", err)
	}
	var runs, rows int
	if err = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.legacy_completion_runs WHERE source_instance_id=$1),
	 (SELECT count(*) FROM v3.legacy_completion_observations WHERE source_instance_id=$1)`, command.SourceInstanceID).Scan(&runs, &rows); err != nil || runs != 1 || rows != 2 {
		t.Fatal("rollback/idempotency failure", runs, rows, err)
	}
}

func TestSyntheticLegacyOutputCorrectionStability(t *testing.T) {
	f := openLegacyOutputFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "legacycorrection"+unique, true)
	f.completeActivity(t, v)
	oldTranscript := f.releaseTranscript(t, v)
	issued, _, err := f.store.Issue(context.Background(), f.tokens["teacher"], "issue"+unique, "issue-request",
		IssueCommand{ClassID: v.classID, LearnerUserID: f.users["student"], TranscriptID: oldTranscript.ID, Privacy: "PUBLIC_MINIMAL"})
	if err != nil {
		t.Fatal(err)
	}
	observed, _, err := f.store.ObserveLegacyOutput(context.Background(), f.tokens["admin"], "observed"+unique, "observed-request", legacyCommand("source"+unique, v, f.users["student"]))
	if err != nil {
		t.Fatal(err)
	}
	f.changeReleasedGrade(t, v)
	newTranscript := f.releaseTranscript(t, v)
	if newTranscript.GradeSourceHash == oldTranscript.GradeSourceHash {
		t.Fatal("grade correction not reflected in new transcript")
	}
	var oldScore int64
	var disposition string
	if err = f.owner.QueryRow(context.Background(), `SELECT raw_value_milli FROM v3.transcript_grade_items WHERE transcript_id=$1`, oldTranscript.ID).Scan(&oldScore); err != nil || oldScore != 100000 {
		t.Fatal("old transcript changed", oldScore, err)
	}
	if err = f.owner.QueryRow(context.Background(), `SELECT disposition FROM v3.legacy_completion_observations WHERE id=$1`, observed.Observations[0].ID).Scan(&disposition); err != nil || disposition != "OBSERVED_UNVERIFIED" {
		t.Fatal("observation changed", disposition, err)
	}
	verification, err := f.store.Verify(context.Background(), issued.ID)
	if err != nil || verification.Status != "VALID" {
		t.Fatal("public credential changed by correction", verification, err)
	}
}
