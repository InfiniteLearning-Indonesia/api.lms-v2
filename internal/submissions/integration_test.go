//go:build qaintegration

package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type integrationFixture struct {
	owner  *pgxpool.Pool
	store  *Store
	users  map[string]string
	tokens map[string]string
}

func openIntegrationFixture(t *testing.T, checkpoint string) *integrationFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallSubmissions(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallSubmissions(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	grants := `GRANT SELECT,INSERT ON v3.submission_aggregates,v3.submission_attempts,v3.submission_revisions,v3.submission_revision_files,v3.submission_commands TO qa_runtime`
	if checkpoint != "m08-cp01" {
		grants += `; GRANT UPDATE(current_attempt_no,version,updated_at) ON v3.submission_aggregates TO qa_runtime; GRANT UPDATE(state,version,updated_at,locked_at,returned_at,returned_by) ON v3.submission_attempts TO qa_runtime`
	}
	if checkpoint == "m08-cp03" || checkpoint == "m08-cp04" {
		grants += `; GRANT SELECT,INSERT ON v3.submission_receipts TO qa_runtime`
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
	f := &integrationFixture{owner: owner, store: New(runtime), users: map[string]string{}, tokens: map[string]string{}}
	for _, subject := range []string{"teacher", "student", "outsider"} {
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

func (f *integrationFixture) assignment(t *testing.T, suffix string, attempts int, mode string, schedule ...time.Time) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	classID := "c08" + suffix
	enrollmentID := "e08" + suffix
	episodeID := "p08" + suffix
	sectionID := "s08" + suffix
	activityID := "a08" + suffix
	revisionID := "r08" + suffix
	teacherEnrollmentID := "te08" + suffix
	var due, cutoff any
	if len(schedule) > 0 {
		due = schedule[0]
	}
	if len(schedule) > 1 {
		cutoff = schedule[1]
	}
	_, err := f.owner.Exec(ctx, `INSERT INTO v3.classes(id,name,state) VALUES($1,$2,'PUBLISHED'); INSERT INTO v3.class_access(class_id) VALUES($1); INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($3,$1,$4,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($5,$3,$4); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($6,$3,$1,'student'); INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($14,$1,$8,'ACTIVE'); INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($15,$14,$8); INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($16,$14,$1,'teacher'); INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($7,$1,'Work',1,$8); INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) VALUES($9,$1,$7,'ASSIGNMENT','Work',1,$8); INSERT INTO v3.content_revisions(id,class_id,activity_id,revision_no,state,canonical_body,content_format,timezone,due_at,cutoff_at,created_by) VALUES($10,$1,$9,1,'DRAFT','Work','SAFE_HTML','Asia/Jakarta',$17,$18,$8); INSERT INTO v3.content_revision_sources(revision_id,raw_body,raw_sha256) VALUES($10,'Work',$11); INSERT INTO v3.assignment_revision_rules(revision_id,class_id,activity_id,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,created_by) VALUES($10,$1,$9,$12,$13,2,2097152,'REJECT_AFTER_CUTOFF',$8); UPDATE v3.content_revisions SET state='PUBLISHED',version=version+1,updated_at=clock_timestamp() WHERE id=$10`, pgx.QueryExecModeSimpleProtocol, classID, "Class "+suffix, enrollmentID, f.users["student"], episodeID, "role"+suffix, sectionID, f.users["teacher"], activityID, revisionID, digest([]byte("Work")), mode, attempts, teacherEnrollmentID, "tep08"+suffix, "trole"+suffix, due, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	return classID, activityID, revisionID
}

func executeFixture(t *testing.T, store *Store, session string, command Command) json.RawMessage {
	t.Helper()
	key, _ := token(16)
	request, _ := token(16)
	response, replayed, err := store.Execute(context.Background(), session, key, request, command)
	if err != nil || replayed {
		t.Fatal(err, replayed)
	}
	return response
}

func TestSyntheticSubmissionFoundation(t *testing.T) {
	f := openIntegrationFixture(t, "m08-cp01")
	unique, _ := token(4)
	classID, activityID, revisionID := f.assignment(t, "01"+unique, 2, "TEXT_AND_FILES")
	response := executeFixture(t, f.store, f.tokens["student"], Command{Action: "create", ClassID: classID, ActivityID: activityID, AssignmentRevisionID: revisionID})
	var aggregate Aggregate
	if json.Unmarshal(response, &aggregate) != nil {
		t.Fatal("decode")
	}
	if aggregate.OwnerUserID != f.users["student"] || aggregate.EnrollmentID == "" || aggregate.EpisodeID == "" || len(aggregate.Attempts) != 1 {
		t.Fatal("server-derived context missing", aggregate)
	}
	foreignClass, foreignActivity, foreignRevision := f.assignment(t, "01foreign"+unique, 2, "TEXT_AND_FILES")
	_, _, err := f.store.Execute(context.Background(), f.tokens["student"], "foreign01", "request01", Command{Action: "create", ClassID: classID, ActivityID: foreignActivity, AssignmentRevisionID: foreignRevision})
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign contract accepted", err, foreignClass)
	}
	if _, err = f.owner.Exec(context.Background(), "UPDATE v3.enrollments SET status='ENDED' WHERE id=$1", aggregate.EnrollmentID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.Execute(context.Background(), f.tokens["student"], "ended0001", "request02", Command{Action: "create", ClassID: classID, ActivityID: activityID, AssignmentRevisionID: revisionID}); !errors.Is(err, ErrForbidden) {
		t.Fatal("ended actor accepted", err)
	}
	var broken int
	if err = f.owner.QueryRow(context.Background(), `SELECT count(*) FROM v3.submission_aggregates s LEFT JOIN v3.enrollments e ON e.id=s.enrollment_id AND e.class_id=s.class_id AND e.user_id=s.owner_user_id LEFT JOIN v3.episodes p ON p.id=s.episode_id AND p.enrollment_id=s.enrollment_id LEFT JOIN v3.content_revisions r ON r.id=s.assignment_revision_id AND r.class_id=s.class_id AND r.activity_id=s.activity_id WHERE e.id IS NULL OR p.id IS NULL OR r.id IS NULL`).Scan(&broken); err != nil || broken != 0 {
		t.Fatal("context reconciliation", broken, err)
	}
}

func TestSyntheticSubmissionRevisionLifecycle(t *testing.T) {
	f := openIntegrationFixture(t, "m08-cp02")
	unique, _ := token(4)
	classID, activityID, revisionID := f.assignment(t, "02"+unique, 2, "TEXT_AND_FILES")
	created := executeFixture(t, f.store, f.tokens["student"], Command{Action: "create", ClassID: classID, ActivityID: activityID, AssignmentRevisionID: revisionID})
	var aggregate Aggregate
	_ = json.Unmarshal(created, &aggregate)
	assetID := "asset08cp02" + unique
	_, err := f.owner.Exec(context.Background(), `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at) VALUES($1,$2,$3,'EVIDENCE',$4,'work.txt','text/plain','text/plain',4,$5,'AVAILABLE','CLEAN','CLEAN',$3,clock_timestamp())`, assetID, classID, f.users["student"], digest([]byte("object")), digest([]byte("work")))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key, _ := token(16)
			request, _ := token(16)
			files := []string{}
			if i == 0 {
				files = []string{assetID}
			}
			_, _, runErr := f.store.Execute(context.Background(), f.tokens["student"], key, request, Command{Action: "append_revision", ClassID: classID, AggregateID: aggregate.ID, Version: aggregate.Version, Body: "draft", FileAssetIDs: files})
			errs <- runErr
		}(index)
	}
	wg.Wait()
	close(errs)
	for runErr := range errs {
		if runErr != nil {
			t.Fatal("concurrent append", runErr)
		}
	}
	locked := executeFixture(t, f.store, f.tokens["student"], Command{Action: "lock", ClassID: classID, AggregateID: aggregate.ID, AttemptVersion: 1})
	var attempt Attempt
	_ = json.Unmarshal(locked, &attempt)
	returned := executeFixture(t, f.store, f.tokens["teacher"], Command{Action: "return", ClassID: classID, AggregateID: aggregate.ID, AttemptVersion: attempt.Version})
	_ = json.Unmarshal(returned, &attempt)
	resubmitted := executeFixture(t, f.store, f.tokens["student"], Command{Action: "resubmit", ClassID: classID, AggregateID: aggregate.ID, Version: aggregate.Version, AttemptVersion: attempt.Version})
	_ = json.Unmarshal(resubmitted, &aggregate)
	if aggregate.CurrentAttemptNo != 2 {
		t.Fatal("resubmit attempt missing")
	}
	read, err := f.store.Read(context.Background(), f.tokens["student"], classID, aggregate.ID)
	if err != nil || len(read.Attempts) != 2 {
		t.Fatal("retained attempts missing", err, read)
	}
	fileReferences := 0
	for _, revision := range read.Attempts[0].Revisions {
		for _, attachment := range revision.Files {
			if attachment.AssetID == assetID {
				fileReferences++
			}
		}
	}
	if len(read.Attempts[0].Revisions) != 2 || fileReferences != 1 {
		t.Fatal("retained work missing", err, read)
	}
	if _, _, err = f.store.Execute(context.Background(), f.tokens["outsider"], "outsider1", "request03", Command{Action: "append_revision", ClassID: classID, AggregateID: aggregate.ID, Version: aggregate.Version, Body: "steal"}); !errors.Is(err, ErrConflict) && !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign actor accepted", err)
	}
}

func TestSyntheticSubmissionHandinReceipt(t *testing.T) {
	f := openIntegrationFixture(t, "m08-cp03")
	unique, _ := token(4)
	now := time.Now().UTC()
	classID, activityID, revisionID := f.assignment(t, "03"+unique, 2, "TEXT_AND_FILES", now.Add(time.Hour), now.Add(2*time.Hour))
	created := executeFixture(t, f.store, f.tokens["student"], Command{Action: "create", ClassID: classID, ActivityID: activityID, AssignmentRevisionID: revisionID})
	var aggregate Aggregate
	if err := json.Unmarshal(created, &aggregate); err != nil {
		t.Fatal(err)
	}
	response := executeFixture(t, f.store, f.tokens["student"], Command{Action: "append_revision", ClassID: classID, AggregateID: aggregate.ID, Version: 1, Body: "final work"})
	var revision Revision
	if err := json.Unmarshal(response, &revision); err != nil {
		t.Fatal(err)
	}
	key, _ := token(16)
	request := Command{Action: "handin", ClassID: classID, AggregateID: aggregate.ID, Version: 1, AttemptVersion: 1}
	var wg sync.WaitGroup
	type outcome struct {
		value    json.RawMessage
		replayed bool
		err      error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, replayed, err := f.store.Execute(context.Background(), f.tokens["student"], key, "request03", request)
			outcomes <- outcome{value, replayed, err}
		}()
	}
	wg.Wait()
	close(outcomes)
	replayedCount := 0
	var receipt Receipt
	for result := range outcomes {
		if result.err != nil || json.Unmarshal(result.value, &receipt) != nil {
			t.Fatal("concurrent handin", result.err)
		}
		if result.replayed {
			replayedCount++
		}
	}
	if replayedCount != 1 || receipt.RevisionID != revision.ID || receipt.SubmittedBy != f.users["student"] || receipt.Late || receipt.Timezone != "Asia/Jakarta" || receipt.SubmittedLocal == "" {
		t.Fatal("incorrect durable receipt", receipt, replayedCount)
	}
	if _, _, err := f.store.Execute(context.Background(), f.tokens["student"], key, "request03", Command{Action: "handin", ClassID: classID, AggregateID: aggregate.ID, Version: 1, AttemptVersion: 2}); !errors.Is(err, ErrConflict) {
		t.Fatal("conflicting retry accepted", err)
	}
	if _, _, err := f.store.Execute(context.Background(), f.tokens["student"], "secondhandin", "request03", request); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate handin accepted", err)
	}
	read, err := f.store.Read(context.Background(), f.tokens["student"], classID, aggregate.ID)
	if err != nil || len(read.Attempts) != 1 || read.Attempts[0].Receipt == nil || read.Attempts[0].Receipt.ID != receipt.ID {
		t.Fatal("receipt missing from history", err)
	}
	lateClass, lateActivity, lateRevision := f.assignment(t, "03late"+unique, 2, "TEXT_AND_FILES", now.Add(-time.Hour), now.Add(time.Hour))
	lateCreated := executeFixture(t, f.store, f.tokens["student"], Command{Action: "create", ClassID: lateClass, ActivityID: lateActivity, AssignmentRevisionID: lateRevision})
	_ = json.Unmarshal(lateCreated, &aggregate)
	executeFixture(t, f.store, f.tokens["student"], Command{Action: "append_revision", ClassID: lateClass, AggregateID: aggregate.ID, Version: 1, Body: "late work"})
	lateResult := executeFixture(t, f.store, f.tokens["student"], Command{Action: "handin", ClassID: lateClass, AggregateID: aggregate.ID, Version: 1, AttemptVersion: 1})
	if json.Unmarshal(lateResult, &receipt) != nil || !receipt.Late {
		t.Fatal("server late status missing")
	}
	closedClass, closedActivity, closedRevision := f.assignment(t, "03closed"+unique, 2, "TEXT_AND_FILES", now.Add(-2*time.Hour), now.Add(-time.Hour))
	closedCreated := executeFixture(t, f.store, f.tokens["student"], Command{Action: "create", ClassID: closedClass, ActivityID: closedActivity, AssignmentRevisionID: closedRevision})
	_ = json.Unmarshal(closedCreated, &aggregate)
	executeFixture(t, f.store, f.tokens["student"], Command{Action: "append_revision", ClassID: closedClass, AggregateID: aggregate.ID, Version: 1, Body: "too late"})
	if _, _, err := f.store.Execute(context.Background(), f.tokens["student"], "closedhandin", "request03", Command{Action: "handin", ClassID: closedClass, AggregateID: aggregate.ID, Version: 1, AttemptVersion: 1}); !errors.Is(err, ErrDeadline) {
		t.Fatal("cutoff bypass", err)
	}
	read, err = f.store.Read(context.Background(), f.tokens["student"], closedClass, aggregate.ID)
	if err != nil || read.Attempts[0].State != "DRAFT" || read.Attempts[0].Receipt != nil {
		t.Fatal("cutoff should roll back lock and receipt", err)
	}
}

func TestSyntheticLegacyChronologyLanding(t *testing.T) {
	f := openIntegrationFixture(t, "m08-cp04")
	unique, _ := token(4)
	ctx := context.Background()
	for index, raw := range []string{`{"studentId":"learner","assignmentId":"task","createdAt":"unknown","body":"first"}`, `{"studentId":"learner","assignmentId":"task","createdAt":"unknown","body":"second"}`} {
		_, err := f.owner.Exec(ctx, `INSERT INTO v3.legacy_submission_evidence(source_instance_id,source_table,source_pk,source_version,raw_payload,raw_sha256,raw_time_text) VALUES($1,'submissions',$2,'snapshot-one',$3::jsonb,$4,'unknown')`, "synthetic-"+unique, fmt.Sprintf("legacy-row-%d", index), raw, digest([]byte(raw)))
		if err != nil {
			t.Fatal("landing", err)
		}
	}
	var count, unknown int
	var runtimeRead bool
	if err := f.owner.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE chronology_status='UNKNOWN'),has_table_privilege('qa_runtime','v3.legacy_submission_evidence','SELECT') FROM v3.legacy_submission_evidence WHERE source_instance_id=$1`, "synthetic-"+unique).Scan(&count, &unknown, &runtimeRead); err != nil || count != 2 || unknown != 2 || runtimeRead {
		t.Fatal("raw duplicate chronology was lost or exposed", count, unknown, runtimeRead, err)
	}
	if _, err := f.owner.Exec(ctx, `UPDATE v3.legacy_submission_evidence SET raw_time_text='invented' WHERE source_instance_id=$1`, "synthetic-"+unique); err == nil {
		t.Fatal("legacy raw evidence mutation accepted")
	}
}
