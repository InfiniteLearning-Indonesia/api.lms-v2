//go:build qaintegration

package logbook

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
	if err = labdb.InstallLogbookPeriods(ctx, owner); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallLogbookPeriods(ctx, owner); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	_, err = owner.Exec(ctx, `GRANT SELECT,INSERT,UPDATE ON v3.logbook_period_sets TO qa_runtime;
 GRANT SELECT,INSERT ON v3.logbook_periods,v3.logbook_period_publications,v3.logbook_period_commands TO qa_runtime`, pgx.QueryExecModeSimpleProtocol)
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
	for _, subject := range []string{"admin", "teacher", "teacher2", "student", "outsider"} {
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

func (f *fixture) class(t *testing.T, ctx context.Context, name string, withStudent bool) string {
	t.Helper()
	exec := func(action string, c classes.Command) json.RawMessage {
		c.Action = action
		key, _ := token()
		request, _ := token()
		value, err := f.classStore.Execute(ctx, f.tokens["admin"], key, request, c)
		if err != nil {
			t.Fatal(action, err)
		}
		return value
	}
	var created classes.Class
	if err := json.Unmarshal(exec("create", classes.Command{Name: name}), &created); err != nil {
		t.Fatal(err)
	}
	exec("add", classes.Command{ClassID: created.ID, UserID: f.users["teacher"], Role: "teacher"})
	if withStudent {
		exec("add", classes.Command{ClassID: created.ID, UserID: f.users["student"], Role: "student"})
	}
	exec("publish", classes.Command{ClassID: created.ID, Version: 1})
	return created.ID
}

func sample(classID string, count int) Command {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	periods := make([]Period, count)
	for i := range periods {
		begin := start.AddDate(0, i, 0)
		end := begin.AddDate(0, 1, 0)
		periods[i] = Period{Name: "Period " + string(rune('A'+i)), StartsAt: begin, EndsAt: end, DueAt: end.Add(7 * 24 * time.Hour)}
	}
	return Command{Action: "create", ClassID: classID, Title: "Mentoring plan", Timezone: "Asia/Jakarta", Periods: periods}
}

func (f *fixture) execute(ctx context.Context, subject, key string, c Command) (PeriodSet, bool, error) {
	request, _ := token()
	raw, replay, err := f.store.Execute(ctx, f.tokens[subject], key, request, c)
	if err != nil {
		return PeriodSet{}, replay, err
	}
	var response struct {
		Set PeriodSet `json:"set"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return PeriodSet{}, replay, err
	}
	return response.Set, replay, nil
}

func mustToken(t *testing.T) string {
	t.Helper()
	id, err := token()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSyntheticClassPeriodLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	classID := f.class(t, ctx, "Logbook lifecycle", true)
	for _, count := range []int{2, 6, 3} {
		command := sample(classID, count)
		key := mustToken(t)
		draft, replay, err := f.execute(ctx, "teacher", key, command)
		if err != nil || replay || draft.State != "DRAFT" || len(draft.Periods) != count {
			t.Fatal("draft", err, draft)
		}
		again, replay, err := f.execute(ctx, "teacher", key, command)
		if err != nil || !replay || again.ID != draft.ID {
			t.Fatal("replay", err)
		}
		if _, err = f.store.Read(ctx, f.tokens["student"], classID, draft.ID); !errors.Is(err, ErrForbidden) {
			t.Fatal("student read draft", err)
		}
		if viewed, err := f.store.Read(ctx, f.tokens["teacher"], classID, draft.ID); err != nil || viewed.State != "DRAFT" {
			t.Fatal("teacher draft", err)
		}
		current := ""
		var previous string
		_ = f.owner.QueryRow(ctx, "SELECT set_id FROM v3.logbook_period_publications WHERE class_id=$1 ORDER BY revision DESC LIMIT 1", classID).Scan(&previous)
		current = previous
		published, _, err := f.execute(ctx, "teacher", mustToken(t), Command{Action: "publish", ClassID: classID, SetID: draft.ID, ExpectedActiveID: current})
		if err != nil || published.State != "PUBLISHED" || len(published.Periods) != count {
			t.Fatal("publish", err, published)
		}
		active, err := f.store.Read(ctx, f.tokens["student"], classID, "")
		if err != nil || active.ID != draft.ID || len(active.Periods) != count {
			t.Fatal("active", err, active)
		}
	}
	var sets, periods, publications, entries int
	if f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_period_sets WHERE class_id=$1", classID).Scan(&sets) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_periods WHERE class_id=$1", classID).Scan(&periods) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.logbook_period_publications WHERE class_id=$1", classID).Scan(&publications) != nil || f.owner.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='v3' AND table_name='logbook_entries'").Scan(&entries) != nil || sets != 3 || periods != 11 || publications != 3 || entries != 0 {
		t.Fatal("history/accounting", sets, periods, publications, entries)
	}
}

func TestSyntheticPeriodBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	f := openFixture(t, ctx)
	a := f.class(t, ctx, "Logbook boundary A", true)
	b := f.class(t, ctx, "Logbook boundary B", false)
	for _, mutate := range []func(*Command){
		func(c *Command) { c.Timezone = "Local" },
		func(c *Command) { c.Periods[0].EndsAt = c.Periods[0].StartsAt },
		func(c *Command) { c.Periods[0].DueAt = c.Periods[0].EndsAt.Add(-time.Hour) },
		func(c *Command) { c.Periods[1].StartsAt = c.Periods[0].EndsAt.Add(-time.Hour) },
	} {
		c := sample(a, 2)
		mutate(&c)
		if _, _, err := f.execute(ctx, "teacher", mustToken(t), c); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid periods accepted", err)
		}
	}
	if _, _, err := f.execute(ctx, "student", mustToken(t), sample(a, 2)); !errors.Is(err, ErrForbidden) {
		t.Fatal("student edit", err)
	}
	if _, err := f.store.Read(ctx, f.tokens["student"], b, ""); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign read", err)
	}
	if _, err := f.store.Read(ctx, f.tokens["outsider"], a, ""); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider read", err)
	}
	draft, _, err := f.execute(ctx, "teacher", mustToken(t), sample(a, 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.execute(ctx, "teacher", mustToken(t), Command{Action: "publish", ClassID: b, SetID: draft.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-class publish", err)
	}
	if _, _, err := f.execute(ctx, "teacher", mustToken(t), Command{Action: "publish", ClassID: a, SetID: draft.ID, ExpectedActiveID: mustToken(t)}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale publish", err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, e := f.execute(ctx, "teacher", key, Command{Action: "publish", ClassID: a, SetID: draft.ID})
			outcomes <- e
		}(mustToken(t))
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
			t.Fatal("unexpected concurrent result", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("publication race", success, conflict)
	}
	wrongPeriodID := mustToken(t)
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.logbook_periods(id,set_id,class_id,ordinal,name,starts_at,ends_at,due_at) VALUES($1,$2,$3,3,'Forged',clock_timestamp(),clock_timestamp()+interval '1 day',clock_timestamp()+interval '2 days')`, wrongPeriodID, draft.ID, b)
	if err == nil {
		t.Fatal("cross-Class period FK accepted")
	}
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.enrollment_access(enrollment_id,suspended_at) SELECT id,clock_timestamp() FROM v3.enrollments WHERE class_id=$1 AND user_id=$2 ON CONFLICT(enrollment_id) DO UPDATE SET suspended_at=EXCLUDED.suspended_at`, a, f.users["teacher"])
	if err != nil {
		t.Fatal("suspension fixture", err)
	}
	if _, _, err = f.execute(ctx, "teacher", mustToken(t), sample(a, 2)); !errors.Is(err, ErrForbidden) {
		t.Fatal("suspended Teacher wrote periods", err)
	}
	var unsafe bool
	if f.owner.QueryRow(ctx, "SELECT has_table_privilege('qa_runtime','v3.logbook_period_sets','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.logbook_periods','DELETE,TRUNCATE') OR has_table_privilege('qa_runtime','v3.logbook_period_publications','DELETE,TRUNCATE')").Scan(&unsafe) != nil || unsafe {
		t.Fatal("runtime can erase history")
	}
	if _, err = f.runtime.Exec(ctx, "UPDATE v3.logbook_period_sets SET title=title WHERE id=$1", draft.ID); err == nil {
		t.Fatal("immutable set accepted runtime update")
	}
}
