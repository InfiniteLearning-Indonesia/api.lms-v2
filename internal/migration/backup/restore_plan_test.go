package backup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func restoreFixture(t *testing.T) (RestorePlanRequest, time.Time) {
	r, now := packageFixture(t)
	return RestorePlanRequest{Version: 1, Package: r, Restore: RestoreDeclarations{TargetIdentitySHA256: r.Intake.Target.IdentitySHA256,
		DatabaseName: "v3_restore_fixture", TargetKind: "new-local-restore", PostgresMajor: 18,
		AvailableExtensions: []InventoryExtension{{"plpgsql", "1.0"}}, NetworkDisabled: true, JobsDisabled: true,
		ExecutableReviewed: true, ReviewRef: "fixture-review-claim"}}, now
}

func TestRestorePlanIsolationAndVersions(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	for _, change := range []func(*RestorePlanRequest){
		func(r *RestorePlanRequest) { r.Restore.RunAsSuperuser = true }, func(r *RestorePlanRequest) { r.Restore.NetworkDisabled = false },
		func(r *RestorePlanRequest) { r.Restore.JobsDisabled = false }, func(r *RestorePlanRequest) { r.Restore.ExecutableReviewed = false },
		func(r *RestorePlanRequest) { r.Restore.TargetKind = "existing-staging" }, func(r *RestorePlanRequest) { r.Restore.DatabaseName = "postgres" },
		func(r *RestorePlanRequest) { r.Restore.PostgresMajor = 17 }, func(r *RestorePlanRequest) { r.Restore.TargetIdentitySHA256 = r.Package.Intake.Source.IdentitySHA256 },
		func(r *RestorePlanRequest) { r.Restore.ReviewRef = "" },
	} {
		r, now := restoreFixture(t)
		change(&r)
		p := planRestore(ctx, r, now, func(context.Context, PackageRequest, time.Time) PackageReport {
			t.Fatal("unsafe declaration reached files")
			return PackageReport{}
		})
		if p.Status != "BLOCKED" || len(p.Steps) != 0 {
			t.Fatal("unsafe restore planned")
		}
	}
}

func TestRestorePlanExtensionsAndBinding(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	r, now := restoreFixture(t)
	p := planRestore(ctx, r, now, localPackage)
	if p.Status != "OFFLINE_RESTORE_PLAN_VALID_NOT_AUTHORIZED" || len(p.PlanSHA256) != 64 || len(p.PackageSHA256) != 64 {
		t.Fatal("valid offline plan rejected")
	}
	r.Restore.ReviewRef = "different-fixture-review"
	if planRestore(ctx, r, now, localPackage).PlanSHA256 == p.PlanSHA256 {
		t.Fatal("review declaration not bound")
	}
	for _, extensions := range [][]InventoryExtension{nil, {{"plpgsql", "wrong-version"}}, {{"plpgsql", "1.0"}, {"plpgsql", "1.0"}}} {
		r.Restore.AvailableExtensions = extensions
		if planRestore(ctx, r, now, localPackage).Status != "BLOCKED" {
			t.Fatal("missing/mismatched/duplicate extension accepted")
		}
	}
	r, now = restoreFixture(t)
	r.Package.Artifacts[0].SHA256 = strings.Repeat("f", 64)
	if planRestore(ctx, r, now, localPackage).Status != "BLOCKED" {
		t.Fatal("invalid package opened restore plan")
	}
}

func TestRestorePlanNeverExecutes(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	r, now := restoreFixture(t)
	raw, _ := json.Marshal(r)
	if _, err := DecodeRestorePlan(strings.NewReader(string(raw))); err != nil {
		t.Fatal("valid request rejected")
	}
	bad := strings.Replace(string(raw), `"run_as_superuser":false`, `"run_as_superuser":false,"run_as_superuser":true`, 1)
	if _, err := DecodeRestorePlan(strings.NewReader(bad)); err == nil {
		t.Fatal("duplicate restore control accepted")
	}
	p := planRestore(ctx, r, now, localPackage)
	if p.AuthorizesExecution || p.ActualRestoreVerified || p.DatabaseAccess != "NONE" || len(p.Steps) != 9 || len(p.ExternalGates) == 0 {
		t.Fatal("restore was claimed or gates lost")
	}
	encoded, _ := json.Marshal(p)
	for _, secret := range []string{r.Restore.DatabaseName, r.Package.Intake.ArtifactRoot, "pg_restore", "DATABASE_URL"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("plan contains command or private target input")
		}
	}
}
