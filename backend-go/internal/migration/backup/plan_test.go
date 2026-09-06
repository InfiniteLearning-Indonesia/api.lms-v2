package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanFailsClosed(t *testing.T) {
	root := t.TempDir()
	good := Request{RunID: "synthetic-run", Source: strings.Repeat("a", 64), Target: strings.Repeat("b", 64), Scope: []string{"public"}, ArtifactRoot: root, RequiredBytes: 100, AvailableBytes: 200, TLSVerified: true, ReadOnlyRole: true, AccessReviewed: true, ApprovalReference: "fixture-not-approval"}
	p := Build(good)
	if p.Status != "PLAN_VALID_NOT_AUTHORIZED" || p.AuthorizesExecution {
		t.Fatal("bad plan boundary")
	}
	if _, err := os.Stat(filepath.Dir(p.Archive)); !os.IsNotExist(err) {
		t.Fatal("planner wrote files")
	}
	for _, mutate := range []func(*Request){
		func(r *Request) { r.Source = r.Target }, func(r *Request) { r.Target = "unknown" },
		func(r *Request) { r.RunID = "../escape" }, func(r *Request) { r.Scope = []string{"public", "public"} },
		func(r *Request) { r.Scope = nil }, func(r *Request) { r.Scope = []string{"pg_catalog"} },
		func(r *Request) { r.AvailableBytes = 1 }, func(r *Request) { r.RequiredBytes = 0 },
		func(r *Request) { r.TLSVerified = false }, func(r *Request) { r.ReadOnlyRole = false },
		func(r *Request) { r.AccessReviewed = false }, func(r *Request) { r.ApprovalReference = "" },
		func(r *Request) { r.ArtifactRoot = "relative" },
	} {
		r := good
		mutate(&r)
		if Build(r).Status != "BLOCKED" {
			t.Fatal("unsafe plan accepted")
		}
	}
	if err := os.Mkdir(filepath.Join(root, good.RunID), 0700); err != nil {
		t.Fatal(err)
	}
	if Build(good).Status != "BLOCKED" {
		t.Fatal("overwrite accepted")
	}
	for _, raw := range []string{`{"unknown":true}`, `{} {}`, `broken`} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, err := Decode(strings.NewReader(`{"run_id":"fixture"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactReceiptRejectsCorruptionAndStaleness(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	scope := strings.Repeat("c", 64)
	r := Receipt{RunID: "fixture", ScopeSHA256: scope, CreatedAt: now}
	for _, name := range []string{"database.dump", "database.sql"} {
		data := []byte("synthetic fixture bytes, NOT a PostgreSQL backup")
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		r.Artifacts = append(r.Artifacts, Artifact{name, Hash(data), int64(len(data))})
	}
	if err := VerifyArtifacts(root, r, scope, now); err != nil {
		t.Fatal(err)
	}
	if VerifyArtifacts(root, r, scope, now.Add(25*time.Hour)) == nil || VerifyArtifacts(root, r, scope, now.Add(-time.Second)) == nil || VerifyArtifacts(root, r, strings.Repeat("d", 64), now) == nil {
		t.Fatal("stale/wrong receipt accepted")
	}
	wrong := r
	wrong.Artifacts = append([]Artifact(nil), r.Artifacts...)
	wrong.Artifacts[0].Name = "../escape"
	if VerifyArtifacts(root, wrong, scope, now) == nil {
		t.Fatal("path escape accepted")
	}
	wrong.Artifacts[0] = wrong.Artifacts[1]
	if VerifyArtifacts(root, wrong, scope, now) == nil {
		t.Fatal("duplicate artifact accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "database.dump"), []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if VerifyArtifacts(root, r, scope, now) == nil {
		t.Fatal("corrupt bytes accepted")
	}
}
