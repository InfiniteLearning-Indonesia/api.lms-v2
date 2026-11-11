//go:build qaintegration

package completion

import (
	"context"
	"errors"
	"testing"

	"lmsv3/internal/labdb"
)

func openCorrectionFixture(t *testing.T) *completionFixture {
	t.Helper()
	f := openCompletionFixture(t)
	ctx := context.Background()
	if err := labdb.InstallCompletionCorrection(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallCompletionCorrection(ctx, f.owner); err != nil {
		t.Fatal("correction replay", err)
	}
	if _, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.completion_corrections,v3.completion_correction_commands TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSyntheticCompletionProgressProjection(t *testing.T) {
	f := openCorrectionFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "progress"+unique, false)
	rule := f.createRule(t, v, 0)
	before, err := f.store.Progress(context.Background(), f.tokens["student"], v.classID, "")
	if err != nil || before.Total != 1 || before.Completed != 0 || before.Activities[0].Version != 0 {
		t.Fatal(before, err)
	}
	_, _, err = f.store.Evaluate(context.Background(), f.tokens["student"], "evaluate"+unique, "evaluate-request"+unique,
		EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true})
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.store.Progress(context.Background(), f.tokens["student"], v.classID, "")
	again, againErr := f.store.Progress(context.Background(), f.tokens["student"], v.classID, "")
	if err != nil || againErr != nil || after.Total != 1 || after.Completed != 1 || after.Activities[0].Version != 1 ||
		after.Activities[0].State != "COMPLETED" || again.Completed != after.Completed || again.Activities[0] != after.Activities[0] {
		t.Fatal(after, again, err, againErr)
	}
	var corrections int
	if err = f.owner.QueryRow(context.Background(), `SELECT count(*) FROM v3.completion_corrections WHERE class_id=$1`, v.classID).Scan(&corrections); err != nil || corrections != 0 {
		t.Fatal("GET wrote history", corrections, err)
	}
}

func TestSyntheticCompletionOverrideReopen(t *testing.T) {
	f := openCorrectionFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "reopen"+unique, false)
	rule := f.createRule(t, v, 0)
	command := CorrectionCommand{ClassID: v.classID, ActivityID: v.activityID, RuleVersionID: rule.ID, LearnerUserID: f.users["student"],
		Action: "OVERRIDE_COMPLETE", Reason: "Teacher verified offline activity", ExpectedVersion: 0}
	first, replayed, err := f.store.Correct(context.Background(), f.tokens["teacher"], "override"+unique, "override-request"+unique, command)
	if err != nil || replayed || first.Version != 1 || first.State != "COMPLETED" {
		t.Fatal(first, replayed, err)
	}
	copy, replayed, err := f.store.Correct(context.Background(), f.tokens["teacher"], "override"+unique, "another-request", command)
	if err != nil || !replayed || copy.ID != first.ID {
		t.Fatal(copy, replayed, err)
	}
	if _, _, err = f.store.Evaluate(context.Background(), f.tokens["student"], "blocked"+unique, "blocked-request",
		EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("evaluation overwrote override", err)
	}
	command.Action = "REOPEN"
	command.Reason = "Reopen for new learner evidence"
	command.ExpectedVersion = 1
	second, _, err := f.store.Correct(context.Background(), f.tokens["teacher"], "reopen"+unique, "reopen-request", command)
	if err != nil || second.Version != 2 || second.State != "INCOMPLETE" {
		t.Fatal(second, err)
	}
	_, _, err = f.store.Evaluate(context.Background(), f.tokens["student"], "allowed"+unique, "allowed-request",
		EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true})
	if err != nil {
		t.Fatal(err)
	}
	progress, err := f.store.Progress(context.Background(), f.tokens["student"], v.classID, "")
	if err != nil || progress.Completed != 1 || progress.Activities[0].Version != 3 {
		t.Fatal(progress, err)
	}
	var corrections, outcomes int
	if err = f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.completion_corrections WHERE class_id=$1),(SELECT count(*) FROM v3.activity_completion_outcomes WHERE class_id=$1)`, v.classID).Scan(&corrections, &outcomes); err != nil || corrections != 2 || outcomes != 1 {
		t.Fatal(corrections, outcomes, err)
	}
}

func TestSyntheticCompletionCorrectionBoundary(t *testing.T) {
	f := openCorrectionFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "boundary2"+unique, false)
	foreign := f.learningContext(t, "foreign2"+unique, false)
	rule := f.createRule(t, v, 0)
	command := CorrectionCommand{ClassID: v.classID, ActivityID: v.activityID, RuleVersionID: rule.ID, LearnerUserID: f.users["student"],
		Action: "OVERRIDE_COMPLETE", Reason: "Teacher verified offline activity", ExpectedVersion: 0}
	if _, _, err := f.store.Correct(context.Background(), f.tokens["student"], "student"+unique, "student-request", command); !errors.Is(err, ErrForbidden) {
		t.Fatal("student correction", err)
	}
	if _, _, err := f.store.Correct(context.Background(), f.tokens["outsider"], "outsider"+unique, "outsider-request", command); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider correction", err)
	}
	if _, err := f.store.Progress(context.Background(), f.tokens["outsider"], v.classID, f.users["student"]); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider progress", err)
	}
	foreignRule := f.createRule(t, foreign, 0)
	command.RuleVersionID = foreignRule.ID
	if _, _, err := f.store.Correct(context.Background(), f.tokens["teacher"], "foreign"+unique, "foreign-request", command); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign rule", err)
	}
	command.RuleVersionID = rule.ID
	command.ExpectedVersion = 7
	if _, _, err := f.store.Correct(context.Background(), f.tokens["teacher"], "stale"+unique, "stale-request", command); !errors.Is(err, ErrConflict) {
		t.Fatal("stale version", err)
	}
	command.ExpectedVersion = 0
	if _, err := f.owner.Exec(context.Background(), `REVOKE INSERT ON v3.outbox FROM qa_runtime`); err != nil {
		t.Fatal(err)
	}
	_, _, injected := f.store.Correct(context.Background(), f.tokens["teacher"], "failure"+unique, "failure-request"+unique, command)
	if _, err := f.owner.Exec(context.Background(), `GRANT INSERT ON v3.outbox TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
	if injected == nil {
		t.Fatal("outbox failure succeeded")
	}
	var corrections, commands, audits int
	if err := f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.completion_corrections WHERE rule_version_id=$1),
	 (SELECT count(*) FROM v3.completion_correction_commands WHERE key=$2),
	 (SELECT count(*) FROM v3.audit WHERE request_id=$3)`, rule.ID, "failure"+unique, "failure-request"+unique).Scan(&corrections, &commands, &audits); err != nil || corrections+commands+audits != 0 {
		t.Fatal("partial transaction", corrections, commands, audits, err)
	}
}
