package backup

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func intakeFixture(root string, now time.Time) IntakeRequest {
	return IntakeRequest{Version: 1, RunID: "fixture-baseline", ArtifactRoot: root,
		Source:       IntakeSource{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 40), "fixture-receipt", now.Format(time.RFC3339Nano)},
		Target:       IntakeTarget{strings.Repeat("d", 64), "new-local-restore", "fixture-namespace"},
		Schemas:      []string{"public", "legacy_extra"},
		Declarations: IntakeDeclarations{"verify-full", true, true, "fixture-owner", "fixture-reviewer", "fixture-approval", "fixture-key-ref", "fixture-object-scope"},
		Capacity:     IntakeCapacity{1024, 2048, 0, 4096, MinimumBaselineReserve}}
}

func TestIntakeStrictJSON(t *testing.T) {
	r := intakeFixture(t.TempDir(), time.Now().UTC())
	valid, _ := json.Marshal(r)
	if _, err := DecodeIntake(strings.NewReader(string(valid))); err != nil {
		t.Fatal("valid fixture rejected")
	}
	bad := []string{
		`null`, `[]`, `{}`, `{"version":1,"version":1}`, `{"version":1,"\u0076ersion":1}`,
		`{"source":{"receipt_ref":"a","receipt_ref":"b"}}`,
		strings.Replace(string(valid), `"version":1`, `"VERSION":1`, 1),
		strings.Replace(string(valid), `"receipt_ref":`, `"Receipt_ref":`, 1),
		strings.Replace(string(valid), `"dump_bytes":1024`, `"dump_bytes":-1`, 1),
		strings.Replace(string(valid), `"dump_bytes":1024`, `"dump_bytes":1.5`, 1),
		strings.Replace(string(valid), `"dump_bytes":1024`, `"dump_bytes":18446744073709551616`, 1),
		strings.Replace(string(valid), `"schemas":["public","legacy_extra"]`, `"schemas":null`, 1),
		strings.Replace(string(valid), `"version":1`, `"unknown_secret":"PRIVATE_FIXTURE_MARKER","version":1`, 1),
		string(valid) + `{}`, string(valid) + strings.Repeat(" ", MaxIntakeJSON),
		strings.Repeat(`{"x":`, 18) + `1` + strings.Repeat(`}`, 18),
	}
	for i, raw := range bad {
		if _, err := DecodeIntake(strings.NewReader(raw)); err == nil || err.Error() != "INVALID_INTAKE_INPUT" {
			t.Fatalf("ambiguous input accepted or unsafe error at fixture %d", i)
		}
	}
	bounded := string(valid) + strings.Repeat(" ", MaxIntakeJSON-len(valid))
	if _, err := DecodeIntake(strings.NewReader(bounded)); err != nil {
		t.Fatal("valid exact-size input rejected")
	}
}

func TestIntakeDeclarationsAndBinding(t *testing.T) {
	now := time.Now().UTC()
	r := intakeFixture(t.TempDir(), now)
	free := func(string) (uint64, error) { return math.MaxUint64, nil }
	p := checkIntake(r, now, free)
	if p.Status != "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED" || len(p.PlanSHA256) != 64 {
		t.Fatal("valid local plan rejected")
	}
	reordered := r
	reordered.Schemas = []string{"legacy_extra", "public"}
	if checkIntake(reordered, now, free).PlanSHA256 != p.PlanSHA256 || r.Schemas[0] != "public" {
		t.Fatal("scope ordering changed binding or mutated caller")
	}
	changed := r
	changed.Declarations.ApprovalRef = "different-fixture-approval"
	if checkIntake(changed, now, free).PlanSHA256 == p.PlanSHA256 {
		t.Fatal("changed declaration did not change binding")
	}
	for i, mutate := range []func(*IntakeRequest){
		func(r *IntakeRequest) { r.Version = 2 }, func(r *IntakeRequest) { r.RunID = "../escape" },
		func(r *IntakeRequest) { r.Target.IdentitySHA256 = r.Source.IdentitySHA256 },
		func(r *IntakeRequest) { r.Source.IdentitySHA256 = "unknown" },
		func(r *IntakeRequest) { r.Source.SchemaSHA256 = "unknown" },
		func(r *IntakeRequest) { r.Source.DeployedCommit = "unknown" },
		func(r *IntakeRequest) { r.Source.ReceiptRef = "" },
		func(r *IntakeRequest) { r.Source.ObservedAt = now.Add(time.Second).Format(time.RFC3339Nano) },
		func(r *IntakeRequest) { r.Source.ObservedAt = now.Add(-25 * time.Hour).Format(time.RFC3339Nano) },
		func(r *IntakeRequest) { r.Source.ObservedAt = "2026-11-12T09:00:00+07:00" },
		func(r *IntakeRequest) { r.Target.Kind = "existing-staging" },
		func(r *IntakeRequest) { r.Target.Namespace = "../existing" },
		func(r *IntakeRequest) { r.Schemas = nil },
		func(r *IntakeRequest) { r.Schemas = []string{"public", "public"} },
		func(r *IntakeRequest) { r.Schemas = []string{"pg_catalog"} },
		func(r *IntakeRequest) { r.Schemas = []string{"information_schema"} },
		func(r *IntakeRequest) { r.Schemas = []string{"Public"} },
		func(r *IntakeRequest) { r.Declarations.TLSMode = "require" },
		func(r *IntakeRequest) { r.Declarations.SourceReadOnly = false },
		func(r *IntakeRequest) { r.Declarations.AccessReviewed = false },
		func(r *IntakeRequest) { r.Declarations.OwnerRef = r.Declarations.ReviewerRef },
		func(r *IntakeRequest) { r.Declarations.ApprovalRef = "" },
		func(r *IntakeRequest) { r.Declarations.KeyCustodyRef = "" },
		func(r *IntakeRequest) { r.Declarations.ArtifactScopeRef = "" },
	} {
		bad := r
		mutate(&bad)
		if checkIntake(bad, now, free).Status != "BLOCKED" {
			t.Fatalf("invalid declaration accepted at fixture %d", i)
		}
	}
}

func TestIntakeCapacityAndNoProbe(t *testing.T) {
	now := time.Now().UTC()
	r := intakeFixture(t.TempDir(), now)
	required, ok := intakeCapacity(r.Capacity)
	if !ok || required != MinimumBaselineReserve+1024+2048+4096 {
		t.Fatal("peak capacity components missing")
	}
	for i, mutate := range []func(*IntakeRequest){
		func(r *IntakeRequest) { r.Capacity.DumpBytes = 0 },
		func(r *IntakeRequest) { r.Capacity.SQLBytes = 0 },
		func(r *IntakeRequest) { r.Capacity.RestoreBytes = 0 },
		func(r *IntakeRequest) { r.Capacity.ReserveBytes = MinimumBaselineReserve - 1 },
		func(r *IntakeRequest) { r.Capacity.ObjectBytes = math.MaxUint64 },
		func(r *IntakeRequest) { r.Version = -1 },
	} {
		bad := r
		mutate(&bad)
		p := checkIntake(bad, now, func(string) (uint64, error) { t.Fatal("invalid input reached disk probe"); return 0, nil })
		if p.Status != "BLOCKED" || p.FilesystemChecked {
			t.Fatalf("capacity/validation bypass at fixture %d", i)
		}
	}
	if p := checkIntake(r, now, func(string) (uint64, error) { return required - 1, nil }); p.Status != "BLOCKED" {
		t.Fatal("insufficient actual capacity accepted")
	}
	if p := checkIntake(r, now, func(string) (uint64, error) { return required, nil }); p.Status != "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED" {
		t.Fatal("exact capacity boundary rejected")
	}
	if p := checkIntake(r, now, func(string) (uint64, error) { return 0, errors.New("PRIVATE_DISK_ERROR") }); p.Status != "BLOCKED" || p.FilesystemChecked {
		t.Fatal("unknown capacity accepted")
	}
}

func TestIntakeFilesystemRefusal(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	r := intakeFixture(root, now)
	free := func(string) (uint64, error) { return math.MaxUint64, nil }
	file := filepath.Join(root, "not-directory")
	if err := os.WriteFile(file, []byte("synthetic fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", root + string(filepath.Separator) + ".", file, filepath.Join(root, "missing")} {
		bad := r
		bad.ArtifactRoot = path
		if checkIntake(bad, now, free).Status != "BLOCKED" {
			t.Fatal("unsafe filesystem path accepted")
		}
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "linked-root")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		bad := r
		bad.ArtifactRoot = link
		if checkIntake(bad, now, free).Status != "BLOCKED" {
			t.Fatal("linked root accepted")
		}
	}
	if err := os.Mkdir(filepath.Join(root, r.RunID), 0700); err != nil {
		t.Fatal(err)
	}
	if checkIntake(r, now, free).Status != "BLOCKED" {
		t.Fatal("existing candidate accepted")
	}
}

func TestIntakeNeverAuthorizes(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	r := intakeFixture(root, now)
	p := CheckIntake(r, now)
	if !p.FilesystemChecked || p.ObservedFreeBytes == 0 {
		t.Fatal("actual filesystem was not observed")
	}
	if p.AuthorizesExecution || p.RealBaselineVerified || p.DatabaseAccess != "NONE" || len(p.ExternalGates) != 5 {
		t.Fatal("operator declarations became authority")
	}
	if _, err := os.Lstat(filepath.Join(root, r.RunID)); !os.IsNotExist(err) {
		t.Fatal("planner created candidate")
	}
	data, _ := json.Marshal(p)
	for _, private := range []string{root, r.Declarations.OwnerRef, r.Declarations.KeyCustodyRef, r.Source.ReceiptRef} {
		if strings.Contains(string(data), private) {
			t.Fatal("report echoed private operator input")
		}
	}
}
