//go:build qaintegration

package content

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/classes"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type fixture struct {
	owner   *pgxpool.Pool
	runtime *pgxpool.Pool
	store   *Store
	tokens  map[string]string
	users   map[string]string
}

func openFixture(t *testing.T, ctx context.Context) *fixture {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallContent(ctx, owner, "m07-cp01"); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallContent(ctx, owner, "m07-cp01"); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	if _, err = owner.Exec(ctx, "GRANT SELECT,INSERT,UPDATE ON v3.content_sections,v3.content_activities TO qa_runtime; GRANT SELECT,INSERT ON v3.content_commands TO qa_runtime", pgx.QueryExecModeSimpleProtocol); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	f := &fixture{owner: owner, runtime: runtime, store: New(runtime), tokens: map[string]string{}, users: map[string]string{}}
	for _, subject := range []string{"admin", "teacher", "student", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, "UPDATE v3identity.links SET valid_until=GREATEST(valid_until,clock_timestamp()+interval '5 minutes') WHERE user_id=$1 AND active", user); err != nil {
			t.Fatal(err)
		}
		session, _ := token(32)
		if _, err = owner.Exec(ctx, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until)
		 VALUES($1,$2,$3,$4,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, identity.TokenHash(session), user, version, identity.TokenHash(session)); err != nil {
			t.Fatal(err)
		}
		f.users[subject], f.tokens[subject] = user, session
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return f
}

func openSafetyFixture(t *testing.T, ctx context.Context) *fixture {
	t.Helper()
	f := openFixture(t, ctx)
	if err := labdb.InstallContent(ctx, f.owner, "m07-cp02"); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallContent(ctx, f.owner, "m07-cp02"); err != nil {
		t.Fatal("migration replay", err)
	}
	if _, err := f.owner.Exec(ctx, `GRANT SELECT,INSERT,UPDATE ON v3.content_revisions TO qa_runtime;
		GRANT INSERT,UPDATE ON v3.content_revision_sources TO qa_runtime`, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	return f
}

func openContractFixture(t *testing.T, ctx context.Context) *fixture {
	t.Helper()
	f := openSafetyFixture(t, ctx)
	if err := labdb.InstallContent(ctx, f.owner, "m07-cp03"); err != nil {
		t.Fatal(err)
	}
	if err := labdb.InstallContent(ctx, f.owner, "m07-cp03"); err != nil {
		t.Fatal("migration replay", err)
	}
	if _, err := f.owner.Exec(ctx, `REVOKE UPDATE ON v3.content_revisions FROM qa_runtime;
		GRANT UPDATE(canonical_body,timezone,available_from,due_at,cutoff_at,state,version,updated_at) ON v3.content_revisions TO qa_runtime;
		REVOKE UPDATE ON v3.content_revision_sources FROM qa_runtime;
		GRANT SELECT(revision_id) ON v3.content_revision_sources TO qa_runtime;
		GRANT UPDATE(raw_body,raw_sha256,updated_at) ON v3.content_revision_sources TO qa_runtime;
		GRANT SELECT,INSERT ON v3.assignment_revision_rules TO qa_runtime;
		GRANT UPDATE(submission_mode,max_attempts,max_files,max_file_bytes,late_policy,version,updated_at) ON v3.assignment_revision_rules TO qa_runtime`, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	return f
}

func assignmentRule(mode string, attempts, files int, bytes int64, late string) *AssignmentRuleInput {
	return &AssignmentRuleInput{SubmissionMode: mode, MaxAttempts: attempts, MaxFiles: files, MaxFileBytes: bytes, LatePolicy: late}
}

func (f *fixture) classroom(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	store := classes.NewContextual(f.runtime)
	execute := func(subject string, command classes.Command) json.RawMessage {
		key, _ := token(16)
		request, _ := token(16)
		value, err := store.Execute(ctx, f.tokens[subject], key, request, command)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	created := execute("admin", classes.Command{Action: "create", Name: name})
	var class classes.Class
	if err := json.Unmarshal(created, &class); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ subject, role string }{{"teacher", "teacher"}, {"student", "student"}} {
		execute("admin", classes.Command{Action: "add", ClassID: class.ID, UserID: f.users[row.subject], Role: row.role})
	}
	execute("admin", classes.Command{Action: "publish", ClassID: class.ID, Version: 1})
	return class.ID
}

func execute(t *testing.T, ctx context.Context, store *Store, session string, command Command) (json.RawMessage, bool, error) {
	t.Helper()
	key, _ := token(16)
	request, _ := token(16)
	return store.Execute(ctx, session, key, request, command)
}

func TestSyntheticContentStructure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	classID := f.classroom(t, ctx, "M07 content")
	create := Command{Action: "create_section", ClassID: classID, Title: "Orientation", Position: 1}
	key, _ := token(16)
	request, _ := token(16)
	first, replayed, err := f.store.Execute(ctx, f.tokens["teacher"], key, request, create)
	if err != nil || replayed {
		t.Fatal("section create failed", err)
	}
	replay, replayed, err := f.store.Execute(ctx, f.tokens["teacher"], key, request, create)
	if err != nil || !replayed || string(replay) != string(first) {
		t.Fatal("section idempotency failed", replayed, err)
	}
	var section Section
	if err = json.Unmarshal(first, &section); err != nil {
		t.Fatal(err)
	}
	activityRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_activity", ClassID: classID, SectionID: section.ID, ActivityType: "ASSIGNMENT", Title: "First task", Position: 1})
	if err != nil {
		t.Fatal("activity create failed", err)
	}
	var activity Activity
	if err = json.Unmarshal(activityRaw, &activity); err != nil || activity.SectionID != section.ID || activity.Version != 1 {
		t.Fatal("activity ancestry mismatch", err)
	}
	snapshot, err := f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || len(snapshot.Sections) != 1 || len(snapshot.Activities) != 1 {
		t.Fatal("student content read failed", err)
	}
	if _, _, err = execute(t, ctx, f.store, f.tokens["student"], Command{Action: "create_section", ClassID: classID, Title: "Escape", Position: 2}); !errors.Is(err, ErrForbidden) {
		t.Fatal("student authored content", err)
	}
	if _, err = f.store.Read(ctx, f.tokens["outsider"], classID); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign content disclosed", err)
	}
	if _, _, err = execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "update_section", ClassID: classID, SectionID: section.ID, Title: "Stale", Position: 1, Version: 99}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	secondRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_section", ClassID: classID, Title: "Week 2", Position: 2})
	if err != nil {
		t.Fatal(err)
	}
	var second Section
	_ = json.Unmarshal(secondRaw, &second)
	if _, _, err = execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "update_section", ClassID: classID, SectionID: second.ID, Title: "Collision", Position: 1, Version: 1}); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate order accepted", err)
	}

	commands := []Command{
		{Action: "update_activity", ClassID: classID, ActivityID: activity.ID, Title: "Winner A", Position: 1, Version: 1},
		{Action: "update_activity", ClassID: classID, ActivityID: activity.ID, Title: "Winner B", Position: 1, Version: 1},
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, command := range commands {
		wg.Add(1)
		go func(command Command) {
			defer wg.Done()
			_, _, commandErr := execute(t, ctx, f.store, f.tokens["teacher"], command)
			results <- commandErr
		}(command)
	}
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for result := range results {
		if result == nil {
			succeeded++
		} else if errors.Is(result, ErrConflict) {
			conflicted++
		} else {
			t.Fatal(result)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatal("optimistic concurrency fence failed", succeeded, conflicted)
	}

	badID, _ := token(16)
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by)
	 VALUES($1,$2,$3,'RESOURCE','Foreign',9,$4)`, badID, classID, badID, f.users["teacher"])
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatal("same-Class section FK missing", err)
	}
	var unsafe bool
	if err = f.runtime.QueryRow(ctx, "SELECT has_table_privilege(current_user,'v3.content_sections','DELETE') OR has_table_privilege(current_user,'v3.content_sections','TRUNCATE') OR has_table_privilege(current_user,'v3.content_activities','DELETE') OR has_table_privilege(current_user,'v3.content_activities','TRUNCATE')").Scan(&unsafe); err != nil || unsafe {
		t.Fatal("runtime has destructive content privilege", err)
	}
	var orphanAudit int
	if err = f.runtime.QueryRow(ctx, "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'content.%' AND o.event_id IS NULL").Scan(&orphanAudit); err != nil || orphanAudit != 0 {
		t.Fatal("content audit without outbox", orphanAudit, err)
	}
}

func TestSyntheticSafeContentAvailability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := openSafetyFixture(t, ctx)
	classID := f.classroom(t, ctx, "M07 safe content")
	sectionRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_section", ClassID: classID, Title: "Week 1", Position: 1})
	if err != nil {
		t.Fatal(err)
	}
	var section Section
	_ = json.Unmarshal(sectionRaw, &section)
	activityRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_activity", ClassID: classID, SectionID: section.ID, ActivityType: "ASSIGNMENT", Title: "Task", Position: 1})
	if err != nil {
		t.Fatal(err)
	}
	var activity Activity
	_ = json.Unmarshal(activityRaw, &activity)

	past := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	due := past.Add(2 * time.Hour)
	cutoff := due.Add(time.Hour)
	createdRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_revision", ClassID: classID, ActivityID: activity.ID,
		RawBody: "Read <script>alert(1)</script>\r\ncarefully", Timezone: "Asia/Jakarta", AvailableFrom: &past, DueAt: &due, CutoffAt: &cutoff})
	if err != nil {
		t.Fatal("revision create", err)
	}
	var revision Revision
	if err = json.Unmarshal(createdRaw, &revision); err != nil || revision.State != "DRAFT" || revision.RevisionNo != 1 || strings.Contains(revision.CanonicalBody, "<script>") || !strings.Contains(revision.CanonicalBody, "&lt;script&gt;") {
		t.Fatal("canonical revision invalid", err, revision)
	}
	teacherView, err := f.store.Read(ctx, f.tokens["teacher"], classID)
	if err != nil || len(teacherView.Revisions) != 1 {
		t.Fatal("teacher draft view", err)
	}
	studentView, err := f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || len(studentView.Revisions) != 0 {
		t.Fatal("draft disclosed", err)
	}
	publishedRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "publish_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: revision.ID, Version: revision.Version})
	if err != nil {
		t.Fatal("publish", err)
	}
	var published Revision
	_ = json.Unmarshal(publishedRaw, &published)
	studentView, err = f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || len(studentView.Revisions) != 1 || studentView.Revisions[0].CanonicalBody != published.CanonicalBody {
		t.Fatal("published content unavailable", err)
	}
	if _, _, err = execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "update_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: revision.ID,
		RawBody: "rewrite", Timezone: "Asia/Jakarta", Version: published.Version}); !errors.Is(err, ErrConflict) {
		t.Fatal("published revision mutated", err)
	}
	if _, _, err = execute(t, ctx, f.store, f.tokens["student"], Command{Action: "withdraw_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: revision.ID, Version: published.Version}); !errors.Is(err, ErrForbidden) {
		t.Fatal("student changed lifecycle", err)
	}
	withdrawnRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "withdraw_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: revision.ID, Version: published.Version})
	if err != nil {
		t.Fatal("withdraw", err)
	}
	var withdrawn Revision
	_ = json.Unmarshal(withdrawnRaw, &withdrawn)
	studentView, err = f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || len(studentView.Revisions) != 0 {
		t.Fatal("withdrawn content disclosed", err)
	}

	var raw, rawHash string
	if err = f.owner.QueryRow(ctx, "SELECT raw_body,raw_sha256 FROM v3.content_revision_sources WHERE revision_id=$1", revision.ID).Scan(&raw, &rawHash); err != nil || raw != "Read <script>alert(1)</script>\ncarefully" || rawHash != hash([]byte(raw)) {
		t.Fatal("restricted original not preserved", err)
	}
	var rawReadable bool
	if err = f.runtime.QueryRow(ctx, "SELECT has_table_privilege(current_user,'v3.content_revision_sources','SELECT')").Scan(&rawReadable); err != nil || rawReadable {
		t.Fatal("runtime can read raw source", err)
	}
	if _, err = f.runtime.Exec(ctx, "SELECT raw_body FROM v3.content_revision_sources LIMIT 1"); err == nil {
		t.Fatal("raw source select unexpectedly succeeded")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatal("raw source denial not database-enforced", err)
		}
	}
	var unsafe bool
	if err = f.runtime.QueryRow(ctx, "SELECT has_table_privilege(current_user,'v3.content_revisions','DELETE') OR has_table_privilege(current_user,'v3.content_revisions','TRUNCATE')").Scan(&unsafe); err != nil || unsafe {
		t.Fatal("destructive content privilege present", err)
	}
	var orphan int
	if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id
		WHERE a.action LIKE 'content.%revision' AND o.event_id IS NULL`).Scan(&orphan); err != nil || orphan != 0 {
		t.Fatal("revision audit/outbox orphan", err, orphan)
	}
}

func TestSyntheticAssignmentRevisionStability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := openContractFixture(t, ctx)
	classID := f.classroom(t, ctx, "M07 stable assignment")
	sectionRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_section", ClassID: classID, Title: "Week 1", Position: 1})
	if err != nil {
		t.Fatal(err)
	}
	var section Section
	_ = json.Unmarshal(sectionRaw, &section)
	activityRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "create_activity", ClassID: classID, SectionID: section.ID, ActivityType: "ASSIGNMENT", Title: "Stable task", Position: 1})
	if err != nil {
		t.Fatal(err)
	}
	var activity Activity
	_ = json.Unmarshal(activityRaw, &activity)
	past := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	due := past.Add(2 * time.Hour)
	cutoff := due.Add(time.Hour)
	base := Command{Action: "create_revision", ClassID: classID, ActivityID: activity.ID, RawBody: "Contract v1", Timezone: "Asia/Jakarta", AvailableFrom: &past, DueAt: &due, CutoffAt: &cutoff}
	if _, _, err = execute(t, ctx, f.store, f.tokens["teacher"], base); !errors.Is(err, ErrInvalid) {
		t.Fatal("assignment revision without rule accepted", err)
	}
	base.Assignment = assignmentRule("TEXT", 1, 0, 0, "REJECT_AFTER_CUTOFF")
	createdRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], base)
	if err != nil {
		t.Fatal("create assignment revision", err)
	}
	var first Revision
	if err = json.Unmarshal(createdRaw, &first); err != nil || first.Assignment == nil || first.Assignment.MaxAttempts != 1 {
		t.Fatal("assignment snapshot absent", err, first)
	}
	update := Command{Action: "update_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: first.ID, Version: first.Version,
		RawBody: "Contract v1 edited", Timezone: "Asia/Jakarta", AvailableFrom: &past, DueAt: &due, CutoffAt: &cutoff,
		Assignment: assignmentRule("TEXT_AND_FILES", 2, 2, 2<<20, "ALLOW_MARKED_LATE")}
	updatedRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], update)
	if err != nil {
		t.Fatal("draft contract update", err)
	}
	if _, _, err = execute(t, ctx, f.store, f.tokens["teacher"], update); !errors.Is(err, ErrConflict) {
		t.Fatal("stale draft update accepted", err)
	}
	if err = json.Unmarshal(updatedRaw, &first); err != nil || first.Assignment == nil || first.Assignment.MaxAttempts != 2 {
		t.Fatal("updated contract mismatch", err, first)
	}
	publishedRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "publish_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: first.ID, Version: first.Version})
	if err != nil {
		t.Fatal("publish first contract", err)
	}
	_ = json.Unmarshal(publishedRaw, &first)

	for label, query := range map[string]string{
		"canonical": "UPDATE v3.content_revisions SET canonical_body='repointed',version=version+1,updated_at=clock_timestamp() WHERE id=$1",
		"source":    "UPDATE v3.content_revision_sources SET raw_body='repointed',raw_sha256=repeat('a',64),updated_at=clock_timestamp() WHERE revision_id=$1",
		"rule":      "UPDATE v3.assignment_revision_rules SET max_attempts=99,version=version+1,updated_at=clock_timestamp() WHERE revision_id=$1",
	} {
		_, directErr := f.runtime.Exec(ctx, query, first.ID)
		var pgErr *pgconn.PgError
		if !errors.As(directErr, &pgErr) || pgErr.Code != "23514" {
			t.Fatal(label, "published mutation not database-enforced", directErr)
		}
	}

	secondCommand := base
	secondCommand.RawBody = "Contract v2"
	secondCommand.Assignment = assignmentRule("FILES", 4, 3, 1<<20, "REJECT_AFTER_CUTOFF")
	secondRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], secondCommand)
	if err != nil {
		t.Fatal("create second revision", err)
	}
	var second Revision
	_ = json.Unmarshal(secondRaw, &second)
	secondRaw, _, err = execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "publish_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: second.ID, Version: second.Version})
	if err != nil {
		t.Fatal("publish second revision", err)
	}
	_ = json.Unmarshal(secondRaw, &second)
	if first.ID == second.ID || first.RevisionNo+1 != second.RevisionNo {
		t.Fatal("new publication replaced prior identity", first, second)
	}
	oldContract, err := f.store.ResolveAssignmentContract(ctx, f.tokens["student"], classID, activity.ID, first.ID)
	if err != nil || oldContract.Revision.CanonicalBody != "Contract v1 edited" || oldContract.Rule.MaxAttempts != 2 || oldContract.Rule.SubmissionMode != "TEXT_AND_FILES" {
		t.Fatal("old learner reference repointed", err, oldContract)
	}
	newContract, err := f.store.ResolveAssignmentContract(ctx, f.tokens["student"], classID, activity.ID, second.ID)
	if err != nil || newContract.Rule.MaxAttempts != 4 || newContract.Revision.ID != second.ID {
		t.Fatal("new learner reference mismatch", err, newContract)
	}
	if _, err = f.store.ResolveAssignmentContract(ctx, f.tokens["outsider"], classID, activity.ID, first.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign learner resolved contract", err)
	}
	withdrawnRaw, _, err := execute(t, ctx, f.store, f.tokens["teacher"], Command{Action: "withdraw_revision", ClassID: classID, ActivityID: activity.ID, RevisionID: first.ID, Version: first.Version})
	if err != nil {
		t.Fatal("withdraw old revision", err)
	}
	_ = json.Unmarshal(withdrawnRaw, &first)
	if _, err = f.store.ResolveAssignmentContract(ctx, f.tokens["student"], classID, activity.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("withdrawn contract disclosed to general student", err)
	}
	oldContract, err = f.store.ResolveAssignmentContract(ctx, f.tokens["teacher"], classID, activity.ID, first.ID)
	if err != nil || oldContract.Revision.State != "WITHDRAWN" || oldContract.Rule.MaxAttempts != 2 {
		t.Fatal("withdraw changed retained author contract", err, oldContract)
	}
	var unsafe, orphan bool
	if err = f.runtime.QueryRow(ctx, `SELECT has_table_privilege(current_user,'v3.assignment_revision_rules','DELETE') OR has_table_privilege(current_user,'v3.assignment_revision_rules','TRUNCATE')`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("runtime has destructive assignment privilege", err)
	}
	if err = f.owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'content.%revision' AND o.event_id IS NULL)`).Scan(&orphan); err != nil || orphan {
		t.Fatal("assignment audit/outbox orphan", err)
	}
}
