//go:build qaintegration

package attendance

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/classes"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type fixture struct {
	owner      *pgxpool.Pool
	runtime    *pgxpool.Pool
	store      *Store
	classStore *classes.Store
	tokens     map[string]string
	users      map[string]string
}

func openFixture(t *testing.T, ctx context.Context) *fixture {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallMeetingCalendar(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallMeetingCalendar(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	_, err = owner.Exec(ctx, `GRANT SELECT,INSERT,UPDATE ON v3.class_meetings TO qa_runtime;
 GRANT SELECT,INSERT ON v3.meeting_exceptions,v3.meeting_commands TO qa_runtime`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	runtime, err := labdb.Runtime(ctx)
	if err != nil {
		owner.Close()
		t.Fatal(err)
	}
	f := &fixture{owner: owner, runtime: runtime, store: New(runtime), classStore: classes.NewContextual(runtime), tokens: map[string]string{}, users: map[string]string{}}
	for _, subject := range []string{"admin", "teacher", "student", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, "UPDATE v3identity.links SET valid_until=GREATEST(valid_until,clock_timestamp()+interval '5 minutes') WHERE user_id=$1 AND active", user); err != nil {
			t.Fatal(err)
		}
		session, _ := identity.RandomToken()
		if _, err = owner.Exec(ctx, `INSERT INTO v3identity.sessions(token_hash,user_id,authority_version,csrf_hash,expires_at,absolute_until) VALUES($1,$2,$3,$4,clock_timestamp()+interval '5 minutes',clock_timestamp()+interval '5 minutes')`, identity.TokenHash(session), user, version, identity.TokenHash(session)); err != nil {
			t.Fatal(err)
		}
		f.tokens[subject], f.users[subject] = session, user
	}
	t.Cleanup(func() { runtime.Close(); owner.Close() })
	return f
}

func (f *fixture) class(t *testing.T, ctx context.Context, name string, withTeacher, withStudent bool) string {
	t.Helper()
	exec := func(action string, command classes.Command) json.RawMessage {
		command.Action = action
		key, _ := token()
		request, _ := token()
		value, err := f.classStore.Execute(ctx, f.tokens["admin"], key, request, command)
		if err != nil {
			t.Fatal(action, err)
		}
		return value
	}
	created := exec("create", classes.Command{Name: name})
	var c classes.Class
	if err := json.Unmarshal(created, &c); err != nil {
		t.Fatal(err)
	}
	if withTeacher {
		exec("add", classes.Command{ClassID: c.ID, UserID: f.users["teacher"], Role: "teacher"})
	}
	if withStudent {
		exec("add", classes.Command{ClassID: c.ID, UserID: f.users["student"], Role: "student"})
	}
	exec("publish", classes.Command{ClassID: c.ID, Version: 1})
	return c.ID
}

func meetingCommand(classID string, start time.Time) Command {
	return Command{Action: "create", ClassID: classID, Title: "Live workshop", LocalDate: start.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02"), Timezone: "Asia/Jakarta", StartsAt: start, EndsAt: start.Add(90 * time.Minute)}
}

func (f *fixture) execute(t *testing.T, ctx context.Context, subject, key string, c Command) (Meeting, bool, error) {
	t.Helper()
	request, _ := token()
	raw, replay, err := f.store.Execute(ctx, f.tokens[subject], key, request, c)
	if err != nil {
		return Meeting{}, replay, err
	}
	var result struct {
		Meeting Meeting `json:"meeting"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result.Meeting, replay, nil
}

func TestSyntheticClassMeetingCalendar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	classID := f.class(t, ctx, "Meeting A", true, true)
	start := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	command := meetingCommand(classID, start)
	key, _ := token()
	created, replay, err := f.execute(t, ctx, "teacher", key, command)
	if err != nil || replay || created.State != "SCHEDULED" || created.Version != 1 {
		t.Fatal("meeting create", err, created)
	}
	again, replay, err := f.execute(t, ctx, "teacher", key, command)
	if err != nil || !replay || again.ID != created.ID {
		t.Fatal("idempotent replay", err)
	}
	if _, _, err = f.execute(t, ctx, "teacher", key, meetingCommand(classID, start.Add(24*time.Hour))); !errors.Is(err, ErrConflict) {
		t.Fatal("changed-key replay accepted", err)
	}
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), meetingCommand(classID, start.Add(30*time.Minute))); !errors.Is(err, ErrConflict) {
		t.Fatal("overlap accepted", err)
	}
	bad := meetingCommand(classID, start)
	bad.LocalDate = "2026-09-20"
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong local date accepted", err)
	}
	bad = meetingCommand(classID, start)
	bad.Timezone = "GMT+7"
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("non-IANA timezone accepted", err)
	}
	bad = meetingCommand(classID, start)
	bad.Timezone = "Local"
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("host-local timezone accepted", err)
	}
	calendar, err := f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || calendar.PlannedMeetings != 1 || calendar.ActiveMeetings != 1 {
		t.Fatal("active denominator", err, calendar)
	}
	cancelled, _, err := f.execute(t, ctx, "teacher", mustToken(t), Command{Action: "cancel", ClassID: classID, MeetingID: created.ID, Version: 1, Reason: "Public holiday"})
	if err != nil || cancelled.Version != 2 || cancelled.State != "CANCELLED" {
		t.Fatal("cancel", err)
	}
	calendar, err = f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || calendar.ActiveMeetings != 0 || calendar.PlannedMeetings != 1 {
		t.Fatal("cancel denominator", err, calendar)
	}
	reinstated, _, err := f.execute(t, ctx, "teacher", mustToken(t), Command{Action: "reinstate", ClassID: classID, MeetingID: created.ID, Version: 2, Reason: "Holiday moved"})
	if err != nil || reinstated.Version != 3 || reinstated.State != "SCHEDULED" {
		t.Fatal("reinstate", err)
	}
	calendar, err = f.store.Read(ctx, f.tokens["student"], classID)
	if err != nil || calendar.ActiveMeetings != 1 || calendar.PlannedMeetings != 1 {
		t.Fatal("reinstate denominator", err, calendar)
	}
	var meetings, exceptions, attendance int
	if f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.class_meetings WHERE class_id=$1", classID).Scan(&meetings) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.meeting_exceptions WHERE meeting_id=$1", created.ID).Scan(&exceptions) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='v3' AND table_name='attendance_records'").Scan(&attendance) != nil || meetings != 1 || exceptions != 2 || attendance != 0 {
		t.Fatal("calendar history/accounting", meetings, exceptions, attendance)
	}
	var unsafe bool
	if f.owner.QueryRow(ctx, "SELECT has_table_privilege('qa_runtime','v3.class_meetings','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.meeting_exceptions','DELETE,TRUNCATE')").Scan(&unsafe) != nil || unsafe {
		t.Fatal("runtime role can erase meeting history")
	}
}

func TestSyntheticMeetingExceptionBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	a := f.class(t, ctx, "Meeting boundary A", true, true)
	b := f.class(t, ctx, "Meeting boundary B", true, false)
	start := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	first, _, err := f.execute(t, ctx, "teacher", mustToken(t), meetingCommand(a, start))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), Command{Action: "cancel", ClassID: b, MeetingID: first.ID, Version: 1, Reason: "Forged class"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-class exception accepted", err)
	}
	if _, _, err = f.execute(t, ctx, "student", mustToken(t), meetingCommand(a, start.Add(24*time.Hour))); !errors.Is(err, ErrForbidden) {
		t.Fatal("student edit accepted", err)
	}
	if _, err = f.store.Read(ctx, f.tokens["student"], b); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign calendar readable", err)
	}
	if _, err = f.store.Read(ctx, f.tokens["outsider"], a); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider calendar readable", err)
	}
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), Command{Action: "cancel", ClassID: a, MeetingID: first.ID, Version: 2, Reason: "Stale"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale exception accepted", err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	keys := []string{mustToken(t), mustToken(t)}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			request, _ := token()
			_, _, e := f.store.Execute(ctx, f.tokens["teacher"], key, request, meetingCommand(a, start.Add(24*time.Hour)))
			outcomes <- e
		}(keys[i])
	}
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for e := range outcomes {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal("parallel create", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("parallel overlap duplicated", success, conflict)
	}
	var forged string
	err = f.owner.QueryRow(ctx, `INSERT INTO v3.meeting_exceptions(id,class_id,meeting_id,revision,kind,reason,actor_id) VALUES($1,$2,$3,1,'CANCELLED','forged',$4) RETURNING id`, mustToken(t), b, first.ID, f.users["admin"]).Scan(&forged)
	if err == nil {
		t.Fatal("cross-Class FK allowed forged exception")
	}
	var enrollment string
	if err = f.owner.QueryRow(ctx, "SELECT id FROM v3.enrollments WHERE class_id=$1 AND user_id=$2", a, f.users["teacher"]).Scan(&enrollment); err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(ctx, "INSERT INTO v3.enrollment_access(enrollment_id,suspended_at) VALUES($1,clock_timestamp())", enrollment); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.execute(t, ctx, "teacher", mustToken(t), meetingCommand(a, start.Add(48*time.Hour))); !errors.Is(err, ErrForbidden) {
		t.Fatal("suspended teacher still writes", err)
	}
}

func mustToken(t *testing.T) string {
	t.Helper()
	value, err := token()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
