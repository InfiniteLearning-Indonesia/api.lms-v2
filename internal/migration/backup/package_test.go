package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func packageFixture(t *testing.T) (PackageRequest, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	root := t.TempDir()
	r := PackageRequest{Version: 1, Intake: intakeFixture(root, now),
		Snapshot: SnapshotDeclaration{ID: "fixture-snapshot", SourceIdentitySHA256: strings.Repeat("a", 64),
			SchemaSHA256: strings.Repeat("b", 64), ExportedAt: now.Format(time.RFC3339), ConsistencyRef: "fixture-consistency-claim", DumpFormat: "postgres-custom", ServerMajor: 18, DumpClientMajor: 18}}
	r.Intake.Schemas = []string{"public"}
	catalog := SchemaInventory{Version: 1, SnapshotID: r.Snapshot.ID, SourceSHA256: r.Snapshot.SourceIdentitySHA256, SchemaSHA256: r.Snapshot.SchemaSHA256,
		Schemas: []string{"public"}, Tables: []InventoryTable{{"public", "users", 2}, {"public", "discovered_tracking", 1}},
		Extensions: []InventoryExtension{{"plpgsql", "1.0"}}, CompleteRef: "fixture-not-real-completeness"}
	files := FileInventory{Version: 1, SnapshotID: r.Snapshot.ID, SourceSHA256: r.Snapshot.SourceIdentitySHA256, Objects: []InventoryObject{}, NoObjectsReviewRef: "fixture-no-objects-review"}
	for _, a := range []struct {
		role, name string
		value      any
	}{
		{"db_custom", "database.dump", []byte("PGDMP synthetic header only; not a valid PostgreSQL archive")},
		{"db_sql", "database.sql", []byte("-- synthetic fixture; never execute SQL\n")},
		{"schema_inventory", "schema.json", catalog}, {"file_inventory", "files.json", files},
	} {
		data, ok := a.value.([]byte)
		if !ok {
			data, _ = json.Marshal(a.value)
		}
		if err := os.WriteFile(filepath.Join(root, a.name), data, 0600); err != nil {
			t.Fatal(err)
		}
		r.Artifacts = append(r.Artifacts, PackageArtifact{a.role, a.name, r.Snapshot.ID, int64(len(data)), Hash(data), "none"})
	}
	return r, now
}

func localPackage(ctx context.Context, r PackageRequest, now time.Time) PackageReport {
	return checkPackage(ctx, r, now, func(i IntakeRequest, n time.Time) IntakeReport {
		return checkIntake(i, n, func(string) (uint64, error) { return math.MaxUint64, nil })
	})
}

func packageContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Minute)
}

func rewritePackageMetadata(t *testing.T, r *PackageRequest, role string, change func(map[string]any)) {
	t.Helper()
	for i, a := range r.Artifacts {
		if a.Role != role {
			continue
		}
		path := filepath.Join(r.Intake.ArtifactRoot, a.Name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			t.Fatal("bad fixture JSON")
		}
		change(value)
		raw, _ = json.Marshal(value)
		if os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("fixture write failed")
		}
		r.Artifacts[i].Bytes = int64(len(raw))
		r.Artifacts[i].SHA256 = Hash(raw)
		return
	}
	t.Fatal("fixture role not found")
}

func TestPackageStrictJSON(t *testing.T) {
	r, _ := packageFixture(t)
	raw, _ := json.Marshal(r)
	if _, err := DecodePackage(bytes.NewReader(raw)); err != nil {
		t.Fatal("valid manifest rejected")
	}
	for _, bad := range []string{"null", "[]", "{}", `{"version":1,"version":1}`, string(raw) + "{}",
		strings.Replace(string(raw), `"role":`, `"Role":`, 1),
		strings.Replace(string(raw), `"snapshot_id":`, `"snapshot_id":"duplicate","snapshot_id":`, 1),
		strings.Replace(string(raw), `"role":`, `"PRIVATE_SECRET":"fixture","role":`, 1),
		string(raw) + strings.Repeat(" ", MaxPackageJSON),
		strings.Replace(string(raw), "fixture-snapshot", string([]byte{'x', 0xff}), 1),
	} {
		if _, err := DecodePackage(strings.NewReader(bad)); err == nil || err.Error() != "BASELINE_PACKAGE_REJECTED" {
			t.Fatal("ambiguous manifest accepted or input leaked")
		}
	}
}

func TestPackageRequiredArtifactsAndBinding(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	r, now := packageFixture(t)
	p := localPackage(ctx, r, now)
	if p.Status != "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED" || p.VerifiedArtifactCount != 4 || p.DeclaredTables != 2 || p.DeclaredRows != 3 || p.DeclaredObjects != 0 {
		t.Fatal("valid fixture not accounted")
	}
	reordered := r
	reordered.Artifacts = append([]PackageArtifact(nil), r.Artifacts...)
	reordered.Artifacts[0], reordered.Artifacts[1] = reordered.Artifacts[1], reordered.Artifacts[0]
	if localPackage(ctx, reordered, now).PlanSHA256 != p.PlanSHA256 {
		t.Fatal("artifact order changes semantic binding")
	}
	for _, mutate := range []func(*PackageRequest){
		func(r *PackageRequest) { r.Version = 2 }, func(r *PackageRequest) { r.Artifacts = r.Artifacts[:3] },
		func(r *PackageRequest) { r.Artifacts[0].Role = "unreviewed" }, func(r *PackageRequest) { r.Artifacts[1].Role = r.Artifacts[0].Role },
		func(r *PackageRequest) { r.Artifacts[1].Name = r.Artifacts[0].Name }, func(r *PackageRequest) { r.Artifacts[0].SnapshotID = "different" },
		func(r *PackageRequest) { r.Snapshot.SourceIdentitySHA256 = strings.Repeat("f", 64) }, func(r *PackageRequest) { r.Snapshot.SchemaSHA256 = strings.Repeat("f", 64) },
		func(r *PackageRequest) { r.Snapshot.ExportedAt = now.Add(-25 * time.Hour).Format(time.RFC3339) },
		func(r *PackageRequest) { r.Snapshot.ServerMajor = 19 }, func(r *PackageRequest) { r.Snapshot.DumpClientMajor = 17 },
		func(r *PackageRequest) { r.Intake.Capacity.DumpBytes = 1 }, func(r *PackageRequest) { r.Artifacts[0].Encryption = "unknown" },
	} {
		bad, _ := packageFixture(t)
		mutate(&bad)
		if localPackage(ctx, bad, now).Status != "BLOCKED" {
			t.Fatal("incomplete/mixed snapshot accepted")
		}
	}
}

func TestPackageIntegrityAndContainment(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	for _, change := range []func(*PackageRequest){
		func(r *PackageRequest) { r.Artifacts[0].Name = "../database.dump" },
		func(r *PackageRequest) { r.Artifacts[0].SHA256 = strings.Repeat("f", 64) },
		func(r *PackageRequest) {
			os.WriteFile(filepath.Join(r.Intake.ArtifactRoot, "database.dump"), []byte("truncated"), 0600)
		},
		func(r *PackageRequest) {
			os.WriteFile(filepath.Join(r.Intake.ArtifactRoot, "unlisted.key"), []byte("PRIVATE_FIXTURE_ONLY"), 0600)
		},
		func(r *PackageRequest) { r.Artifacts[0].Bytes = MaxStreamArtifactBytes + 1 },
	} {
		r, now := packageFixture(t)
		change(&r)
		if localPackage(ctx, r, now).Status != "BLOCKED" {
			t.Fatal("corruption/escape/unlisted entry accepted")
		}
	}
	r, now := packageFixture(t)
	data := []byte("not a PostgreSQL archive")
	os.WriteFile(filepath.Join(r.Intake.ArtifactRoot, r.Artifacts[0].Name), data, 0600)
	r.Artifacts[0].Bytes = int64(len(data))
	r.Artifacts[0].SHA256 = Hash(data)
	if p := localPackage(ctx, r, now); p.Status != "BLOCKED" {
		t.Fatal("valid hash treated as valid archive header")
	}
}

func TestPackageInventoryAccounting(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	for _, item := range []struct {
		role   string
		change func(map[string]any)
	}{
		{"schema_inventory", func(m map[string]any) { m["snapshot_id"] = "different" }},
		{"schema_inventory", func(m map[string]any) { m["schemas"] = []string{"foreign"} }},
		{"schema_inventory", func(m map[string]any) {
			m["tables"] = []any{map[string]any{"schema": "foreign", "name": "users", "rows": 1}}
		}},
		{"schema_inventory", func(m map[string]any) { a := m["tables"].([]any); m["tables"] = append(a, a[0]) }},
		{"schema_inventory", func(m map[string]any) { a := m["tables"].([]any); a[0].(map[string]any)["rows"] = -1 }},
		{"schema_inventory", func(m map[string]any) { m["unknown_secret"] = "PRIVATE_FIXTURE" }},
		{"file_inventory", func(m map[string]any) { m["no_objects_review_ref"] = "" }},
		{"file_inventory", func(m map[string]any) {
			m["objects"] = []any{map[string]any{"ref": "file-1", "bytes": 1, "sha256": strings.Repeat("e", 64)}}
		}},
	} {
		r, now := packageFixture(t)
		rewritePackageMetadata(t, &r, item.role, item.change)
		if localPackage(ctx, r, now).Status != "BLOCKED" {
			t.Fatal("invalid inventory accepted despite updated file hash")
		}
	}
	r, now := packageFixture(t)
	data := []byte("synthetic bundle; object contents not restored")
	a := PackageArtifact{"object_bundle", "objects.bundle", r.Snapshot.ID, int64(len(data)), Hash(data), "none"}
	os.WriteFile(filepath.Join(r.Intake.ArtifactRoot, a.Name), data, 0600)
	r.Artifacts = append(r.Artifacts, a)
	r.Intake.Capacity.ObjectBytes = uint64(a.Bytes)
	rewritePackageMetadata(t, &r, "file_inventory", func(m map[string]any) {
		m["no_objects_review_ref"] = ""
		m["bundle_sha256"] = a.SHA256
		m["objects"] = []any{map[string]any{"ref": "file-1", "bytes": 1, "sha256": strings.Repeat("e", 64)}}
	})
	p := localPackage(ctx, r, now)
	if p.Status != "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED" || p.DeclaredObjects != 1 || p.VerifiedArtifactCount != 5 || p.ActualRestoreVerified || p.ArchiveAuthentication {
		t.Fatal("bundle accounting or proof boundary wrong")
	}
	rewritePackageMetadata(t, &r, "file_inventory", func(m map[string]any) { a := m["objects"].([]any); m["objects"] = append(a, a[0]) })
	if localPackage(ctx, r, now).Status != "BLOCKED" {
		t.Fatal("duplicate object accepted")
	}
}

func TestPackageLimitsAndCancellation(t *testing.T) {
	r, now := packageFixture(t)
	ctx, cancel := packageContext()
	cancel()
	if localPackage(ctx, r, now).Status != "BLOCKED" || localPackage(context.Background(), r, now).Status != "BLOCKED" {
		t.Fatal("missing/canceled deadline accepted")
	}
	ctx, cancel = packageContext()
	defer cancel()
	r.Artifacts[2].Bytes = MaxInventoryJSON + 1
	if localPackage(ctx, r, now).Status != "BLOCKED" {
		t.Fatal("oversized inventory accepted")
	}
	r, now = packageFixture(t)
	p := localPackage(ctx, r, now)
	if p.AuthorizesExecution || p.RealBaselineVerified || p.ActualRestoreVerified || p.ArchiveAuthentication || len(p.ExternalGates) == 0 || p.DatabaseAccess != "NONE" {
		t.Fatal("byte checks opened operational authority")
	}
	if _, err := os.Stat(filepath.Join(r.Intake.ArtifactRoot, r.Intake.RunID)); !os.IsNotExist(err) {
		t.Fatal("candidate directory created")
	}
}
