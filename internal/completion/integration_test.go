//go:build qaintegration

package completion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type completionFixture struct {
	owner  *pgxpool.Pool
	store  *Store
	users  map[string]string
	tokens map[string]string
}

type completionContext struct {
	classID, activityID, revisionID, enrollmentID, episodeID string
}

func openCompletionFixture(t *testing.T) *completionFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallCompletionFoundation(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallCompletionFoundation(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("completion migration replay", err)
	}
	if _, err = owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.completion_rule_versions,v3.learning_evidence,v3.activity_completion_outcomes,v3.completion_commands TO qa_runtime`, pgx.QueryExecModeSimpleProtocol); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	f := &completionFixture{owner: owner, store: New(runtime), users: map[string]string{}, tokens: map[string]string{}}
	for _, subject := range []string{"admin", "teacher", "student", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		session, _ := token(32)
		if _, err = owner.Exec(ctx, `UPDATE v3identity.links SET valid_until=GREATEST(valid_until,clock_timestamp()+interval '5 minutes') WHERE user_id=$1;
		 INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until) VALUES($2,$1,$3,$2,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, pgx.QueryExecModeSimpleProtocol, user, identity.TokenHash(session), version); err != nil {
			t.Fatal(err)
		}
		f.users[subject], f.tokens[subject] = user, session
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return f
}

func (f *completionFixture) learningContext(t *testing.T, suffix string, withReleasedGrade bool) completionContext {
	t.Helper()
	student, teacher := f.users["student"], f.users["teacher"]
	v := completionContext{"cc" + suffix, "ca" + suffix, "cr" + suffix, "ce" + suffix, "cep" + suffix}
	_, err := f.owner.Exec(context.Background(), `INSERT INTO v3.classes(id,name,state) VALUES($1,$2,'PUBLISHED'); INSERT INTO v3.class_access(class_id) VALUES($1);
	 INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($3,$1,$4,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($5,$3,$4); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($6,$3,$1,'student');
	 INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($7,$1,$8,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($9,$7,$8); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($10,$7,$1,'teacher');
	 INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($11,$1,'Completion',1,$8);
	 INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) VALUES($12,$1,$11,'ASSIGNMENT','Completion activity',1,$8);
	 INSERT INTO v3.content_revisions(id,class_id,activity_id,revision_no,state,canonical_body,content_format,timezone,created_by) VALUES($13,$1,$12,1,'DRAFT','Complete me','SAFE_HTML','Asia/Jakarta',$8);
	 INSERT INTO v3.content_revision_sources(revision_id,raw_body,raw_sha256) VALUES($13,'Complete me',$14);
	 INSERT INTO v3.assignment_revision_rules(revision_id,class_id,activity_id,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,created_by) VALUES($13,$1,$12,'TEXT',2,0,0,'REJECT_AFTER_CUTOFF',$8);
	 UPDATE v3.content_revisions SET state='PUBLISHED',version=2,updated_at=clock_timestamp() WHERE id=$13`, pgx.QueryExecModeSimpleProtocol,
		v.classID, "Completion "+suffix, v.enrollmentID, student, v.episodeID, "csr"+suffix, "cte"+suffix, teacher, "ctep"+suffix, "ctr"+suffix, "cs"+suffix, v.activityID, v.revisionID, digest([]byte("Complete me")))
	if err != nil {
		t.Fatal(err)
	}
	if !withReleasedGrade {
		return v
	}
	_, err = f.owner.Exec(context.Background(), `INSERT INTO v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$7);
	 INSERT INTO v3.submission_attempts(id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,created_by) VALUES($8,$1,$2,$3,$4,1,$7);
	 INSERT INTO v3.submission_revisions(id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_by) VALUES($9,$2,$1,$8,1,'answer',$10,$7);
	 UPDATE v3.submission_attempts SET state='LOCKED',version=2,updated_at=clock_timestamp(),locked_at=clock_timestamp() WHERE id=$8;
	 INSERT INTO v3.submission_receipts(id,class_id,aggregate_id,attempt_id,revision_id,submitted_by) VALUES($11,$2,$1,$8,$9,$7);
	 INSERT INTO v3.grade_scales(id,class_id,name,minimum_milli,maximum_milli,display_decimals,created_by) VALUES($12,$2,'Points',0,100000,3,$13);
	 INSERT INTO v3.grade_items(id,class_id,activity_id,title,created_by) VALUES($14,$2,$3,'Grade',$13);
	 INSERT INTO v3.grade_definition_versions(id,grade_item_id,class_id,version,scale_id,weight_bps,created_by) VALUES($15,$14,$2,1,$12,10000,$13);
	 INSERT INTO v3.rubric_criteria(id,definition_version_id,grade_item_id,class_id,criterion_key,title,weight_bps,maximum_milli,position) VALUES($16,$15,$14,$2,'complete','Complete',10000,100000,1);
	 INSERT INTO v3.grade_definition_publications(id,grade_item_id,class_id,definition_version_id,revision,actor_id) VALUES($17,$14,$2,$15,1,$13);
	 INSERT INTO v3.grade_revisions(id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,learner_user_id,grader_user_id,version,value_milli,reason) VALUES($18,$2,$14,$15,$1,$8,$9,$7,$13,1,100000,'HIGH_GRADE');
	 INSERT INTO v3.grade_current(grade_item_id,aggregate_id,class_id,revision_id,version) VALUES($14,$1,$2,$18,1);
	 INSERT INTO v3.grade_releases(id,class_id,grade_item_id,aggregate_id,revision_id,grade_version,learner_user_id,actor_id,key,request_hash) VALUES($19,$2,$14,$1,$18,1,$7,$13,$20,$21)`,
		pgx.QueryExecModeSimpleProtocol, "cagg"+suffix, v.classID, v.activityID, v.revisionID, v.enrollmentID, v.episodeID, student,
		"cat"+suffix, "csrev"+suffix, digest([]byte("answer")), "crec"+suffix, "cscale"+suffix, teacher, "citem"+suffix,
		"cdef"+suffix, "ccrit"+suffix, "cpub"+suffix, "cgrade"+suffix, "crel"+suffix, "release"+suffix, digest([]byte("release"+suffix)))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ruleCommand(v completionContext, expected int64) RuleCommand {
	return RuleCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, ExpectedVersion: expected, Requirement: "EVIDENCE_PRESENT", EvidenceKind: "ACTIVITY_ACKNOWLEDGED"}
}

func (f *completionFixture) createRule(t *testing.T, v completionContext, expected int64) Rule {
	t.Helper()
	key, _ := token(12)
	request, _ := token(12)
	rule, replayed, err := f.store.CreateRule(context.Background(), f.tokens["teacher"], key, request, ruleCommand(v, expected))
	if err != nil || replayed {
		t.Fatal(err, replayed)
	}
	return rule
}

func TestSyntheticCompletionRuleSnapshot(t *testing.T) {
	f := openCompletionFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "rule"+unique, false)
	key, request := "rulekey"+unique, "rulerequest"+unique
	first, replayed, err := f.store.CreateRule(context.Background(), f.tokens["teacher"], key, request, ruleCommand(v, 0))
	if err != nil || replayed || first.Version != 1 {
		t.Fatal(first, replayed, err)
	}
	replay, replayed, err := f.store.CreateRule(context.Background(), f.tokens["teacher"], key, "other-request", ruleCommand(v, 0))
	if err != nil || !replayed || replay.ID != first.ID {
		t.Fatal(replay, replayed, err)
	}
	second := f.createRule(t, v, 1)
	if second.Version != 2 || second.ID == first.ID {
		t.Fatal(second)
	}
	if _, err = f.owner.Exec(context.Background(), "UPDATE v3.completion_rule_versions SET evidence_kind='ACTIVITY_ACKNOWLEDGED' WHERE id=$1", first.ID); err == nil {
		t.Fatal("immutable rule snapshot updated")
	}
}

func TestSyntheticCompletionOutcomeSeparation(t *testing.T) {
	f := openCompletionFixture(t)
	unique, _ := token(4)
	withoutGrade := f.learningContext(t, "nograde"+unique, false)
	rule := f.createRule(t, withoutGrade, 0)
	completed, _, err := f.store.Evaluate(context.Background(), f.tokens["student"], "complete"+unique, "complete-request"+unique, EvaluationCommand{ClassID: withoutGrade.classID, ActivityID: withoutGrade.activityID, ContentRevisionID: withoutGrade.revisionID, RuleVersionID: rule.ID, Acknowledge: true})
	if err != nil || completed.State != "COMPLETED" || completed.EvidenceID == nil {
		t.Fatal(completed, err)
	}
	withGrade := f.learningContext(t, "highgrade"+unique, true)
	highRule := f.createRule(t, withGrade, 0)
	incomplete, _, err := f.store.Evaluate(context.Background(), f.tokens["student"], "incomplete"+unique, "incomplete-request"+unique, EvaluationCommand{ClassID: withGrade.classID, ActivityID: withGrade.activityID, ContentRevisionID: withGrade.revisionID, RuleVersionID: highRule.ID})
	if err != nil || incomplete.State != "INCOMPLETE" || incomplete.EvidenceID != nil || incomplete.Reason != "EVIDENCE_MISSING" {
		t.Fatal(incomplete, err)
	}
	var releases, evidence int
	if err = f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.grade_releases WHERE class_id=$1),(SELECT count(*) FROM v3.learning_evidence WHERE class_id=$1)`, withGrade.classID).Scan(&releases, &evidence); err != nil || releases != 1 || evidence != 0 {
		t.Fatal("grade/completion separation failed", releases, evidence, err)
	}
}

func TestSyntheticCompletionBoundary(t *testing.T) {
	f := openCompletionFixture(t)
	unique, _ := token(4)
	v := f.learningContext(t, "boundary"+unique, false)
	foreign := f.learningContext(t, "foreign"+unique, false)
	if _, _, err := f.store.CreateRule(context.Background(), f.tokens["student"], "deniedrule"+unique, "denied-request", ruleCommand(v, 0)); !errors.Is(err, ErrForbidden) {
		t.Fatal("learner authored rule", err)
	}
	rule := f.createRule(t, v, 0)
	if _, _, err := f.store.Evaluate(context.Background(), f.tokens["outsider"], "outsider"+unique, "outsider-request", EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true}); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider evaluated", err)
	}
	if _, _, err := f.store.Evaluate(context.Background(), f.tokens["student"], "foreign"+unique, "foreign-request", EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: foreign.revisionID, RuleVersionID: rule.ID, Acknowledge: true}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign revision accepted", err)
	}
	latest := f.createRule(t, v, 1)
	if _, _, err := f.store.Evaluate(context.Background(), f.tokens["student"], "stale"+unique, "stale-request", EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: rule.ID, Acknowledge: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale rule accepted", err)
	}
	if _, err := f.owner.Exec(context.Background(), "REVOKE INSERT ON v3.outbox FROM qa_runtime"); err != nil {
		t.Fatal(err)
	}
	failureKey, failureRequest := "failure"+unique, "failure-request"+unique
	_, _, injected := f.store.Evaluate(context.Background(), f.tokens["student"], failureKey, failureRequest, EvaluationCommand{ClassID: v.classID, ActivityID: v.activityID, ContentRevisionID: v.revisionID, RuleVersionID: latest.ID, Acknowledge: true})
	if _, err := f.owner.Exec(context.Background(), "GRANT INSERT ON v3.outbox TO qa_runtime"); err != nil {
		t.Fatal(err)
	}
	if injected == nil {
		t.Fatal("outbox failure injection succeeded")
	}
	var evidence, outcomes, commands, audits int
	if err := f.owner.QueryRow(context.Background(), `SELECT
	 (SELECT count(*) FROM v3.learning_evidence WHERE source_ref=$1),
	 (SELECT count(*) FROM v3.activity_completion_outcomes WHERE rule_version_id=$2),
	 (SELECT count(*) FROM v3.completion_commands WHERE key=$3),
	 (SELECT count(*) FROM v3.audit WHERE request_id=$4)`, "command:"+digest([]byte(f.users["student"]+":"+failureKey)), latest.ID, failureKey, failureRequest).Scan(&evidence, &outcomes, &commands, &audits); err != nil || evidence+outcomes+commands+audits != 0 {
		t.Fatal("partial completion transaction survived", evidence, outcomes, commands, audits, err)
	}
}
