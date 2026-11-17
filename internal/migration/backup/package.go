package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const MaxPackageArtifacts = 128
const MaxInventoryEntries = 4096

var extensionName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var extensionVersion = regexp.MustCompile(`^[a-zA-Z0-9_.+-]{1,40}$`)

type SnapshotDeclaration struct {
	ID                   string `json:"id"`
	SourceIdentitySHA256 string `json:"source_identity_sha256"`
	SchemaSHA256         string `json:"schema_sha256"`
	ExportedAt           string `json:"exported_at"`
	ConsistencyRef       string `json:"consistency_ref"`
	DumpFormat           string `json:"dump_format"`
	ServerMajor          int    `json:"server_major"`
	DumpClientMajor      int    `json:"dump_client_major"`
}

type PackageArtifact struct {
	Role       string `json:"role"`
	Name       string `json:"name"`
	SnapshotID string `json:"snapshot_id"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
	Encryption string `json:"encryption"`
}

type PackageRequest struct {
	Version   int                 `json:"version"`
	Intake    IntakeRequest       `json:"intake"`
	Snapshot  SnapshotDeclaration `json:"snapshot"`
	Artifacts []PackageArtifact   `json:"artifacts"`
}

type InventoryTable struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
}

type InventoryExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type SchemaInventory struct {
	Version      int                  `json:"version"`
	SnapshotID   string               `json:"snapshot_id"`
	SourceSHA256 string               `json:"source_identity_sha256"`
	SchemaSHA256 string               `json:"schema_sha256"`
	Schemas      []string             `json:"schemas"`
	Tables       []InventoryTable     `json:"tables"`
	Extensions   []InventoryExtension `json:"extensions"`
	CompleteRef  string               `json:"complete_ref"`
}

type InventoryObject struct {
	Ref    string `json:"ref"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type FileInventory struct {
	Version            int               `json:"version"`
	SnapshotID         string            `json:"snapshot_id"`
	SourceSHA256       string            `json:"source_identity_sha256"`
	Objects            []InventoryObject `json:"objects"`
	BundleSHA256       string            `json:"bundle_sha256"`
	NoObjectsReviewRef string            `json:"no_objects_review_ref"`
}

type PackageReport struct {
	Status                string               `json:"status"`
	PlanSHA256            string               `json:"plan_sha256,omitempty"`
	VerifiedArtifactCount int                  `json:"verified_artifact_count"`
	VerifiedArtifactBytes int64                `json:"verified_artifact_bytes"`
	DeclaredTables        int                  `json:"declared_tables"`
	DeclaredRows          int64                `json:"declared_rows"`
	DeclaredObjects       int                  `json:"declared_objects"`
	RequiredExtensions    []InventoryExtension `json:"required_extensions"`
	Blockers              []string             `json:"blockers"`
	ExternalGates         []string             `json:"external_gates"`
	AuthorizesExecution   bool                 `json:"authorizes_execution"`
	ArchiveAuthentication bool                 `json:"archive_authentication_verified"`
	ActualRestoreVerified bool                 `json:"actual_restore_verified"`
	RealBaselineVerified  bool                 `json:"real_baseline_verified"`
	DatabaseAccess        string               `json:"database_access"`
}

func packageBaseReport() PackageReport {
	return PackageReport{Status: "BLOCKED", DatabaseAccess: "NONE", Blockers: []string{},
		RequiredExtensions: []InventoryExtension{}, ExternalGates: []string{
			"SOURCE_IDENTITY_ROLE_TLS_AND_APPROVAL_NOT_INDEPENDENTLY_VERIFIED",
			"CATALOG_AND_OBJECT_COMPLETENESS_NOT_INDEPENDENTLY_VERIFIED",
			"CONSISTENT_SOURCE_SNAPSHOT_NOT_INDEPENDENTLY_VERIFIED",
			"ARCHIVE_AUTHENTICATION_PROTECTION_AND_KEY_CUSTODY_NOT_VERIFIED",
			"OBJECT_BUNDLE_CONTENTS_NOT_RESTORED_OR_RECONCILED",
			"ACTUAL_DATABASE_RESTORE_NOT_EXECUTED",
		}}
}

// CheckPackage reads only supplied local files. Hash/metadata/header checks
// are bounded integrity evidence, never source approval or pg_restore proof.
func CheckPackage(ctx context.Context, r PackageRequest, now time.Time) PackageReport {
	return checkPackage(ctx, r, now, CheckIntake)
}

func checkPackage(ctx context.Context, r PackageRequest, now time.Time, preflight func(IntakeRequest, time.Time) IntakeReport) PackageReport {
	p := packageBaseReport()
	fail := func(code string) PackageReport { p.Blockers = append(p.Blockers, code); return p }
	if !validPackageContext(ctx) || r.Version != 1 {
		return fail("PACKAGE_CONTEXT_OR_VERSION_INVALID")
	}
	intake := preflight(r.Intake, now)
	if intake.Status != "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED" || !fingerprint.MatchString(intake.PlanSHA256) || intake.AuthorizesExecution {
		return fail("PACKAGE_INTAKE_PREFLIGHT_NOT_VALID")
	}
	s := r.Snapshot
	exported, err := time.Parse(time.RFC3339Nano, s.ExportedAt)
	_, offset := exported.Zone()
	if !token.MatchString(s.ID) || s.SourceIdentitySHA256 != r.Intake.Source.IdentitySHA256 || s.SchemaSHA256 != r.Intake.Source.SchemaSHA256 ||
		!token.MatchString(s.ConsistencyRef) || s.DumpFormat != "postgres-custom" || err != nil || offset != 0 || exported.IsZero() ||
		exported.After(now) || now.Sub(exported) > 24*time.Hour || s.ServerMajor < 14 || s.ServerMajor > 18 || s.DumpClientMajor < s.ServerMajor || s.DumpClientMajor > 18 {
		return fail("PACKAGE_SNAPSHOT_BINDING_OR_VERSION_INVALID")
	}
	if len(r.Artifacts) < 4 || len(r.Artifacts) > MaxPackageArtifacts {
		return fail("PACKAGE_ARTIFACT_COUNT_INVALID")
	}
	roles := map[string]PackageArtifact{}
	names := map[string]bool{}
	var total int64
	for _, a := range r.Artifacts {
		switch a.Role {
		case "db_custom", "db_sql", "schema_inventory", "file_inventory", "object_bundle":
		default:
			return fail("PACKAGE_ARTIFACT_ROLE_INVALID")
		}
		if _, duplicate := roles[a.Role]; duplicate || names[a.Name] || !streamArtifactName.MatchString(a.Name) ||
			a.SnapshotID != s.ID || a.Bytes <= 0 || a.Bytes > MaxStreamArtifactBytes || !fingerprint.MatchString(a.SHA256) ||
			(a.Encryption != "none" && a.Encryption != "age-v1") || total > MaxStreamArtifactBytes-a.Bytes {
			return fail("PACKAGE_ARTIFACT_BINDING_OR_LIMIT_INVALID")
		}
		if (a.Role == "schema_inventory" || a.Role == "file_inventory") && (a.Encryption != "none" || a.Bytes > MaxInventoryJSON) {
			return fail("PACKAGE_METADATA_FORMAT_OR_LIMIT_INVALID")
		}
		total += a.Bytes
		roles[a.Role], names[a.Name] = a, true
	}
	for _, role := range []string{"db_custom", "db_sql", "schema_inventory", "file_inventory"} {
		if _, ok := roles[role]; !ok {
			return fail("PACKAGE_REQUIRED_ARTIFACT_MISSING")
		}
	}
	if roles["db_custom"].Encryption != roles["db_sql"].Encryption ||
		uint64(roles["db_custom"].Bytes) > r.Intake.Capacity.DumpBytes || uint64(roles["db_sql"].Bytes) > r.Intake.Capacity.SQLBytes {
		return fail("PACKAGE_DATABASE_PROTECTION_OR_CAPACITY_MISMATCH")
	}
	if bundle, exists := roles["object_bundle"]; exists && uint64(bundle.Bytes) > r.Intake.Capacity.ObjectBytes {
		return fail("PACKAGE_OBJECT_CAPACITY_MISMATCH")
	}
	if !packageDirectoryMatches(r.Intake.ArtifactRoot, names) {
		return fail("PACKAGE_DIRECTORY_SCOPE_MISMATCH")
	}
	for _, a := range r.Artifacts {
		if _, err := VerifyStreamFile(ctx, r.Intake.ArtifactRoot, a.Name, a.Bytes, a.Bytes, a.SHA256); err != nil {
			return fail("PACKAGE_ARTIFACT_INTEGRITY_FAILED")
		}
		p.VerifiedArtifactCount++
		p.VerifiedArtifactBytes += a.Bytes
	}
	custom := roles["db_custom"]
	prefix, err := packagePrefix(r.Intake.ArtifactRoot, custom)
	if err != nil || custom.Encryption == "none" && !bytes.HasPrefix(prefix, []byte("PGDMP")) ||
		custom.Encryption == "age-v1" && !bytes.HasPrefix(prefix, []byte("age-encryption.org/v1\n")) {
		return fail("PACKAGE_CUSTOM_DUMP_HEADER_INVALID")
	}
	for _, a := range r.Artifacts {
		if a.Encryption == "age-v1" {
			prefix, err := packagePrefix(r.Intake.ArtifactRoot, a)
			if err != nil || !bytes.HasPrefix(prefix, []byte("age-encryption.org/v1\n")) {
				return fail("PACKAGE_DECLARED_ENCRYPTION_HEADER_INVALID")
			}
		}
	}
	var catalog SchemaInventory
	if readPackageMetadata(r.Intake.ArtifactRoot, roles["schema_inventory"], &catalog) != nil || !validCatalog(catalog, r) {
		return fail("PACKAGE_SCHEMA_INVENTORY_INVALID")
	}
	var files FileInventory
	if readPackageMetadata(r.Intake.ArtifactRoot, roles["file_inventory"], &files) != nil || !validFiles(files, r, roles) {
		return fail("PACKAGE_FILE_INVENTORY_INVALID")
	}
	if !validPackageContext(ctx) {
		return fail("PACKAGE_CONTEXT_EXPIRED")
	}
	p.DeclaredTables, p.DeclaredObjects = len(catalog.Tables), len(files.Objects)
	for _, table := range catalog.Tables {
		p.DeclaredRows += table.Rows
	}
	p.RequiredExtensions = append(p.RequiredExtensions, catalog.Extensions...)
	sort.Slice(p.RequiredExtensions, func(i, j int) bool { return p.RequiredExtensions[i].Name < p.RequiredExtensions[j].Name })
	r.Artifacts = append([]PackageArtifact(nil), r.Artifacts...)
	sort.Slice(r.Artifacts, func(i, j int) bool { return r.Artifacts[i].Role < r.Artifacts[j].Role })
	encoded, _ := json.Marshal(struct {
		IntakeSHA string
		Snapshot  SnapshotDeclaration
		Artifacts []PackageArtifact
	}{intake.PlanSHA256, s, r.Artifacts})
	p.PlanSHA256 = Hash(encoded)
	p.Status = "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED"
	return p
}

func validPackageContext(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	deadline, ok := ctx.Deadline()
	return ok && time.Until(deadline) > 0 && time.Until(deadline) <= MaxStreamCheckTime
}

func packageDirectoryMatches(directory string, names map[string]bool) bool {
	if !intakeRoot(directory) {
		return false
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return false
	}
	defer root.Close()
	folder, err := root.Open(".")
	if err != nil {
		return false
	}
	defer folder.Close()
	entries, err := folder.ReadDir(MaxPackageArtifacts + 1)
	if err != nil && err != io.EOF || len(entries) != len(names) {
		return false
	}
	for _, entry := range entries {
		if !names[entry.Name()] || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func validCatalog(c SchemaInventory, r PackageRequest) bool {
	if c.Version != 1 || c.SnapshotID != r.Snapshot.ID || c.SourceSHA256 != r.Snapshot.SourceIdentitySHA256 ||
		c.SchemaSHA256 != r.Snapshot.SchemaSHA256 || !token.MatchString(c.CompleteRef) || len(c.Tables) == 0 ||
		len(c.Tables) > MaxInventoryEntries || len(c.Extensions) > 128 || len(c.Schemas) != len(r.Intake.Schemas) {
		return false
	}
	schemas := map[string]bool{}
	for _, name := range r.Intake.Schemas {
		schemas[name] = true
	}
	for _, name := range c.Schemas {
		if !schemas[name] {
			return false
		}
		delete(schemas, name)
	}
	if len(schemas) != 0 {
		return false
	}
	for _, name := range c.Schemas {
		schemas[name] = true
	}
	seen := map[string]bool{}
	var rows int64
	for _, t := range c.Tables {
		key := t.Schema + "." + t.Name
		if !schemas[t.Schema] || !schemaName.MatchString(t.Name) || seen[key] || t.Rows < 0 || t.Rows > math.MaxInt64-rows {
			return false
		}
		seen[key] = true
		rows += t.Rows
	}
	seen = map[string]bool{}
	for _, e := range c.Extensions {
		if !extensionName.MatchString(e.Name) || !extensionVersion.MatchString(e.Version) || seen[e.Name] {
			return false
		}
		seen[e.Name] = true
	}
	return true
}

func validFiles(f FileInventory, r PackageRequest, roles map[string]PackageArtifact) bool {
	if f.Version != 1 || f.SnapshotID != r.Snapshot.ID || f.SourceSHA256 != r.Snapshot.SourceIdentitySHA256 || len(f.Objects) > MaxInventoryEntries {
		return false
	}
	bundle, exists := roles["object_bundle"]
	if len(f.Objects) == 0 {
		return !exists && f.BundleSHA256 == "" && token.MatchString(f.NoObjectsReviewRef)
	}
	if !exists || f.BundleSHA256 != bundle.SHA256 || f.NoObjectsReviewRef != "" {
		return false
	}
	seen := map[string]bool{}
	var total int64
	for _, object := range f.Objects {
		if !token.MatchString(object.Ref) || seen[object.Ref] || object.Bytes < 0 || object.Bytes > MaxStreamArtifactBytes-total || !fingerprint.MatchString(object.SHA256) {
			return false
		}
		seen[object.Ref] = true
		total += object.Bytes
	}
	return true
}

// Metadata is rehashed on its bounded reread so a file swap after the streamed
// verification cannot supply a different inventory to the parser.
func readPackageMetadata(directory string, a PackageArtifact, destination any) error {
	if a.Bytes > MaxInventoryJSON {
		return errPackage
	}
	root, file, err := openPackageFile(directory, a)
	if err != nil {
		return errPackage
	}
	defer root.Close()
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxInventoryJSON+1))
	if err != nil || int64(len(raw)) != a.Bytes || Hash(raw) != a.SHA256 {
		return errPackage
	}
	return decodePackageJSON(bytes.NewReader(raw), MaxInventoryJSON, destination)
}

func packagePrefix(directory string, a PackageArtifact) ([]byte, error) {
	root, file, err := openPackageFile(directory, a)
	if err != nil {
		return nil, errPackage
	}
	defer root.Close()
	defer file.Close()
	prefix, err := io.ReadAll(io.LimitReader(file, 32))
	if err != nil {
		return nil, errPackage
	}
	return prefix, nil
}

func openPackageFile(directory string, a PackageArtifact) (*os.Root, *os.File, error) {
	if !intakeRoot(directory) || !streamArtifactName.MatchString(a.Name) {
		return nil, nil, errPackage
	}
	beforeRoot, err := os.Lstat(directory)
	if err != nil {
		return nil, nil, errPackage
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, errPackage
	}
	anchored, err := root.Stat(".")
	if err != nil || !os.SameFile(beforeRoot, anchored) {
		root.Close()
		return nil, nil, errPackage
	}
	before, err := root.Lstat(a.Name)
	if err != nil || !before.Mode().IsRegular() || before.Size() != a.Bytes || intakeReparse(filepath.Join(directory, a.Name)) {
		root.Close()
		return nil, nil, errPackage
	}
	file, err := root.Open(a.Name)
	if err != nil {
		root.Close()
		return nil, nil, errPackage
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		file.Close()
		root.Close()
		return nil, nil, errPackage
	}
	return root, file, nil
}
