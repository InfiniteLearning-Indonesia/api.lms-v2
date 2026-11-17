package backup

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type RestoreDeclarations struct {
	TargetIdentitySHA256 string               `json:"target_identity_sha256"`
	DatabaseName         string               `json:"database_name"`
	TargetKind           string               `json:"target_kind"`
	PostgresMajor        int                  `json:"postgres_major"`
	AvailableExtensions  []InventoryExtension `json:"available_extensions"`
	NetworkDisabled      bool                 `json:"network_disabled"`
	JobsDisabled         bool                 `json:"jobs_disabled"`
	ExecutableReviewed   bool                 `json:"executable_dump_reviewed"`
	RunAsSuperuser       bool                 `json:"run_as_superuser"`
	ReviewRef            string               `json:"review_ref"`
}

type RestorePlanRequest struct {
	Version int                 `json:"version"`
	Package PackageRequest      `json:"package"`
	Restore RestoreDeclarations `json:"restore"`
}

type RestorePlanReport struct {
	Status                string   `json:"status"`
	PlanSHA256            string   `json:"plan_sha256,omitempty"`
	PackageSHA256         string   `json:"package_sha256,omitempty"`
	Steps                 []string `json:"steps"`
	Blockers              []string `json:"blockers"`
	ExternalGates         []string `json:"external_gates"`
	AuthorizesExecution   bool     `json:"authorizes_execution"`
	ActualRestoreVerified bool     `json:"actual_restore_verified"`
	DatabaseAccess        string   `json:"database_access"`
}

// PlanRestore returns static preparation steps only. Database names, tool
// versions, isolation and reviews are declarations, never actual target proof.
// No executable command, DSN, SQL, directory creation or DB call is provided.
func PlanRestore(ctx context.Context, r RestorePlanRequest, now time.Time) RestorePlanReport {
	return planRestore(ctx, r, now, CheckPackage)
}

func planRestore(ctx context.Context, r RestorePlanRequest, now time.Time, verify func(context.Context, PackageRequest, time.Time) PackageReport) RestorePlanReport {
	p := RestorePlanReport{Status: "BLOCKED", DatabaseAccess: "NONE", Steps: []string{}, Blockers: []string{}, ExternalGates: []string{
		"ACTUAL_EMPTY_TARGET_IDENTITY_AND_PRIVILEGES_NOT_VERIFIED",
		"ACTUAL_POSTGRES_AND_EXTENSION_VERSIONS_NOT_VERIFIED",
		"OUTBOUND_JOBS_ISOLATION_AND_EXECUTABLE_REVIEW_NOT_INDEPENDENTLY_VERIFIED",
		"FRESH_TARGET_PRESTATE_BACKUP_NOT_VERIFIED", "ACTUAL_DATABASE_RESTORE_NOT_EXECUTED",
	}}
	fail := func(code string) RestorePlanReport { p.Blockers = append(p.Blockers, code); return p }
	d := r.Restore
	if !validPackageContext(ctx) || r.Version != 1 || d.TargetKind != "new-local-restore" ||
		d.TargetIdentitySHA256 != r.Package.Intake.Target.IdentitySHA256 || !fingerprint.MatchString(d.TargetIdentitySHA256) ||
		!schemaName.MatchString(d.DatabaseName) || !strings.HasPrefix(d.DatabaseName, "v3_restore_") || d.DatabaseName == "v3_restore_" ||
		d.PostgresMajor != 18 || d.RunAsSuperuser || !d.NetworkDisabled || !d.JobsDisabled || !d.ExecutableReviewed || !token.MatchString(d.ReviewRef) || len(d.AvailableExtensions) > 128 {
		return fail("RESTORE_DECLARATIONS_UNSAFE_OR_INCOMPLETE")
	}
	available := map[string]string{}
	for _, e := range d.AvailableExtensions {
		if !extensionName.MatchString(e.Name) || !extensionVersion.MatchString(e.Version) || available[e.Name] != "" {
			return fail("RESTORE_EXTENSION_DECLARATIONS_INVALID")
		}
		available[e.Name] = e.Version
	}
	pkg := verify(ctx, r.Package, now)
	if pkg.Status != "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED" || !fingerprint.MatchString(pkg.PlanSHA256) || pkg.AuthorizesExecution {
		return fail("RESTORE_PACKAGE_INTEGRITY_NOT_VALID")
	}
	if r.Package.Snapshot.ServerMajor > d.PostgresMajor || r.Package.Snapshot.DumpClientMajor > d.PostgresMajor {
		return fail("RESTORE_DECLARED_DATABASE_VERSION_MISMATCH")
	}
	for _, required := range pkg.RequiredExtensions {
		if available[required.Name] != required.Version {
			return fail("RESTORE_DECLARED_EXTENSION_MISSING_OR_DIFFERENT")
		}
	}
	if !validPackageContext(ctx) {
		return fail("RESTORE_CONTEXT_EXPIRED")
	}
	p.ExternalGates = append(p.ExternalGates, pkg.ExternalGates...)
	p.PackageSHA256 = pkg.PlanSHA256
	d.AvailableExtensions = append([]InventoryExtension(nil), d.AvailableExtensions...)
	sort.Slice(d.AvailableExtensions, func(i, j int) bool { return d.AvailableExtensions[i].Name < d.AvailableExtensions[j].Name })
	encoded, _ := json.Marshal(struct {
		PackageSHA256 string
		Restore       RestoreDeclarations
	}{pkg.PlanSHA256, d})
	p.PlanSHA256 = Hash(encoded)
	p.Steps = []string{
		"PROVE_NEW_EMPTY_LOCAL_TARGET_AND_INDEPENDENT_IDENTITY",
		"VERIFY_ACTUAL_TOOLS_EXTENSIONS_PRIVILEGES_AND_SPACE",
		"PROVE_OUTBOUND_AND_BACKGROUND_JOBS_DISABLED",
		"REVIEW_EXECUTABLE_DUMP_OBJECTS_WITH_SEPARATE_OWNER",
		"VERIFY_FRESH_TARGET_PRESTATE_BACKUP_AND_KEY_RECOVERY",
		"REVERIFY_INPUT_BYTES_AND_AUTHENTICATED_ARCHIVE_TO_EOF",
		"RESTORE_ONLY_TO_NEW_TARGET_WITH_REVIEWED_SCOPE",
		"RECONCILE_SCHEMA_KEYS_ROWS_HASHES_AND_FILE_CONTENTS",
		"RETAIN_RECEIPTS_FOR_REVIEW_NO_PUBLISH_OR_PILOT",
	}
	p.Status = "OFFLINE_RESTORE_PLAN_VALID_NOT_AUTHORIZED"
	return p
}
