//go:build password_rehearsal

package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/classes"
	"lmsv3/internal/content"
	"lmsv3/internal/files"
	"lmsv3/internal/gradebook"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"lmsv3/internal/submissions"
)

func installClassInvitationFixture(t *testing.T, ctx context.Context, owner *pgxpool.Pool) {
	t.Helper()
	versions := map[string]bool{
		"cp12-1": true, "m05-cp03-1": true, "m05-cp04-1": true,
		"m06-cp01-1": true, "m07-cp01-1": true, "m07-cp02-1": true, "m07-cp03-1": true,
		"m08-cp01-1": true, "m08-cp02-1": true, "m09-cp01-1": true,
	}
	installed := 0
	for _, migration := range labdb.Catalogue() {
		if versions[migration.Version] {
			if _, err := owner.Exec(ctx, migration.SQL, pgx.QueryExecModeSimpleProtocol); err != nil {
				t.Fatalf("fixture migration %s failed: %T", migration.Version, err)
			}
			installed++
		}
	}
	if installed != len(versions) {
		t.Fatal("fixture dependency missing")
	}
	fixtureExec(t, ctx, owner, `GRANT SELECT ON v3.classes,v3.enrollments,v3.episodes,v3.roles,v3.class_access,v3.enrollment_access,
 v3.commands,v3.invitations,v3.file_assets,v3.content_sections,v3.content_activities,v3.content_revisions,v3.assignment_revision_rules,
 v3.submission_aggregates,v3.submission_attempts,v3.submission_revisions,v3.submission_revision_files,
 v3.grade_items,v3.grade_scales,v3.grade_definition_versions,v3.rubric_criteria TO m04_password_writer;
 GRANT INSERT ON v3.classes,v3.enrollments,v3.episodes,v3.roles,v3.invitations,v3.transition_events,v3.commands,v3.class_access TO m04_password_writer;
 GRANT UPDATE (state,version) ON v3.classes TO m04_password_writer;
 GRANT UPDATE (status) ON v3.enrollments TO m04_password_writer;
 GRANT UPDATE (status,accepted_at,accepted_episode_id) ON v3.invitations TO m04_password_writer;
 GRANT UPDATE (closed_at) ON v3.class_access TO m04_password_writer;
 CREATE FUNCTION v3.fixture_invitation_audit_rollback() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
 BEGIN
 IF NEW.action='invite_account' AND EXISTS(SELECT 1 FROM v3.classes WHERE id=NEW.class_id AND name='Rollback fixture') THEN
  RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='SYNTHETIC_AUDIT_FAILURE';
 END IF;
 RETURN NEW;
 END $$;
 CREATE TRIGGER fixture_invitation_audit_rollback BEFORE INSERT ON v3.audit FOR EACH ROW EXECUTE FUNCTION v3.fixture_invitation_audit_rollback();`, pgx.QueryExecModeSimpleProtocol)
}

type classFixtureResources struct{ material, evidence, draft, activity, submission, gradeItem string }

func seedClassFixtureResources(t *testing.T, ctx context.Context, owner *pgxpool.Pool, classID, teacher, learner, enrollment, episode string) classFixtureResources {
	t.Helper()
	newID := func() string {
		t.Helper()
		id, err := classes.ID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	res := classFixtureResources{newID(), newID(), newID(), newID(), newID(), newID()}
	for _, file := range []struct{ id, purpose, owner string }{{res.material, "MATERIAL", teacher}, {res.evidence, "EVIDENCE", teacher}} {
		fixtureExec(t, ctx, owner, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,
 detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at)
 VALUES($1,$2,$3,$4,$5,'fixture.txt','text/plain','text/plain',12,$6,'AVAILABLE','CLEAN','FIXTURE',$3,now())`,
			file.id, classID, file.owner, file.purpose, identity.TokenHash(file.id), identity.TokenHash("fixture"))
	}
	section, revision := newID(), newID()
	fixtureExec(t, ctx, owner, `INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($1,$2,'Fixture',1,$3)`, section, classID, teacher)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by)
 VALUES($1,$2,$3,'ASSIGNMENT','Fixture assignment',1,$4)`, res.activity, classID, section, teacher)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.content_revisions(id,class_id,activity_id,revision_no,canonical_body,content_format,timezone,created_by)
 VALUES($1,$2,$3,1,'<p>Published fixture</p>','SAFE_HTML','UTC',$4),($5,$2,$3,2,'<p>Draft fixture</p>','SAFE_HTML','UTC',$4)`, revision, classID, res.activity, teacher, res.draft)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.assignment_revision_rules(revision_id,class_id,activity_id,submission_mode,max_attempts,max_files,max_file_bytes,late_policy,created_by)
 VALUES($1,$2,$3,'TEXT',2,0,0,'REJECT_AFTER_CUTOFF',$4)`, revision, classID, res.activity, teacher)
	fixtureExec(t, ctx, owner, `UPDATE v3.content_revisions SET state='PUBLISHED',version=version+1,updated_at=clock_timestamp() WHERE id=$1`, revision)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$7)`, res.submission, classID, res.activity, revision, enrollment, episode, learner)
	scale := newID()
	fixtureExec(t, ctx, owner, `INSERT INTO v3.grade_scales(id,class_id,name,minimum_milli,maximum_milli,display_decimals,created_by)
 VALUES($1,$2,'Fixture scale',0,100000,1,$3)`, scale, classID, teacher)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.grade_items(id,class_id,activity_id,title,created_by) VALUES($1,$2,$3,'Fixture grade item',$4)`, res.gradeItem, classID, res.activity, teacher)
	fixtureExec(t, ctx, owner, `INSERT INTO v3.grade_definition_versions(id,grade_item_id,class_id,version,scale_id,weight_bps,created_by)
 VALUES($1,$2,$3,1,$4,10000,$5)`, newID(), res.gradeItem, classID, scale, teacher)
	return res
}

func TestM04ClassInvitationPostgresJourney(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	owner, lifecycle, passwords, sessions, csrf, _ := newPasswordFixture(t, ctx)
	installClassInvitationFixture(t, ctx, owner)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	objects, err := files.NewLocalStore(filepath.Join(filepath.Dir(os.Getenv("QA_M04_CLUSTER")), "objects"))
	if err != nil {
		t.Fatal("fixture object directory failed")
	}
	fileStore, err := files.New(lifecycle.Pool, objects, files.FixtureScanner{}, time.Second)
	if err != nil {
		t.Fatal("fixture file store failed")
	}
	classHandler := classes.Handler(classes.NewWithAccountInvitations(lifecycle.Pool, passwords), logger)
	fileHandler := files.Handler(fileStore, logger)
	contentHandler := content.Handler(content.New(lifecycle.Pool), logger)
	submissionHandler := submissions.Handler(submissions.New(lifecycle.Pool), logger)
	gradeHandler := gradebook.Handler(gradebook.New(lifecycle.Pool), logger)
	business := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case files.OwnsPath(path):
			fileHandler.ServeHTTP(w, r)
		case submissions.OwnsPath(path):
			submissionHandler.ServeHTTP(w, r)
		case content.OwnsPath(path):
			contentHandler.ServeHTTP(w, r)
		case gradebook.OwnsPath(path):
			gradeHandler.ServeHTTP(w, r)
		default:
			classHandler.ServeHTTP(w, r)
		}
	})
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	server.Config.Handler = identity.GatewayWithPasswords(lifecycle, passwords, origin, business, logger)
	server.StartTLS()
	defer server.Close()

	type actor struct{ session, csrf string }
	type result struct {
		status  int
		session string
		body    []byte
	}
	request := func(method, path string, who actor, key string, body map[string]any) result {
		data, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(data))
		if err != nil {
			return result{}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", who.csrf)
		req.Header.Set("Idempotency-Key", key)
		if who.session != "" {
			req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: who.session})
		}
		res, err := server.Client().Do(req)
		if err != nil {
			return result{}
		}
		defer res.Body.Close()
		var out result
		out.status = res.StatusCode
		out.body, _ = io.ReadAll(io.LimitReader(res.Body, 64<<10))
		for _, cookie := range res.Cookies() {
			if cookie.Name == identity.SessionCookie && cookie.Secure && cookie.HttpOnly {
				out.session = cookie.Value
			}
		}
		return out
	}
	req := func(want int, method, path string, who actor, key string, body map[string]any) result {
		t.Helper()
		out := request(method, path, who, key, body)
		if out.status != want {
			t.Fatalf("%s %s returned %d, expected %d", method, path, out.status, want)
		}
		return out
	}
	value := func(r result, name string) string {
		t.Helper()
		var m map[string]json.RawMessage
		var out string
		if json.Unmarshal(r.body, &m) != nil || json.Unmarshal(m[name], &out) != nil || out == "" {
			t.Fatalf("response field missing: %s", name)
		}
		return out
	}
	admin, existingTeacher, anonymous := actor{sessions[0], csrf[0]}, actor{sessions[1], csrf[1]}, actor{}
	newClass := func(name, key string) string {
		t.Helper()
		return "/v3/classes/" + value(req(200, "POST", "/v3/classes", admin, key, map[string]any{"name": name}), "id")
	}
	a, b, rollback := newClass("Class A", "new-class-a"), newClass("Class B", "new-class-b"), newClass("Rollback fixture", "new-rollback")
	mentorEmail := "class-mentor@example.invalid"
	menteeEmail := "class-mentee@example.invalid"
	mentorInput := map[string]any{"email": mentorEmail, "role": "teacher"}
	post := func(want int, path string, who actor, key string, body map[string]any) result {
		t.Helper()
		return req(want, "POST", path, who, key, body)
	}
	get := func(want int, path string, who actor) result {
		t.Helper()
		return req(want, "GET", path, who, "", nil)
	}
	post(403, a+"/account-invitations", existingTeacher, "no-provision", mentorInput)
	post(403, a+"/account-invitations", actor{session: admin.session}, "csrf-required", mentorInput)
	post(400, a+"/account-invitations", admin, "invalid-role", map[string]any{"email": mentorEmail, "role": "admin"})
	failedEmail := "rollback@example.invalid"
	before := func() [4]int {
		t.Helper()
		var out [4]int
		if err := owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.users),(SELECT count(*) FROM v3password.deliveries),
 (SELECT count(*) FROM v3.audit),(SELECT count(*) FROM v3.outbox)`).Scan(&out[0], &out[1], &out[2], &out[3]); err != nil {
			t.Fatal("fixture audit counts unavailable")
		}
		return out
	}()
	post(503, rollback+"/account-invitations", admin, "rollback-invite", map[string]any{"email": failedEmail, "role": "student"})
	var leaked int
	if err := owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.users)-$1+(SELECT count(*) FROM v3password.deliveries)-$2+
 (SELECT count(*) FROM v3.audit)-$3+(SELECT count(*) FROM v3.outbox)-$4+(SELECT count(*) FROM v3.invitations)`,
		before[0], before[1], before[2], before[3]).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatal("Class failure did not roll back provisioning and evidence")
	}

	mentorInvite := post(200, a+"/account-invitations", admin, "invite-mentor-a", mentorInput)
	mentorID, mentorInviteID := value(mentorInvite, "user_id"), value(mentorInvite, "id")
	menteeInvite := post(200, a+"/account-invitations", admin, "invite-mentee-a", map[string]any{"email": menteeEmail, "role": "student"})
	menteeID, menteeInviteID := value(menteeInvite, "user_id"), value(menteeInvite, "id")
	pendingMentor := fixtureDelivery(t, ctx, owner, passwords, mentorID, "invite")
	mentorB := post(200, b+"/account-invitations", admin, "invite-mentor-b", map[string]any{"email": "CLASS-MENTOR@example.invalid", "role": "student"})
	if value(mentorB, "user_id") != mentorID || fixtureDelivery(t, ctx, owner, passwords, mentorID, "invite") != pendingMentor {
		t.Fatal("pending account or account activation token changed across Classes")
	}
	repeated := post(200, a+"/account-invitations", admin, "invite-mentor-retry", mentorInput)
	if value(repeated, "id") != mentorInviteID || value(repeated, "user_id") != mentorID {
		t.Fatal("pending Class invitation was duplicated")
	}
	activate := func(email, activationToken string) actor {
		t.Helper()
		post(200, "/v3/auth/password/invite/accept", anonymous, "", map[string]any{"token": activationToken, "password": firstPassword})
		login := post(200, "/v3/auth/password/login", anonymous, "", map[string]any{"email": email, "password": firstPassword})
		if login.session == "" {
			t.Fatal("fixture login session missing")
		}
		return actor{login.session, value(login, "csrf_token")}
	}
	mentor := activate(mentorEmail, pendingMentor)
	mentee := activate(menteeEmail, fixtureDelivery(t, ctx, owner, passwords, menteeID, "invite"))
	activeReuse := post(200, b+"/account-invitations", admin, "reuse-active", map[string]any{"email": mentorEmail, "role": "student"})
	if value(activeReuse, "user_id") != mentorID || value(activeReuse, "id") != value(mentorB, "id") {
		t.Fatal("active account not reused")
	}
	get(403, a, mentor)
	pending := get(200, "/v3/me/class-invitations", mentor).body
	if !bytes.Contains(pending, []byte(mentorInviteID)) || !bytes.Contains(pending, []byte(value(mentorB, "id"))) || bytes.Contains(pending, []byte(menteeInviteID)) {
		t.Fatal("pending invitation list is not scoped to the session")
	}
	get(401, "/v3/me/class-invitations", anonymous)
	acceptPath := func(classPath, invitation string) string { return classPath + "/invitations/" + invitation + "/accept" }
	post(403, acceptPath(a, mentorInviteID), mentee, "foreign-account", map[string]any{})
	post(403, acceptPath(b, mentorInviteID), mentor, "foreign-class", map[string]any{})
	post(400, acceptPath(a, menteeInviteID), mentee, "forbidden-role", map[string]any{"role": "teacher"})
	probe := exec.CommandContext(ctx, os.Getenv("QA_M04_PYTHON"), "-B", os.Getenv("QA_M04_CLASS_PROBE"))
	probeInput, _ := json.Marshal(map[string]string{"origin": origin, "class_path": a, "invitation_id": menteeInviteID, "session": mentee.session, "csrf": mentee.csrf})
	probe.Stdin = bytes.NewReader(probeInput)
	if output, err := probe.Output(); err != nil || strings.TrimSpace(string(output)) != "PASS_M04_CLASS_PLAYWRIGHT_5" {
		t.Fatal("Playwright Class invitation smoke failed")
	}
	accepted := post(200, acceptPath(a, mentorInviteID), mentor, "accept-mentor-a", map[string]any{})
	replay := post(200, acceptPath(a, mentorInviteID), mentor, "accept-mentor-a", map[string]any{})
	var acceptedFields, replayFields map[string]string
	if json.Unmarshal(accepted.body, &acceptedFields) != nil || json.Unmarshal(replay.body, &replayFields) != nil ||
		!maps.Equal(acceptedFields, replayFields) || acceptedFields["role"] != "teacher" {
		t.Fatal("same-key acceptance replay changed outcome or role")
	}
	post(403, a+"/account-invitations", mentor, "same-class-teacher", map[string]any{"email": failedEmail, "role": "student"})
	post(409, acceptPath(a, mentorInviteID), mentor, "accept-again", map[string]any{})
	acceptedLearner := post(200, acceptPath(a, menteeInviteID), mentee, "accept-mentee-a", map[string]any{})
	if value(acceptedLearner, "role") != "student" || bytes.Contains(get(200, "/v3/me/class-invitations", mentee).body, []byte(menteeInviteID)) {
		t.Fatal("completed Student invitation still pending or wrong role")
	}
	post(200, a+"/publish", mentor, "publish-class-a", map[string]any{"version": 1})
	post(200, acceptPath(b, value(mentorB, "id")), mentor, "accept-mentor-b", map[string]any{})
	post(403, b+"/publish", mentor, "publish-foreign", map[string]any{"version": 1})
	post(200, b+"/archive", admin, "archive-class-b", map[string]any{"version": 1})
	post(409, b+"/account-invitations", admin, "archived-invite", map[string]any{"email": failedEmail, "role": "teacher"})
	var deniedAccount bool
	if err := owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3password.accounts WHERE email=$1)`, failedEmail).Scan(&deniedAccount); err != nil || deniedAccount {
		t.Fatal("denied invitation provisioned an account")
	}

	idA := strings.TrimPrefix(a, "/v3/classes/")
	res := seedClassFixtureResources(t, ctx, owner, idA, mentorID, menteeID, value(acceptedLearner, "enrollment_id"), value(acceptedLearner, "episode_id"))
	get(200, "/v3/files/"+res.material, mentee)
	get(200, "/v3/files/"+res.evidence, mentor)
	get(403, "/v3/files/"+res.evidence, mentee)
	get(403, "/v3/files/"+res.material, existingTeacher)
	mentorContent, studentContent := get(200, a+"/content", mentor), get(200, a+"/content", mentee)
	if !bytes.Contains(mentorContent.body, []byte(res.draft)) || bytes.Contains(studentContent.body, []byte(res.draft)) {
		t.Fatal("Class draft revision scope mismatch")
	}
	get(403, a+"/content", existingTeacher)
	get(200, a+"/submissions/"+res.submission, mentor)
	get(200, a+"/submissions/"+res.submission, mentee)
	get(403, a+"/submissions/"+res.submission, existingTeacher)
	get(404, b+"/submissions/"+res.submission, mentor)
	grade := get(200, a+"/grade-items/"+res.gradeItem, mentor)
	if !bytes.Contains(grade.body, []byte(res.gradeItem)) {
		t.Fatal("same-Class Teacher gradebook definition missing")
	}
	get(403, a+"/grade-items/"+res.gradeItem, mentee)
	get(403, a+"/grade-items/"+res.gradeItem, existingTeacher)
	get(403, b+"/grade-items/"+res.gradeItem, mentor)
	post(200, a+"/close", admin, "close-class-a", map[string]any{"version": 2})
	post(409, a+"/account-invitations", admin, "closed-invite", map[string]any{"email": failedEmail, "role": "teacher"})
	var acceptedCount, episodes, roles, inviteEvents int
	if err := owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.invitations WHERE status='ACCEPTED'),
 (SELECT count(*) FROM v3.episodes ep JOIN v3.enrollments e ON e.id=ep.enrollment_id WHERE e.user_id IN ($1,$2)),
 (SELECT count(*) FROM v3.roles r JOIN v3.enrollments e ON e.id=r.enrollment_id WHERE e.user_id IN ($1,$2)),
 (SELECT count(*) FROM v3.audit a JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='invite_account')`,
		mentorID, menteeID).Scan(&acceptedCount, &episodes, &roles, &inviteEvents); err != nil || acceptedCount != 3 || episodes != 3 || roles != 3 || inviteEvents != 3 {
		t.Fatal("Class invitations, episodes, role, or audit/outbox did not reconcile")
	}
}
