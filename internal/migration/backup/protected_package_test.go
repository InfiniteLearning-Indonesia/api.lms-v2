package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateACLPolicy(t *testing.T) {
	user := "S-1-5-21-111-222-333-1001"
	good := []privateGrant{{user, 0, 3, 0x1f01ff}, {"S-1-5-18", 0, 3, 0x1f01ff}}
	if !privateACL(user, user, good) {
		t.Fatal("private owner/SYSTEM policy rejected")
	}
	for _, g := range [][]privateGrant{nil, {{user, 0, 3, 1}}, {{"S-1-1-0", 0, 0, 1}},
		{{"S-1-5-32-545", 0, 0, 0x1f01ff}}, {{user, 1, 0, 0x1f01ff}}, {{user, 0, 8, 0x1f01ff}},
		append(append([]privateGrant(nil), good...), privateGrant{"S-1-5-32-544", 0, 0, 0x1f01ff})} {
		if privateACL(user, user, g) {
			t.Fatal("broad, deny, inheritance-only or incomplete ACL accepted")
		}
	}
	if privateACL("other", user, good) || privateACL("", "", good) {
		t.Fatal("unowned ACL accepted")
	}
}

func TestProtectedAgeRefusalBeforeWrite(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "identity.txt")
	if identityOutsideArtifacts(root, filepath.Join(root, "identity.txt")) || identityOutsideArtifacts(root, "relative.key") || !identityOutsideArtifacts(root, outside) {
		t.Fatal("key/output separation")
	}
	if volume := filepath.VolumeName(root); volume != "" {
		other := "Z:"
		if strings.EqualFold(volume, other) {
			other = "Y:"
		}
		if !identityOutsideArtifacts(root, other+string(os.PathSeparator)+"private"+string(os.PathSeparator)+"identity.txt") {
			t.Fatal("distinct local volume rejected as inside artifact directory")
		}
	}
	r := ProtectedAgeRequest{Directory: root, Name: "../escape.age", IdentityPath: outside}
	never := streamReaderFunc(func([]byte) (int, error) { t.Fatal("refused request read source"); return 0, io.EOF })
	if p, err := SealProtectedAgeFile(ctx, AgeTool{}, r, never, time.Now().UTC()); err == nil || p.Status != "" {
		t.Fatal("unsafe output promoted")
	}
}

func authDeclarationFixture(t *testing.T) AuthenticatedPackageRequest {
	t.Helper()
	p, _ := packageFixture(t)
	r := AuthenticatedPackageRequest{Version: 1, Package: p, IdentityPath: "private-fixture", Artifacts: []AuthenticatedArtifact{}}
	for i := range r.Package.Artifacts {
		a := &r.Package.Artifacts[i]
		if a.Role != "db_custom" && a.Role != "db_sql" {
			continue
		}
		a.Encryption = "age-v1"
		plain := StreamDigest{4, Hash([]byte("data"))}
		binding, err := PackageAgeBinding(r.Package, *a, plain)
		if err != nil {
			t.Fatal(err)
		}
		receipt := AgeStreamReceipt{Status: "SEALED_STREAM_NOT_RESTORE_VERIFIED", Format: AgeBackupFormat, Binding: binding, Plain: plain,
			Cipher: StreamDigest{a.Bytes, a.SHA256}, TarBytes: 3072, ToolSHA256: strings.Repeat("d", 64)}
		r.Artifacts = append(r.Artifacts, AuthenticatedArtifact{a.Role, a.Name, receipt})
	}
	return r
}

func TestAuthenticatedPackageStrictInput(t *testing.T) {
	r := authDeclarationFixture(t)
	raw, _ := json.Marshal(r)
	if _, err := DecodeAuthenticatedPackage(bytes.NewReader(raw)); err != nil {
		t.Fatal("valid structured input rejected")
	}
	for _, bad := range []string{"null", "{}", string(raw) + "{}", `{"version":1,"version":1}`,
		strings.Replace(string(raw), `"identity_path":`, `"Identity_path":`, 1),
		strings.Replace(string(raw), `"role":`, `"unlisted_secret":"fixture","role":`, 1),
		strings.Replace(string(raw), `"cipher":`, `"cipher":null,"cipher":`, 1)} {
		if _, err := DecodeAuthenticatedPackage(strings.NewReader(bad)); err == nil || err.Error() != "BASELINE_PACKAGE_REJECTED" {
			t.Fatal("ambiguous JSON accepted/input leaked")
		}
	}
}

func TestAuthenticatedPackageCoverageAndBinding(t *testing.T) {
	r := authDeclarationFixture(t)
	// Structural receipt validation only: no mock crypto or archive success.
	if _, err := authenticatedCoverage(r); err != nil {
		t.Fatal("complete declaration rejected")
	}
	for _, change := range []func(*AuthenticatedPackageRequest){
		func(r *AuthenticatedPackageRequest) { r.Artifacts = r.Artifacts[:1] },
		func(r *AuthenticatedPackageRequest) { r.Artifacts = append(r.Artifacts, r.Artifacts[0]) },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[1] = r.Artifacts[0] },
		func(r *AuthenticatedPackageRequest) { r.Package.Artifacts[0].Encryption = "none" },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[0].Name = "another.age" },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[0].Role = "schema_inventory" },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[0].Receipt.Binding.Scope = strings.Repeat("f", 64) },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[0].Receipt.Plain.Bytes++ },
		func(r *AuthenticatedPackageRequest) { r.Package.Snapshot.ConsistencyRef = "changed-snapshot" },
		func(r *AuthenticatedPackageRequest) { r.Artifacts[0].Receipt.AuthorizesExecution = true },
	} {
		bad := authDeclarationFixture(t)
		change(&bad)
		if _, err := authenticatedCoverage(bad); err == nil {
			t.Fatal("incomplete/relabelled/mismatched receipt set accepted")
		}
	}
	ctx, cancel := packageContext()
	defer cancel()
	p := AuthenticatePackage(ctx, r, time.Now().UTC())
	if p.Status != "BLOCKED" || p.ArchiveAuthentication || p.AuthorizesExecution || p.ActualRestoreVerified {
		t.Fatal("declarations promoted to actual crypto/restore proof")
	}
}
