//go:build qaintegration

package completion

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/labdb"
)

func openCredentialFixture(t *testing.T) *completionFixture {
	t.Helper()
	f := openCorrectionFixture(t)
	ctx := context.Background()
	if err := labdb.InstallTranscriptCredential(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallTranscriptCredential(ctx, f.owner); err != nil {
		t.Fatal("credential replay", err)
	}
	if _, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.transcript_releases,v3.transcript_grade_items,v3.credential_records,v3.credential_events,v3.credential_commands TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *completionFixture) releaseTranscript(t *testing.T, v completionContext) Transcript {
	t.Helper()
	key, _ := token(12)
	result, replayed, err := f.store.ReleaseTranscript(context.Background(), f.tokens["teacher"], key, "transcript-request"+key,
		TranscriptCommand{ClassID: v.classID, LearnerUserID: f.users["student"]})
	if err != nil || replayed {
		t.Fatal(result, replayed, err)
	}
	return result
}

func (f *completionFixture) completeActivity(t *testing.T, v completionContext) {
	t.Helper()
	rule := f.createRule(t, v, 0)
	key, _ := token(12)
	result, _, err := f.store.Evaluate(context.Background(), f.tokens["student"], key, "completion-request"+key,
		EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true})
	if err != nil || result.State != "COMPLETED" {
		t.Fatal(result, err)
	}
}

func (f *completionFixture) changeReleasedGrade(t *testing.T, v completionContext) {
	t.Helper()
	ctx := context.Background()
	newID, _ := token(16)
	releaseID, _ := token(16)
	_, err := f.owner.Exec(ctx, `INSERT INTO v3.grade_revisions(id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,
	 learner_user_id,grader_user_id,version,value_milli,reason)
	 SELECT $1,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,
	 learner_user_id,grader_user_id,2,90000,'CORRECTION' FROM v3.grade_revisions WHERE class_id=$2 AND version=1;
	 UPDATE v3.grade_current SET revision_id=$1,version=2,updated_at=clock_timestamp() WHERE class_id=$2;
	 INSERT INTO v3.grade_releases(id,class_id,grade_item_id,aggregate_id,revision_id,grade_version,learner_user_id,actor_id,key,request_hash)
	 SELECT $3,r.class_id,r.grade_item_id,r.aggregate_id,r.id,2,r.learner_user_id,r.grader_user_id,$4,$5 FROM v3.grade_revisions r WHERE r.id=$1`,
		pgx.QueryExecModeSimpleProtocol, newID, v.classID, releaseID, "corrected-"+newID, digest([]byte(newID)))
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyntheticTranscriptProvenance(t *testing.T) {
	f := openCredentialFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "transcript"+unique, true)
	if _, _, err := f.store.ReleaseTranscript(context.Background(), f.tokens["student"], "student"+unique, "student-request", TranscriptCommand{ClassID: v.classID, LearnerUserID: f.users["student"]}); !errors.Is(err, ErrForbidden) {
		t.Fatal("student released transcript", err)
	}
	first := f.releaseTranscript(t, v)
	if first.Version != 1 || first.PolicyRef != transcriptPolicy || len(first.Items) != 1 || first.Items[0].RawValueMilli != 100000 ||
		first.Items[0].ScaleMinimumMilli != 0 || first.Items[0].ScaleMaximumMilli != 100000 || first.Items[0].GradeReleaseID == "" {
		t.Fatal(first)
	}
	f.changeReleasedGrade(t, v)
	second := f.releaseTranscript(t, v)
	if second.Version != 2 || second.Items[0].RawValueMilli != 90000 || first.GradeSourceHash == second.GradeSourceHash {
		t.Fatal(first, second)
	}
	var oldValue int64
	if err := f.owner.QueryRow(context.Background(), `SELECT raw_value_milli FROM v3.transcript_grade_items WHERE transcript_id=$1`, first.ID).Scan(&oldValue); err != nil || oldValue != 100000 {
		t.Fatal("old transcript mutated", oldValue, err)
	}
	if _, err := f.owner.Exec(context.Background(), `UPDATE v3.transcript_grade_items SET raw_value_milli=0 WHERE transcript_id=$1`, first.ID); err == nil {
		t.Fatal("transcript item mutable")
	}
	foreign := f.learningContext(t, "foreigntranscript"+unique, true)
	if _, _, err := f.store.ReleaseTranscript(context.Background(), f.tokens["outsider"], "foreign"+unique, "foreign-request", TranscriptCommand{ClassID: foreign.classID, LearnerUserID: f.users["student"]}); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign actor released transcript", err)
	}
}

func TestSyntheticCredentialLifecycle(t *testing.T) {
	f := openCredentialFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "credential"+unique, true)
	f.completeActivity(t, v)
	firstTranscript := f.releaseTranscript(t, v)
	command := IssueCommand{ClassID: v.classID, LearnerUserID: f.users["student"], TranscriptID: firstTranscript.ID, Privacy: "PUBLIC_MINIMAL"}
	issued, replayed, err := f.store.Issue(context.Background(), f.tokens["teacher"], "issue"+unique, "issue-request", command)
	if err != nil || replayed || issued.Version != 1 || issued.Status != "VALID" {
		t.Fatal(issued, replayed, err)
	}
	repeat, replayed, err := f.store.Issue(context.Background(), f.tokens["teacher"], "issue"+unique, "other-request", command)
	if err != nil || !replayed || repeat.ID != issued.ID {
		t.Fatal(repeat, replayed, err)
	}
	public, err := f.store.Verify(context.Background(), issued.ID)
	if err != nil || public.Status != "VALID" {
		t.Fatal(public, err)
	}
	privateTranscript := f.releaseTranscript(t, v)
	private, _, err := f.store.Issue(context.Background(), f.tokens["teacher"], "private"+unique, "private-request",
		IssueCommand{ClassID: v.classID, LearnerUserID: f.users["student"], TranscriptID: privateTranscript.ID, Privacy: "PRIVATE"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Verify(context.Background(), private.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("private verify leaked", err)
	}
	revoked, _, err := f.store.Revoke(context.Background(), f.tokens["teacher"], "revoke"+unique, "revoke-request",
		TransitionCommand{CredentialID: private.ID, ExpectedVersion: 1, Reason: "Credential withdrawn after review"})
	if err != nil || revoked.Status != "REVOKED" || revoked.Version != 2 {
		t.Fatal(revoked, err)
	}
	successorTranscript := f.releaseTranscript(t, v)
	superseded, _, err := f.store.Supersede(context.Background(), f.tokens["teacher"], "supersede"+unique, "supersede-request",
		TransitionCommand{CredentialID: issued.ID, ExpectedVersion: 1, Reason: "Reissued with later transcript", SuccessorTranscriptID: successorTranscript.ID, Privacy: "PUBLIC_MINIMAL"})
	if err != nil || superseded.Status != "SUPERSEDED" || superseded.SuccessorID == "" {
		t.Fatal(superseded, err)
	}
	oldVerify, oldErr := f.store.Verify(context.Background(), issued.ID)
	newVerify, newErr := f.store.Verify(context.Background(), superseded.SuccessorID)
	if oldErr != nil || newErr != nil || oldVerify.Status != "SUPERSEDED" || newVerify.Status != "VALID" {
		t.Fatal(oldVerify, newVerify, oldErr, newErr)
	}
	var records, events int
	if err = f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.credential_records WHERE class_id=$1),
	 (SELECT count(*) FROM v3.credential_events e JOIN v3.credential_records c ON c.id=e.credential_id WHERE c.class_id=$1)`, v.classID).Scan(&records, &events); err != nil || records != 3 || events != 5 {
		t.Fatal(records, events, err)
	}
}

func TestSyntheticCredentialBoundary(t *testing.T) {
	f := openCredentialFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "credboundary"+unique, true)
	transcript := f.releaseTranscript(t, v)
	issue := IssueCommand{ClassID: v.classID, LearnerUserID: f.users["student"], TranscriptID: transcript.ID, Privacy: "PUBLIC_MINIMAL"}
	if _, _, err := f.store.Issue(context.Background(), f.tokens["teacher"], "incomplete"+unique, "incomplete-request", issue); !errors.Is(err, ErrConflict) {
		t.Fatal("incomplete issued", err)
	}
	f.completeActivity(t, v)
	if _, _, err := f.store.Issue(context.Background(), f.tokens["student"], "self"+unique, "self-request", issue); !errors.Is(err, ErrForbidden) {
		t.Fatal("self issued", err)
	}
	foreign := f.learningContext(t, "foreigncred"+unique, true)
	f.completeActivity(t, foreign)
	foreignTranscript := f.releaseTranscript(t, foreign)
	issue.TranscriptID = foreignTranscript.ID
	if _, _, err := f.store.Issue(context.Background(), f.tokens["teacher"], "foreign"+unique, "foreign-request", issue); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign transcript issued", err)
	}
	issue.TranscriptID = transcript.ID
	f.changeReleasedGrade(t, v)
	if _, _, err := f.store.Issue(context.Background(), f.tokens["teacher"], "stale"+unique, "stale-request", issue); !errors.Is(err, ErrConflict) {
		t.Fatal("stale transcript issued", err)
	}
	fresh := f.releaseTranscript(t, v)
	issue.TranscriptID = fresh.ID
	if _, err := f.owner.Exec(context.Background(), `REVOKE INSERT ON v3.outbox FROM qa_runtime`); err != nil {
		t.Fatal(err)
	}
	_, _, injected := f.store.Issue(context.Background(), f.tokens["teacher"], "failure"+unique, "failure-request"+unique, issue)
	if _, err := f.owner.Exec(context.Background(), `GRANT INSERT ON v3.outbox TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
	if injected == nil {
		t.Fatal("outbox failure succeeded")
	}
	var records, commands, audits int
	if err := f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.credential_records WHERE transcript_id=$1),
	 (SELECT count(*) FROM v3.credential_commands WHERE key=$2),
	 (SELECT count(*) FROM v3.audit WHERE request_id=$3)`, fresh.ID, "failure"+unique, "failure-request"+unique).
		Scan(&records, &commands, &audits); err != nil || records+commands+audits != 0 {
		t.Fatal("partial issue", records, commands, audits, err)
	}
}
