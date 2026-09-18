//go:build qaintegration

package logbook

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/classes"
	"lmsv3/internal/labdb"
)

func installEntries(t *testing.T, ctx context.Context, f *fixture) {
	t.Helper()
	if err := labdb.InstallLogbookEntries(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallLogbookEntries(ctx, f.owner); err != nil {
		t.Fatal("entry migration replay", err)
	}
	_, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT,UPDATE ON v3.logbook_entries TO qa_runtime;
 GRANT SELECT,INSERT ON v3.logbook_entry_revisions,v3.logbook_revision_files,v3.logbook_entry_events,v3.logbook_entry_commands TO qa_runtime;
 GRANT SELECT ON v3.file_assets,v3.mentor_assignments TO qa_runtime`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
}

func installReviews(t *testing.T, ctx context.Context, f *fixture) {
	t.Helper()
	if err := labdb.InstallLogbookReviews(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallLogbookReviews(ctx, f.owner); err != nil {
		t.Fatal("review migration replay", err)
	}
	if _, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT ON v3.logbook_review_commands TO qa_runtime`); err != nil {
		t.Fatal(err)
	}
}

func activePeriod(t *testing.T, ctx context.Context, f *fixture, classID, subject string) string {
	t.Helper()
	now := time.Now().UTC()
	command := Command{Action: "create", ClassID: classID, Title: "Active logbook", Timezone: "UTC", Periods: []Period{{Name: "Active period", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), DueAt: now.Add(24 * time.Hour)}}}
	draft, _, err := f.execute(ctx, subject, mustToken(t), command)
	if err != nil {
		t.Fatal("period draft", err)
	}
	var active string
	_ = f.owner.QueryRow(ctx, "SELECT set_id FROM v3.logbook_period_publications WHERE class_id=$1 ORDER BY revision DESC LIMIT 1", classID).Scan(&active)
	if _, _, err = f.execute(ctx, subject, mustToken(t), Command{Action: "publish", ClassID: classID, SetID: draft.ID, ExpectedActiveID: active}); err != nil {
		t.Fatal("period publish", err)
	}
	return draft.Periods[0].ID
}

func expiredPeriod(t *testing.T, ctx context.Context, f *fixture, classID string) string {
	t.Helper()
	now := time.Now().UTC()
	draft, _, err := f.execute(ctx, "teacher", mustToken(t), Command{Action: "create", ClassID: classID, Title: "Expired logbook", Timezone: "UTC", Periods: []Period{{Name: "Past", StartsAt: now.Add(-72 * time.Hour), EndsAt: now.Add(-48 * time.Hour), DueAt: now.Add(-24 * time.Hour)}}})
	if err != nil {
		t.Fatal(err)
	}
	var active string
	_ = f.owner.QueryRow(ctx, "SELECT set_id FROM v3.logbook_period_publications WHERE class_id=$1 ORDER BY revision DESC LIMIT 1", classID).Scan(&active)
	if _, _, err = f.execute(ctx, "teacher", mustToken(t), Command{Action: "publish", ClassID: classID, SetID: draft.ID, ExpectedActiveID: active}); err != nil {
		t.Fatal(err)
	}
	return draft.Periods[0].ID
}

func evidenceAsset(t *testing.T, ctx context.Context, f *fixture, classID, owner string) string {
	t.Helper()
	id := mustToken(t)
	_, err := f.owner.Exec(ctx, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at) VALUES($1,$2,$3,'EVIDENCE',$4,'proof.txt','text/plain','text/plain',8,$5,'AVAILABLE','CLEAN','CLEAN',$3,clock_timestamp())`, id, classID, owner, id+id, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func executeEntry(t *testing.T, ctx context.Context, f *fixture, subject, key string, c EntryCommand) (EntryView, bool, error) {
	t.Helper()
	request, _ := token()
	raw, replay, err := f.store.ExecuteEntry(ctx, f.tokens[subject], key, request, c)
	if err != nil {
		return EntryView{}, replay, err
	}
	var result struct {
		Entry EntryView `json:"entry"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result.Entry, replay, nil
}

func classAction(t *testing.T, ctx context.Context, f *fixture, subject string, c classes.Command) json.RawMessage {
	t.Helper()
	key, _ := token()
	request, _ := token()
	raw, err := f.classStore.Execute(ctx, f.tokens[subject], key, request, c)
	if err != nil {
		t.Fatal(c.Action, err)
	}
	return raw
}

func TestSyntheticLogbookEntryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	installEntries(t, ctx, f)
	classID := f.class(t, ctx, "Entry lifecycle", true)
	periodID := activePeriod(t, ctx, f, classID, "teacher")
	assetID := evidenceAsset(t, ctx, f, classID, f.users["student"])
	key := mustToken(t)
	command := EntryCommand{Action: "save", ClassID: classID, PeriodID: periodID, Body: "First reflection", FileAssetIDs: []string{assetID}}
	first, replay, err := executeEntry(t, ctx, f, "student", key, command)
	if err != nil || replay || first.State != "DRAFT" || first.Version != 1 || first.RevisionNo != 1 || len(first.FileAssetIDs) != 1 {
		t.Fatal("first draft", err, first)
	}
	again, replay, err := executeEntry(t, ctx, f, "student", key, command)
	if err != nil || !replay || again.RevisionID != first.RevisionID {
		t.Fatal("draft replay", err)
	}
	second, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: classID, PeriodID: periodID, Body: "Second reflection"})
	if err != nil || second.RevisionNo != 2 || second.RevisionCount != 2 || second.Version != 2 {
		t.Fatal("second revision", err, second)
	}
	submitted, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "submit", ClassID: classID, EntryID: second.ID, RevisionID: second.RevisionID, ExpectedVersion: second.Version})
	if err != nil || submitted.State != "SUBMITTED" || submitted.Version != 3 {
		t.Fatal("submit", err, submitted)
	}
	view, err := f.store.ReadEntry(ctx, f.tokens["student"], classID, submitted.ID)
	if err != nil || view.RevisionCount != 2 || view.Body != "Second reflection" {
		t.Fatal("student read", err, view)
	}
	if _, _, err = executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: classID, PeriodID: periodID, Body: "Silent overwrite"}); !errors.Is(err, ErrConflict) {
		t.Fatal("submitted edit accepted", err)
	}
	var revisions, files, events int
	if f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_entry_revisions WHERE entry_id=$1", submitted.ID).Scan(&revisions) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_revision_files WHERE entry_id=$1", submitted.ID).Scan(&files) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_entry_events WHERE entry_id=$1", submitted.ID).Scan(&events) != nil || revisions != 2 || files != 1 || events != 3 {
		t.Fatal("entry history", revisions, files, events)
	}
}

func TestSyntheticLogbookEntryBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	installEntries(t, ctx, f)
	a := f.class(t, ctx, "Entry boundary A", true)
	b := f.class(t, ctx, "Entry boundary B", false)
	period := activePeriod(t, ctx, f, a, "teacher")
	if _, _, err := executeEntry(t, ctx, f, "teacher", mustToken(t), EntryCommand{Action: "save", ClassID: a, PeriodID: period, Body: "Teacher forged"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("teacher entry accepted", err)
	}
	foreignAsset := evidenceAsset(t, ctx, f, b, f.users["student"])
	if _, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: a, PeriodID: period, Body: "Foreign file", FileAssetIDs: []string{foreignAsset}}); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign file accepted", err)
	}
	ownedByTeacher := evidenceAsset(t, ctx, f, a, f.users["teacher"])
	if _, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: a, PeriodID: period, Body: "Other owner", FileAssetIDs: []string{ownedByTeacher}}); !errors.Is(err, ErrConflict) {
		t.Fatal("other-owner file accepted", err)
	}
	expired := expiredPeriod(t, ctx, f, a)
	if _, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: a, PeriodID: expired, Body: "Late"}); !errors.Is(err, ErrConflict) {
		t.Fatal("expired period accepted", err)
	}
	period = activePeriod(t, ctx, f, a, "teacher")
	entry, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: a, PeriodID: period, Body: "Scoped"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ReadEntry(ctx, f.tokens["teacher"], a, entry.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("unassigned teacher read", err)
	}
	if _, err = f.store.ReadEntry(ctx, f.tokens["outsider"], a, entry.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider read", err)
	}
	if _, err = f.store.ReadEntry(ctx, f.tokens["student"], b, entry.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-Class read", err)
	}
	var unsafe bool
	if f.owner.QueryRow(ctx, "SELECT has_table_privilege('qa_runtime','v3.logbook_entries','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.logbook_entry_revisions','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.logbook_entry_events','DELETE,TRUNCATE')").Scan(&unsafe) != nil || unsafe {
		t.Fatal("runtime can erase entry history")
	}
	if _, err = f.runtime.Exec(ctx, "UPDATE v3.logbook_entries SET created_at=created_at WHERE id=$1", entry.ID); err == nil {
		t.Fatal("runtime mutated entry")
	}
}

func assignMentor(t *testing.T, ctx context.Context, f *fixture, classID, mentor string) string {
	t.Helper()
	raw := classAction(t, ctx, f, "admin", classes.Command{Action: "assign_mentor", ClassID: classID, UserID: f.users["student"], MentorID: f.users[mentor]})
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result["id"]
}

func submitFixture(t *testing.T, ctx context.Context, f *fixture, classID string) EntryView {
	t.Helper()
	period := activePeriod(t, ctx, f, classID, "teacher")
	draft, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: classID, PeriodID: period, Body: "Please review"})
	if err != nil {
		t.Fatal(err)
	}
	submitted, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "submit", ClassID: classID, EntryID: draft.ID, RevisionID: draft.RevisionID, ExpectedVersion: draft.Version})
	if err != nil {
		t.Fatal(err)
	}
	return submitted
}

func executeReview(t *testing.T, ctx context.Context, f *fixture, subject, key string, c ReviewCommand) (EntryView, bool, error) {
	t.Helper()
	request, _ := token()
	raw, replay, err := f.store.Review(ctx, f.tokens[subject], key, request, c)
	if err != nil {
		return EntryView{}, replay, err
	}
	var result struct {
		Entry EntryView `json:"entry"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result.Entry, replay, nil
}

func TestSyntheticLogbookReviewLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	installEntries(t, ctx, f)
	installReviews(t, ctx, f)
	classID := f.class(t, ctx, "Review lifecycle", true)
	firstAssignment := assignMentor(t, ctx, f, classID, "teacher")
	submitted := submitFixture(t, ctx, f, classID)
	reviewed, _, err := executeReview(t, ctx, f, "teacher", mustToken(t), ReviewCommand{ClassID: classID, EntryID: submitted.ID, RevisionID: submitted.RevisionID, ExpectedVersion: submitted.Version, Decision: "REVISION_REQUIRED", Feedback: "Add more detail"})
	if err != nil || reviewed.State != "REVISION_REQUIRED" {
		t.Fatal("revision decision", err, reviewed)
	}
	period := reviewed.PeriodID
	revised, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "save", ClassID: classID, PeriodID: period, Body: "Expanded reflection"})
	if err != nil || revised.RevisionNo != 2 || revised.State != "DRAFT" {
		t.Fatal("learner revision", err, revised)
	}
	resubmitted, _, err := executeEntry(t, ctx, f, "student", mustToken(t), EntryCommand{Action: "submit", ClassID: classID, EntryID: revised.ID, RevisionID: revised.RevisionID, ExpectedVersion: revised.Version})
	if err != nil {
		t.Fatal(err)
	}
	classAction(t, ctx, f, "admin", classes.Command{Action: "add", ClassID: classID, UserID: f.users["teacher2"], Role: "teacher"})
	secondAssignment := assignMentor(t, ctx, f, classID, "teacher2")
	if _, _, err = executeReview(t, ctx, f, "teacher", mustToken(t), ReviewCommand{ClassID: classID, EntryID: resubmitted.ID, RevisionID: resubmitted.RevisionID, ExpectedVersion: resubmitted.Version, Decision: "ACCEPTED", Feedback: "Old mentor"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("old mentor review accepted", err)
	}
	accepted, _, err := executeReview(t, ctx, f, "teacher2", mustToken(t), ReviewCommand{ClassID: classID, EntryID: resubmitted.ID, RevisionID: resubmitted.RevisionID, ExpectedVersion: resubmitted.Version, Decision: "ACCEPTED", Feedback: "Complete"})
	if err != nil || accepted.State != "ACCEPTED" || accepted.Version != resubmitted.Version+1 {
		t.Fatal("accept", err, accepted)
	}
	var firstEnded bool
	var eventAssignments int
	if f.owner.QueryRow(ctx, "SELECT ended_at IS NOT NULL FROM v3.mentor_assignments WHERE id=$1", firstAssignment).Scan(&firstEnded) != nil || !firstEnded || f.owner.QueryRow(ctx, "SELECT count(DISTINCT mentor_assignment_id) FROM v3.logbook_entry_events WHERE entry_id=$1 AND mentor_assignment_id IS NOT NULL", accepted.ID).Scan(&eventAssignments) != nil || eventAssignments != 2 || firstAssignment == secondAssignment {
		t.Fatal("mentor provenance", firstEnded, eventAssignments)
	}
	if _, err = f.store.ReadEntry(ctx, f.tokens["teacher"], classID, accepted.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("old mentor retained read", err)
	}
	if current, err := f.store.ReadEntry(ctx, f.tokens["teacher2"], classID, accepted.ID); err != nil || current.State != "ACCEPTED" {
		t.Fatal("current mentor read", err)
	}
}

func TestSyntheticLogbookReviewBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	installEntries(t, ctx, f)
	installReviews(t, ctx, f)
	classID := f.class(t, ctx, "Review boundary", true)
	classAction(t, ctx, f, "admin", classes.Command{Action: "add", ClassID: classID, UserID: f.users["teacher2"], Role: "teacher"})
	assignMentor(t, ctx, f, classID, "teacher")
	submitted := submitFixture(t, ctx, f, classID)
	command := ReviewCommand{ClassID: classID, EntryID: submitted.ID, RevisionID: submitted.RevisionID, ExpectedVersion: submitted.Version, Decision: "ACCEPTED", Feedback: "Good"}
	if _, err := f.owner.Exec(ctx, `INSERT INTO v3.logbook_entry_events(id,entry_id,class_id,revision_id,version,state,actor_id,feedback) VALUES($1,$2,$3,$4,$5,'ACCEPTED',$6,'Bypass')`, mustToken(t), submitted.ID, classID, submitted.RevisionID, submitted.Version+1, f.users["teacher2"]); err == nil {
		t.Fatal("database trigger accepted unassigned reviewer without assignment provenance")
	}
	if _, _, err := executeReview(t, ctx, f, "teacher2", mustToken(t), command); !errors.Is(err, ErrForbidden) {
		t.Fatal("unassigned teacher review", err)
	}
	if _, _, err := executeReview(t, ctx, f, "student", mustToken(t), command); !errors.Is(err, ErrForbidden) {
		t.Fatal("self review", err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(decision, key string) {
			defer wg.Done()
			c := command
			c.Decision = decision
			_, _, err := executeReview(t, ctx, f, "teacher", key, c)
			outcomes <- err
		}([]string{"ACCEPTED", "REVISION_REQUIRED"}[i], mustToken(t))
	}
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal("review race", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("review race count", success, conflict)
	}
	var events, auditEvents, outboxEvents int
	if f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_entry_events WHERE entry_id=$1 AND state IN ('ACCEPTED','REVISION_REQUIRED')", submitted.ID).Scan(&events) != nil || events != 1 || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.audit WHERE class_id=$1 AND action LIKE 'logbook.entry.review.%'", classID).Scan(&auditEvents) != nil || auditEvents != 1 || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.outbox o JOIN v3.audit a ON a.id=o.event_id WHERE a.class_id=$1 AND a.action LIKE 'logbook.entry.review.%'", classID).Scan(&outboxEvents) != nil || outboxEvents != 1 {
		t.Fatal("atomic review", events, auditEvents, outboxEvents)
	}
	var unsafe bool
	if f.owner.QueryRow(ctx, "SELECT has_table_privilege('qa_runtime','v3.logbook_review_commands','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.logbook_entry_events','DELETE,TRUNCATE')").Scan(&unsafe) != nil || unsafe {
		t.Fatal("runtime can erase review history")
	}
}
