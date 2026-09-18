//go:build qaintegration

package classes

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
)

type participationFixture struct {
	store       *Store
	owner       *pgxpool.Pool
	runtime     *pgxpool.Pool
	writer      *pgxpool.Pool
	credentials map[string]identity.Credential
	users       map[string]string
}

func openParticipationFixture(t *testing.T, ctx context.Context, checkpoint string) *participationFixture {
	t.Helper()
	owner, err := labdb.Owner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = labdb.InstallParticipation(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal(err)
	}
	if err = labdb.InstallParticipation(ctx, owner, checkpoint); err != nil {
		owner.Close()
		t.Fatal("migration replay", err)
	}
	grants := `GRANT SELECT,INSERT,UPDATE ON v3.class_groups,v3.group_memberships,v3.mentor_assignments TO qa_runtime;`
	if checkpoint == "m05-cp04" {
		grants += ` GRANT SELECT,INSERT,UPDATE ON v3.invitations TO qa_runtime;`
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
	writer, err := labdb.IdentityRuntime(ctx)
	if err != nil {
		runtime.Close()
		owner.Close()
		t.Fatal(err)
	}
	seedText, err := os.ReadFile(filepath.Join(os.Getenv("QA_LAB_DIR"), "issuer-seed.private"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := hex.DecodeString(string(seedText))
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatal("issuer seed invalid")
	}
	key := ed25519.NewKeyFromSeed(seed)
	service := &identity.Lifecycle{Pool: writer, Verifier: identity.Verifier{Issuer: "https://synthetic-issuer.invalid", Audience: "lms-v3-local", Keys: map[string]ed25519.PublicKey{"fixture-1": key.Public().(ed25519.PublicKey)}}}
	fixture := &participationFixture{store: NewContextual(runtime), owner: owner, runtime: runtime, writer: writer, credentials: map[string]identity.Credential{}, users: map[string]string{}}
	for _, subject := range []string{"admin", "teacher", "teacher2", "student", "student2", "outsider"} {
		var user string
		var version int64
		if err = owner.QueryRow(ctx, "SELECT user_id,version FROM v3identity.links WHERE subject=$1", subject).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		nonce, binding, e := service.Challenge(ctx)
		if e != nil {
			t.Fatal(e)
		}
		jti, _ := identity.RandomToken()
		now := time.Now()
		active := true
		claims := identity.Assertion{RegisteredClaims: jwt.RegisteredClaims{Issuer: service.Verifier.Issuer, Audience: jwt.ClaimStrings{service.Verifier.Audience}, Subject: subject, ID: jti, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, Purpose: "login", Nonce: nonce, Version: version, Active: &active}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = "fixture-1"
		token.Header["typ"] = "lms-bridge+jwt"
		raw, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		credential, e := service.Exchange(ctx, raw, binding)
		if e != nil {
			t.Fatal(e)
		}
		fixture.users[subject] = user
		fixture.credentials[subject] = credential
	}
	t.Cleanup(func() {
		writer.Close()
		runtime.Close()
		owner.Close()
	})
	return fixture
}

func (f *participationFixture) execute(ctx context.Context, subject string, cmd Command) (json.RawMessage, error) {
	id, _ := ID()
	return f.store.Execute(ctx, f.credentials[subject].Token, id, id, cmd)
}

func decodeMap(t *testing.T, raw json.RawMessage) map[string]string {
	t.Helper()
	value := map[string]string{}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func createParticipationClass(t *testing.T, ctx context.Context, f *participationFixture, name string) Class {
	t.Helper()
	raw, err := f.execute(ctx, "admin", Command{Action: "create", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	var class Class
	if json.Unmarshal(raw, &class) != nil {
		t.Fatal("invalid Class")
	}
	return class
}

func addParticipation(t *testing.T, ctx context.Context, f *participationFixture, classID, subject, role string) map[string]string {
	t.Helper()
	raw, err := f.execute(ctx, "admin", Command{Action: "add", ClassID: classID, UserID: f.users[subject], Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return decodeMap(t, raw)
}

func TestSyntheticGroupingAndMentor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := openParticipationFixture(t, ctx, "m05-cp03")
	class := createParticipationClass(t, ctx, f, "M05 CP03")
	addParticipation(t, ctx, f, class.ID, "teacher", "teacher")
	addParticipation(t, ctx, f, class.ID, "teacher2", "teacher")
	addParticipation(t, ctx, f, class.ID, "teacher2", "student")
	student := addParticipation(t, ctx, f, class.ID, "student", "student")
	optional := addParticipation(t, ctx, f, class.ID, "student2", "student")

	groupA, err := f.execute(ctx, "teacher", Command{Action: "create_group", ClassID: class.ID, Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := f.execute(ctx, "teacher", Command{Action: "create_group", ClassID: class.ID, Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	a, b := decodeMap(t, groupA), decodeMap(t, groupB)
	if _, err = f.execute(ctx, "teacher", Command{Action: "create_group", ClassID: class.ID, Name: " alpha "}); !errors.Is(err, ErrConflict) {
		t.Fatal("case-insensitive duplicate group accepted", err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_group", ClassID: class.ID, GroupID: a["id"], UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_group", ClassID: class.ID, GroupID: b["id"], UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_mentor", ClassID: class.ID, UserID: f.users["student"], MentorID: f.users["teacher"]}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_mentor", ClassID: class.ID, UserID: f.users["student"], MentorID: f.users["teacher2"]}); err != nil {
		t.Fatal(err)
	}

	foreign := createParticipationClass(t, ctx, f, "Foreign M05")
	addParticipation(t, ctx, f, foreign.ID, "teacher", "teacher")
	foreignGroup, err := f.execute(ctx, "admin", Command{Action: "create_group", ClassID: foreign.ID, Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	foreignID := decodeMap(t, foreignGroup)["id"]
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_group", ClassID: class.ID, GroupID: foreignID, UserID: f.users["student"]}); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign group accepted", err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "assign_mentor", ClassID: foreign.ID, UserID: f.users["student"], MentorID: f.users["teacher2"]}); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign participation accepted", err)
	}
	if _, err = f.execute(ctx, "outsider", Command{Action: "create_group", ClassID: class.ID, Name: "Intrusion"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider group creation accepted", err)
	}

	var total, active int
	if err = f.owner.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE ended_at IS NULL) FROM v3.group_memberships WHERE enrollment_id=$1", student["enrollment_id"]).Scan(&total, &active); err != nil || total != 2 || active != 1 {
		t.Fatal("group reassignment history invalid", total, active, err)
	}
	if err = f.owner.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE ended_at IS NULL) FROM v3.mentor_assignments WHERE student_enrollment_id=$1", student["enrollment_id"]).Scan(&total, &active); err != nil || total != 2 || active != 1 {
		t.Fatal("mentor reassignment history invalid", total, active, err)
	}
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.group_memberships WHERE enrollment_id=$1", optional["enrollment_id"]).Scan(&total); err != nil || total != 0 {
		t.Fatal("optional group became mandatory")
	}
	badID, _ := ID()
	_, err = f.owner.Exec(ctx, "INSERT INTO v3.group_memberships(id,class_id,group_id,enrollment_id,assigned_by) VALUES($1,$2,$3,$4,$5)", badID, class.ID, foreignID, optional["enrollment_id"], f.users["admin"])
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "23503" {
		t.Fatal("same-Class FK fence missing", err)
	}
}

func TestSyntheticInvitationRetakeAndProvenance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	f := openParticipationFixture(t, ctx, "m05-cp04")
	class := createParticipationClass(t, ctx, f, "M05 CP04 target")
	addParticipation(t, ctx, f, class.ID, "teacher", "teacher")

	invited, err := f.execute(ctx, "teacher", Command{Action: "invite", ClassID: class.ID, UserID: f.users["student"], Role: "student"})
	if err != nil {
		t.Fatal(err)
	}
	invitation := decodeMap(t, invited)
	if _, err = f.execute(ctx, "outsider", Command{Action: "accept_invitation", ClassID: class.ID, InvitationID: invitation["id"]}); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign invitation acceptance succeeded", err)
	}
	key, _ := ID()
	request, _ := ID()
	accepted, err := f.store.Execute(ctx, f.credentials["student"].Token, key, request, Command{Action: "accept_invitation", ClassID: class.ID, InvitationID: invitation["id"]})
	if err != nil {
		t.Fatal(err)
	}
	retryRequest, _ := ID()
	retried, err := f.store.Execute(ctx, f.credentials["student"].Token, key, retryRequest, Command{Action: "accept_invitation", ClassID: class.ID, InvitationID: invitation["id"]})
	if err != nil || !reflect.DeepEqual(decodeMap(t, accepted), decodeMap(t, retried)) {
		t.Fatal("invitation retry changed outcome", err)
	}
	acceptedMap := decodeMap(t, accepted)
	initialEpisode := acceptedMap["episode_id"]
	if initialEpisode == "" {
		t.Fatal("initial episode missing")
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "invite", ClassID: class.ID, UserID: f.users["teacher2"], Role: "teacher"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("teacher invited staff", err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "end", ClassID: class.ID, UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}
	retake, err := f.execute(ctx, "teacher", Command{Action: "start_episode", ClassID: class.ID, UserID: f.users["student"], EpisodeKind: "RETAKE", OriginEpisode: initialEpisode, Policy: "SEPARATE_HISTORY"})
	if err != nil {
		t.Fatal(err)
	}
	retakeID := decodeMap(t, retake)["episode_id"]
	if _, err = f.execute(ctx, "teacher", Command{Action: "end", ClassID: class.ID, UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}

	source := createParticipationClass(t, ctx, f, "M05 CP04 source")
	addParticipation(t, ctx, f, source.ID, "teacher", "teacher")
	sourceStudent := addParticipation(t, ctx, f, source.ID, "student", "student")
	var sourceEpisode string
	if err = f.owner.QueryRow(ctx, "SELECT id FROM v3.episodes WHERE enrollment_id=$1 AND ended_at IS NULL", sourceStudent["enrollment_id"]).Scan(&sourceEpisode); err != nil {
		t.Fatal(err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "end", ClassID: source.ID, UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}
	transfer, err := f.execute(ctx, "teacher", Command{Action: "start_episode", ClassID: class.ID, UserID: f.users["student"], EpisodeKind: "TRANSFER", OriginEpisode: sourceEpisode, Policy: "REVIEW_REQUIRED"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "end", ClassID: class.ID, UserID: f.users["student"]}); err != nil {
		t.Fatal(err)
	}
	equivalent, err := f.execute(ctx, "teacher", Command{Action: "start_episode", ClassID: class.ID, UserID: f.users["student"], EpisodeKind: "EQUIVALENCE", OriginEpisode: sourceEpisode, Policy: "REFERENCE_ONLY"})
	if err != nil {
		t.Fatal(err)
	}
	if decodeMap(t, transfer)["recognition_policy"] != "REVIEW_REQUIRED" || decodeMap(t, equivalent)["recognition_policy"] != "REFERENCE_ONLY" {
		t.Fatal("explicit policy lost")
	}
	if _, err = f.execute(ctx, "teacher", Command{Action: "start_episode", ClassID: class.ID, UserID: f.users["student"], EpisodeKind: "TRANSFER", OriginEpisode: sourceEpisode, Policy: "SEPARATE_HISTORY"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("implicit transfer policy accepted", err)
	}
	var targetEpisodes, originRows int
	if err = f.owner.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE origin_episode_id IS NOT NULL)
 FROM v3.episodes WHERE enrollment_id=$1`, acceptedMap["enrollment_id"]).Scan(&targetEpisodes, &originRows); err != nil || targetEpisodes != 4 || originRows != 3 {
		t.Fatal("episode history/provenance lost", targetEpisodes, originRows, err)
	}
	var retained bool
	if err = f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.episodes WHERE id=$1) AND EXISTS(SELECT 1 FROM v3.episodes WHERE id=$2)", initialEpisode, retakeID).Scan(&retained); err != nil || !retained {
		t.Fatal("prior episode not retained", err)
	}

	concurrentInvite, err := f.execute(ctx, "teacher", Command{Action: "invite", ClassID: class.ID, UserID: f.users["student2"], Role: "student"})
	if err != nil {
		t.Fatal(err)
	}
	concurrentID := decodeMap(t, concurrentInvite)["id"]
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, e := f.execute(ctx, "student2", Command{Action: "accept_invitation", ClassID: class.ID, InvitationID: concurrentID})
			errorsOut <- e
		}()
	}
	wait.Wait()
	close(errorsOut)
	successes, conflicts := 0, 0
	for e := range errorsOut {
		if e == nil {
			successes++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent invitation acceptance was not single-winner", successes, conflicts)
	}
	var orphaned int
	if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM v3.invitations i LEFT JOIN v3.enrollments e ON e.class_id=i.class_id AND e.user_id=i.user_id
 WHERE i.status='ACCEPTED' AND e.id IS NULL`).Scan(&orphaned); err != nil || orphaned != 0 {
		t.Fatal("accepted invitation orphaned", orphaned, err)
	}
}
