//go:build qaintegration

package gradebook

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type gradeFixture struct {
	owner  *pgxpool.Pool
	store  *Store
	users  map[string]string
	tokens map[string]string
}

func openGradeFixture(t *testing.T, checkpoint string) *gradeFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallGradeDefinitions(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallGradeDefinitions(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("definition migration replay", err)
	}
	grants := `GRANT SELECT,INSERT ON v3.grade_scales,v3.grade_items,v3.grade_definition_versions,v3.rubric_criteria,v3.grade_definition_publications,v3.grade_definition_commands TO qa_runtime`
	if checkpoint == "m09-cp02" {
		if err = labdb.InstallScopedGrades(ctx, owner); err != nil {
			owner.Close()
			t.Fatal(err)
		}
		if err = labdb.InstallScopedGrades(ctx, owner); err != nil {
			owner.Close()
			t.Fatal("grade migration replay", err)
		}
		grants += `; GRANT SELECT,INSERT ON v3.grade_revisions,v3.grade_current,v3.grade_commands TO qa_runtime; GRANT UPDATE(revision_id,version,updated_at) ON v3.grade_current TO qa_runtime`
	}
	if _, err = owner.Exec(ctx, grants, pgx.QueryExecModeSimpleProtocol); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	f := &gradeFixture{owner: owner, store: New(runtime), users: map[string]string{}, tokens: map[string]string{}}
	for _, subject := range []string{"teacher", "teacher2", "student", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		session, _ := token(32)
		if _, err = owner.Exec(ctx, `UPDATE v3identity.links SET valid_until=GREATEST(valid_until,clock_timestamp()+interval '5 minutes') WHERE user_id=$1; INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until) VALUES($2,$1,$3,$2,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, pgx.QueryExecModeSimpleProtocol, user, identity.TokenHash(session), version); err != nil {
			t.Fatal(err)
		}
		f.users[subject], f.tokens[subject] = user, session
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return f
}

type gradeContext struct{ classID, activityID, aggregateID, attemptID, revisionID string }

func (f *gradeFixture) submission(t *testing.T, suffix, ownerSubject string, teacherIsStudent bool) gradeContext {
	t.Helper()
	ctx := context.Background()
	v := gradeContext{"gc" + suffix, "ga" + suffix, "gagg" + suffix, "gat" + suffix, "gsr" + suffix}
	student := f.users[ownerSubject]
	teacher := f.users["teacher"]
	_, err := f.owner.Exec(ctx, `INSERT INTO v3.classes(id,name,state) VALUES($1,$2,'PUBLISHED'); INSERT INTO v3.class_access(class_id) VALUES($1);
	INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($3,$1,$4,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($5,$3,$4); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($6,$3,$1,'student');
	INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($7,$1,$8,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($9,$7,$8); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($10,$7,$1,'teacher');
	INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($11,$1,'Grades',1,$8); INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) VALUES($12,$1,$11,'ASSIGNMENT','Essay',1,$8);
	INSERT INTO v3.content_revisions(id,class_id,activity_id,revision_no,state,canonical_body,content_format,timezone,created_by) VALUES($13,$1,$12,1,'DRAFT','Essay','SAFE_HTML','Asia/Jakarta',$8); INSERT INTO v3.content_revision_sources(revision_id,raw_body,raw_sha256) VALUES($13,'Essay',$14);
	INSERT INTO v3.assignment_revision_rules(revision_id,class_id,activity_id,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,created_by) VALUES($13,$1,$12,'TEXT',2,0,0,'REJECT_AFTER_CUTOFF',$8); UPDATE v3.content_revisions SET state='PUBLISHED',version=2,updated_at=clock_timestamp() WHERE id=$13;
	INSERT INTO v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,created_by) VALUES($15,$1,$12,$13,$3,$5,$4,$4);
	INSERT INTO v3.submission_attempts(id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,created_by) VALUES($16,$15,$1,$12,$13,1,$4);
	INSERT INTO v3.submission_revisions(id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_by) VALUES($17,$1,$15,$16,1,'answer',$18,$4);
	UPDATE v3.submission_attempts SET state='LOCKED',version=2,updated_at=clock_timestamp(),locked_at=clock_timestamp() WHERE id=$16;
	INSERT INTO v3.submission_receipts(id,class_id,aggregate_id,attempt_id,revision_id,submitted_by) VALUES($19,$1,$15,$16,$17,$4)`, pgx.QueryExecModeSimpleProtocol, v.classID, "Grade "+suffix, "ge"+suffix, student, "gep"+suffix, "grs"+suffix, "gte"+suffix, teacher, "gtep"+suffix, "grt"+suffix, "gsec"+suffix, v.activityID, "gcr"+suffix, digest([]byte("Essay")), v.aggregateID, v.attemptID, v.revisionID, digest([]byte("answer")), "grec"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	if ownerSubject != "teacher2" {
		_, err = f.owner.Exec(ctx, `INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($1,$2,$3,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($4,$1,$3); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($5,$1,$2,'teacher')`, pgx.QueryExecModeSimpleProtocol, "gte2"+suffix, v.classID, f.users["teacher2"], "gtep2"+suffix, "grt2"+suffix)
		if err != nil {
			t.Fatal(err)
		}
	}
	if teacherIsStudent {
		_, err = f.owner.Exec(ctx, "INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($1,$2,$3,'teacher')", "grts"+suffix, "ge"+suffix, v.classID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func definitionCommand(ctx gradeContext, weights ...int) DefinitionCommand {
	criteria := []CriterionInput{{Key: "analysis", Title: "Analysis", WeightBPS: weights[0], MaximumMilli: 100000, Position: 1}, {Key: "clarity", Title: "Clarity", WeightBPS: weights[1], MaximumMilli: 100000, Position: 2}}
	return DefinitionCommand{Action: "create_item", ClassID: ctx.classID, ActivityID: ctx.activityID, Title: "Essay grade", WeightBPS: 10000, Scale: ScaleInput{Name: "Points", MinimumMilli: 0, MaximumMilli: 100000, DisplayDecimals: 3}, Criteria: criteria}
}

func executeDefinition(t *testing.T, f *gradeFixture, subject string, c DefinitionCommand) json.RawMessage {
	t.Helper()
	k, _ := token(16)
	r, _ := token(16)
	value, replayed, err := f.store.ExecuteDefinition(context.Background(), f.tokens[subject], k, r, c)
	if err != nil || replayed {
		t.Fatal(err, replayed)
	}
	return value
}

func createPublishedDefinition(t *testing.T, f *gradeFixture, ctx gradeContext) Definition {
	t.Helper()
	raw := executeDefinition(t, f, "teacher", definitionCommand(ctx, 6000, 4000))
	var d Definition
	if json.Unmarshal(raw, &d) != nil {
		t.Fatal("decode definition")
	}
	publish := DefinitionCommand{Action: "publish", ClassID: ctx.classID, GradeItemID: d.GradeItemID, DefinitionVersionID: d.DefinitionVersionID, ExpectedPublicationRevision: 0}
	executeDefinition(t, f, "teacher", publish)
	return d
}

func TestSyntheticGradeDefinitionLifecycle(t *testing.T) {
	f := openGradeFixture(t, "m09-cp01")
	unique, _ := token(4)
	ctx := f.submission(t, "d1"+unique, "student", false)
	d := createPublishedDefinition(t, f, ctx)
	versions, err := f.store.ReadDefinition(context.Background(), f.tokens["teacher"], ctx.classID, d.GradeItemID)
	if err != nil || len(versions) != 1 || len(versions[0].Criteria) != 2 {
		t.Fatal("definition missing", err, versions)
	}
	v2 := definitionCommand(ctx, 5000, 5000)
	v2.Action = "create_version"
	v2.ActivityID = ""
	v2.Title = ""
	v2.GradeItemID = d.GradeItemID
	raw := executeDefinition(t, f, "teacher", v2)
	var next Definition
	_ = json.Unmarshal(raw, &next)
	if next.Version != 2 {
		t.Fatal("version not advanced", next)
	}
	executeDefinition(t, f, "teacher", DefinitionCommand{Action: "publish", ClassID: ctx.classID, GradeItemID: d.GradeItemID, DefinitionVersionID: next.DefinitionVersionID, ExpectedPublicationRevision: 1})
	if _, err = f.owner.Exec(context.Background(), "UPDATE v3.rubric_criteria SET title='changed' WHERE definition_version_id=$1", d.DefinitionVersionID); err == nil {
		t.Fatal("published rubric mutation accepted")
	}
	var publications int
	if err = f.owner.QueryRow(context.Background(), "SELECT count(*) FROM v3.grade_definition_publications WHERE grade_item_id=$1", d.GradeItemID).Scan(&publications); err != nil || publications != 2 {
		t.Fatal(publications, err)
	}
}

func TestSyntheticGradeDefinitionBoundary(t *testing.T) {
	f := openGradeFixture(t, "m09-cp01")
	unique, _ := token(4)
	ctx := f.submission(t, "d2"+unique, "student", false)
	badRaw := executeDefinition(t, f, "teacher", definitionCommand(ctx, 6000, 3000))
	var bad Definition
	_ = json.Unmarshal(badRaw, &bad)
	_, _, err := f.store.ExecuteDefinition(context.Background(), f.tokens["teacher"], "badweight1", "request1", DefinitionCommand{Action: "publish", ClassID: ctx.classID, GradeItemID: bad.GradeItemID, DefinitionVersionID: bad.DefinitionVersionID, ExpectedPublicationRevision: 0})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("invalid weight published", err)
	}
	cycle := definitionCommand(ctx, 5000, 5000)
	cycle.ActivityID = ctx.activityID + "x"
	if _, err = f.owner.Exec(context.Background(), "INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) SELECT $1,class_id,section_id,'ASSIGNMENT','Cycle',2,created_by FROM v3.content_activities WHERE id=$2", cycle.ActivityID, ctx.activityID); err != nil {
		t.Fatal(err)
	}
	cycle.Criteria[0].ParentKey = "clarity"
	cycle.Criteria[1].ParentKey = "analysis"
	cycleRaw := executeDefinition(t, f, "teacher", cycle)
	var cyclic Definition
	_ = json.Unmarshal(cycleRaw, &cyclic)
	_, _, err = f.store.ExecuteDefinition(context.Background(), f.tokens["teacher"], "badcycle1", "request2", DefinitionCommand{Action: "publish", ClassID: ctx.classID, GradeItemID: cyclic.GradeItemID, DefinitionVersionID: cyclic.DefinitionVersionID, ExpectedPublicationRevision: 0})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("cycle published", err)
	}
	_, _, err = f.store.ExecuteDefinition(context.Background(), f.tokens["outsider"], "outsider1", "request3", definitionCommand(ctx, 5000, 5000))
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider authored definition", err)
	}
	var published int
	_ = f.owner.QueryRow(context.Background(), "SELECT count(*) FROM v3.grade_definition_publications WHERE grade_item_id IN ($1,$2)", bad.GradeItemID, cyclic.GradeItemID).Scan(&published)
	if published != 0 {
		t.Fatal("failed publication left state", published)
	}
}

func executeGrade(f *gradeFixture, subject, key, request string, c GradeCommand) (json.RawMessage, bool, error) {
	return f.store.Grade(context.Background(), f.tokens[subject], key, request, c)
}

func TestSyntheticScopedGradeLifecycle(t *testing.T) {
	f := openGradeFixture(t, "m09-cp02")
	unique, _ := token(4)
	ctx := f.submission(t, "g1"+unique, "student", false)
	d := createPublishedDefinition(t, f, ctx)
	command := GradeCommand{ClassID: ctx.classID, GradeItemID: d.GradeItemID, AggregateID: ctx.aggregateID, SubmissionRevisionID: ctx.revisionID, ExpectedVersion: 0, ValueMilli: 80000}
	raw, replayed, err := executeGrade(f, "teacher", "initial"+unique, "grade1"+unique, command)
	if err != nil || replayed {
		t.Fatal(err, replayed)
	}
	var grade Grade
	_ = json.Unmarshal(raw, &grade)
	if grade.Version != 1 || grade.Reason != "INITIAL" {
		t.Fatal(grade)
	}
	command.ExpectedVersion = 1
	command.ValueMilli = 85000
	command.Reason = "Reviewed evidence"
	raw, _, err = executeGrade(f, "teacher", "regrade"+unique, "grade2"+unique, command)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &grade)
	if grade.Version != 2 || grade.SubmissionRevisionID != ctx.revisionID {
		t.Fatal(grade)
	}
	command.ExpectedVersion = 2
	command.Reason = "Concurrent moderation"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := command
			c.ValueMilli = 86000 + int64(i)
			_, _, e := executeGrade(f, "teacher2", []string{"concur1" + unique, "concur2" + unique}[i], []string{"grade3a" + unique, "grade3b" + unique}[i], c)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	var history, current int
	if err = f.owner.QueryRow(context.Background(), "SELECT count(*),(SELECT version FROM v3.grade_current WHERE grade_item_id=$1 AND aggregate_id=$2) FROM v3.grade_revisions WHERE grade_item_id=$1 AND aggregate_id=$2", d.GradeItemID, ctx.aggregateID).Scan(&history, &current); err != nil || history != 3 || current != 3 {
		t.Fatal(history, current, err)
	}
}

func TestSyntheticScopedGradeBoundary(t *testing.T) {
	f := openGradeFixture(t, "m09-cp02")
	unique, _ := token(4)
	ctx := f.submission(t, "g2"+unique, "student", false)
	d := createPublishedDefinition(t, f, ctx)
	base := GradeCommand{ClassID: ctx.classID, GradeItemID: d.GradeItemID, AggregateID: ctx.aggregateID, SubmissionRevisionID: ctx.revisionID, ExpectedVersion: 0, ValueMilli: 50000}
	foreign := f.submission(t, "foreign"+unique, "student", false)
	_, _, err := executeGrade(f, "teacher", "foreign01", "boundary1", GradeCommand{ClassID: ctx.classID, GradeItemID: d.GradeItemID, AggregateID: ctx.aggregateID, SubmissionRevisionID: foreign.revisionID, ExpectedVersion: 0, ValueMilli: 50000})
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign ancestry accepted", err)
	}
	over := base
	over.ValueMilli = 100001
	_, _, err = executeGrade(f, "teacher", "bounds001", "boundary2", over)
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("out of bounds accepted", err)
	}
	self := f.submission(t, "self"+unique, "teacher2", true)
	selfDef := createPublishedDefinition(t, f, self)
	_, _, err = executeGrade(f, "teacher2", "selfgrade1", "boundary3", GradeCommand{ClassID: self.classID, GradeItemID: selfDef.GradeItemID, AggregateID: self.aggregateID, SubmissionRevisionID: self.revisionID, ExpectedVersion: 0, ValueMilli: 50000})
	if !errors.Is(err, ErrForbidden) {
		t.Fatal("self grade accepted", err)
	}
	if _, err = f.owner.Exec(context.Background(), "REVOKE INSERT ON v3.outbox FROM qa_runtime"); err != nil {
		t.Fatal(err)
	}
	_, _, injected := executeGrade(f, "teacher", "failure01", "boundary4", base)
	if _, err = f.owner.Exec(context.Background(), "GRANT INSERT ON v3.outbox TO qa_runtime"); err != nil {
		t.Fatal(err)
	}
	if injected == nil {
		t.Fatal("failure injection did not fail")
	}
	var revisions, current, commands, audits int
	err = f.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM v3.grade_revisions WHERE grade_item_id=$1 AND aggregate_id=$2),(SELECT count(*) FROM v3.grade_current WHERE grade_item_id=$1 AND aggregate_id=$2),(SELECT count(*) FROM v3.grade_commands WHERE key='failure01'),(SELECT count(*) FROM v3.audit WHERE request_id='boundary4')`, d.GradeItemID, ctx.aggregateID).Scan(&revisions, &current, &commands, &audits)
	if err != nil || revisions+current+commands+audits != 0 {
		t.Fatal("partial grade survived rollback", revisions, current, commands, audits, err)
	}
}
