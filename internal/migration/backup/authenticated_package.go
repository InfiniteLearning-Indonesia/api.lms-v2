package backup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"
)

type AuthenticatedArtifact struct {
	Role    string           `json:"role"`
	Name    string           `json:"name"`
	Receipt AgeStreamReceipt `json:"receipt"`
}

type AuthenticatedPackageRequest struct {
	Version      int                     `json:"version"`
	Package      PackageRequest          `json:"package"`
	Tool         AgeTool                 `json:"tool"`
	IdentityPath string                  `json:"identity_path"`
	Artifacts    []AuthenticatedArtifact `json:"artifacts"`
}

type AuthenticatedPackageReport struct {
	Status                     string   `json:"status"`
	PlanSHA256                 string   `json:"plan_sha256,omitempty"`
	IntegrityPlanSHA256        string   `json:"integrity_plan_sha256,omitempty"`
	AuthenticatedArtifactCount int      `json:"authenticated_artifact_count"`
	ArchiveAuthentication      bool     `json:"archive_authentication_verified"`
	LocalProtectionVerified    bool     `json:"local_protection_verified"`
	IndependentCustodyVerified bool     `json:"independent_custody_verified"`
	ActualRestoreVerified      bool     `json:"actual_restore_verified"`
	RealBaselineVerified       bool     `json:"real_baseline_verified"`
	AuthorizesExecution        bool     `json:"authorizes_execution"`
	DatabaseAccess             string   `json:"database_access"`
	Blockers                   []string `json:"blockers"`
	ExternalGates              []string `json:"external_gates"`
}

func DecodeAuthenticatedPackage(reader io.Reader) (AuthenticatedPackageRequest, error) {
	var r AuthenticatedPackageRequest
	if decodePackageJSON(reader, MaxPackageJSON, &r) != nil || r.Version != 1 {
		return AuthenticatedPackageRequest{}, errPackage
	}
	return r, nil
}

// PackageAgeBinding binds the encrypted inner manifest to source/snapshot,
// target, schemas, role, filename and plaintext digest, without a circular
// dependency on the ciphertext/package hash. This is not signed provenance.
func PackageAgeBinding(p PackageRequest, a PackageArtifact, plain StreamDigest) (Binding, error) {
	created, err := time.Parse(time.RFC3339Nano, p.Snapshot.ExportedAt)
	if err != nil || !token.MatchString(p.Intake.RunID) || !token.MatchString(p.Snapshot.ID) || a.SnapshotID != p.Snapshot.ID ||
		!fingerprint.MatchString(p.Intake.Source.IdentitySHA256) || p.Snapshot.SourceIdentitySHA256 != p.Intake.Source.IdentitySHA256 ||
		!fingerprint.MatchString(p.Intake.Target.IdentitySHA256) || !streamArtifactName.MatchString(a.Name) || a.Encryption != "age-v1" ||
		(a.Role != "db_custom" && a.Role != "db_sql" && a.Role != "object_bundle") ||
		plain.Bytes <= 0 || plain.Bytes > MaxStreamArtifactBytes-(1<<20) || !fingerprint.MatchString(plain.SHA256) {
		return Binding{}, errPackage
	}
	schemas := append([]string(nil), p.Intake.Schemas...)
	sort.Strings(schemas)
	encoded, _ := json.Marshal(struct {
		Version    int
		Snapshot   SnapshotDeclaration
		Source     IntakeSource
		Target     IntakeTarget
		Schemas    []string
		Role, Name string
		Plain      StreamDigest
	}{1, p.Snapshot, p.Intake.Source, p.Intake.Target, schemas, a.Role, a.Name, plain})
	return Binding{p.Intake.RunID, p.Intake.Target.IdentitySHA256, Hash(encoded), created}, nil
}

func authenticatedCoverage(r AuthenticatedPackageRequest) (map[string]AuthenticatedArtifact, error) {
	if r.Version != 1 || len(r.Artifacts) < 2 || len(r.Artifacts) > 3 {
		return nil, errPackage
	}
	expected := map[string]PackageArtifact{}
	for _, a := range r.Package.Artifacts {
		if a.Role == "db_custom" || a.Role == "db_sql" || a.Role == "object_bundle" {
			if a.Encryption != "age-v1" {
				return nil, errPackage
			}
			if _, ok := expected[a.Role]; ok {
				return nil, errPackage
			}
			expected[a.Role] = a
		}
	}
	if len(expected) != len(r.Artifacts) || expected["db_custom"].Role == "" || expected["db_sql"].Role == "" {
		return nil, errPackage
	}
	result := map[string]AuthenticatedArtifact{}
	for _, supplied := range r.Artifacts {
		a, ok := expected[supplied.Role]
		v := supplied.Receipt
		_, duplicate := result[supplied.Role]
		binding, err := PackageAgeBinding(r.Package, a, v.Plain)
		if !ok || duplicate || err != nil || supplied.Name != a.Name || v.Cipher != (StreamDigest{a.Bytes, a.SHA256}) ||
			v.AuthorizesExecution || v.Format != AgeBackupFormat || !sameAgeBinding(v.Binding, binding) ||
			(v.Status != "SEALED_STREAM_NOT_RESTORE_VERIFIED" && v.Status != "PASS_AUTHENTICATED_STREAM_ROUNDTRIP_ONLY") ||
			!fingerprint.MatchString(v.ToolSHA256) {
			return nil, errPackage
		}
		result[supplied.Role] = supplied
	}
	return result, nil
}

// AuthenticatePackage never exposes plaintext or executes SQL/restore. It
// requires actual age authentication, rather than trusting success booleans.
func AuthenticatePackage(ctx context.Context, r AuthenticatedPackageRequest, now time.Time) AuthenticatedPackageReport {
	p := AuthenticatedPackageReport{Status: "BLOCKED", DatabaseAccess: "NONE", Blockers: []string{}, ExternalGates: []string{
		"SOURCE_IDENTITY_ROLE_TLS_AND_APPROVAL_NOT_INDEPENDENTLY_VERIFIED",
		"CONSISTENT_SOURCE_SNAPSHOT_AND_CATALOG_COMPLETENESS_NOT_INDEPENDENTLY_VERIFIED",
		"SOURCE_PROVENANCE_NOT_CRYPTOGRAPHICALLY_SIGNED",
		"INDEPENDENT_KEY_CUSTODY_AND_RECOVERY_NOT_VERIFIED",
		"POSTGRES_ARCHIVE_AND_SQL_EXECUTABLE_SCOPE_NOT_REVIEWED",
		"OBJECT_CONTENTS_NOT_RESTORED_OR_RECONCILED", "ACTUAL_DATABASE_RESTORE_NOT_EXECUTED"}}
	fail := func(code string) AuthenticatedPackageReport { p.Blockers = append(p.Blockers, code); return p }
	if !validPackageContext(ctx) {
		return fail("AUTH_CONTEXT_INVALID")
	}
	coverage, err := authenticatedCoverage(r)
	if err != nil {
		return fail("AUTH_ARTIFACT_COVERAGE_OR_BINDING_INVALID")
	}
	rootPath := r.Package.Intake.ArtifactRoot
	if protectedIdentity(rootPath, r.IdentityPath) != nil || privateDirectory(rootPath) != nil || ValidateAgeTool(ctx, r.Tool, "age") != nil {
		return fail("AUTH_KEY_TOOL_OR_PROTECTION_INVALID")
	}
	integrity := CheckPackage(ctx, r.Package, now)
	if integrity.Status != "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED" || integrity.AuthorizesExecution {
		return fail("AUTH_PACKAGE_INTEGRITY_INVALID")
	}
	original, err := os.Lstat(rootPath)
	if err != nil {
		return fail("AUTH_DIRECTORY_CHANGED")
	}
	anchor, err := os.OpenRoot(rootPath)
	if err != nil {
		return fail("AUTH_DIRECTORY_CHANGED")
	}
	defer anchor.Close()
	if !protectedRootStable(anchor, rootPath, original) {
		return fail("AUTH_DIRECTORY_CHANGED")
	}
	for _, a := range r.Package.Artifacts {
		v, encrypted := coverage[a.Role]
		if !encrypted {
			continue
		}
		root, file, err := openPackageFile(rootPath, a)
		if err != nil {
			return fail("AUTH_ARTIFACT_UNAVAILABLE")
		}
		protected := privateHandle(file) == nil && singleLink(file)
		var verifyErr error
		if protected {
			_, verifyErr = VerifyAgeStream(ctx, r.Tool, r.IdentityPath, v.Receipt, file, now)
		}
		file.Close()
		root.Close()
		if !protected || verifyErr != nil {
			return fail("AUTH_ARTIFACT_AUTHENTICATION_FAILED")
		}
		p.AuthenticatedArtifactCount++
	}
	// Recheck metadata after crypto consumes ciphertext. Original matching
	// metadata hashes bind the integrity plan; no third whole-dump read needed.
	names := map[string]bool{}
	for _, a := range r.Package.Artifacts {
		names[a.Name] = true
		if a.Role == "schema_inventory" || a.Role == "file_inventory" {
			root, file, err := openPackageFile(rootPath, a)
			if err != nil {
				return fail("AUTH_METADATA_UNAVAILABLE")
			}
			protected := privateHandle(file) == nil && singleLink(file)
			file.Close()
			root.Close()
			if !protected {
				return fail("AUTH_METADATA_PROTECTION_INVALID")
			}
			if _, err := VerifyStreamFile(ctx, rootPath, a.Name, a.Bytes, MaxInventoryJSON, a.SHA256); err != nil {
				return fail("AUTH_METADATA_CHANGED")
			}
		}
	}
	if !validPackageContext(ctx) || !packageDirectoryMatches(rootPath, names) || !protectedRootStable(anchor, rootPath, original) || protectedIdentity(rootPath, r.IdentityPath) != nil {
		return fail("AUTH_INPUT_CHANGED_OR_EXPIRED")
	}
	entries := append([]AuthenticatedArtifact(nil), r.Artifacts...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Role < entries[j].Role })
	encoded, _ := json.Marshal(struct {
		Integrity, ConsumerToolSHA256 string
		Artifacts                     []AuthenticatedArtifact
	}{integrity.PlanSHA256, r.Tool.SHA256, entries})
	p.PlanSHA256, p.IntegrityPlanSHA256 = Hash(encoded), integrity.PlanSHA256
	p.Status, p.ArchiveAuthentication, p.LocalProtectionVerified = "OFFLINE_PACKAGE_AUTHENTICATED_NOT_AUTHORIZED", true, true
	return p
}
