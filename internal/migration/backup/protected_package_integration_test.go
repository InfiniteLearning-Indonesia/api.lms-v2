//go:build qaage

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func actualPrivateIdentity(t *testing.T, original string) string {
	t.Helper()
	root := actualACLDirectory(t, false)
	raw, err := os.ReadFile(original)
	if err != nil {
		t.Fatal("synthetic identity unavailable")
	}
	path := filepath.Join(root, "identity.txt")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("synthetic identity protected copy rejected: %v", errors.Unwrap(err))
	}
	return path
}

func TestAgeActualProtectedOutput(t *testing.T) {
	ctx, tool, original, recipient := actualAgeFixture(t)
	identity := actualPrivateIdentity(t, original)
	root := actualACLDirectory(t, false)
	r := ProtectedAgeRequest{Stream: AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, Directory: root, Name: "payload.age", PlainSHA256: Hash([]byte("data")), IdentityPath: identity, CustodyRef: "synthetic-not-independent-custody"}
	p, err := SealProtectedAgeFile(ctx, tool, r, strings.NewReader("data"), time.Now().UTC())
	if err != nil || p.Status != "PASS_PROTECTED_ENCRYPTED_FILE_VERIFIED_ONLY" || !p.ProtectionVerified || !p.ArchiveAuthentication || p.AuthorizesExecution || p.IndependentCustodyVerified || p.ActualRestoreVerified {
		t.Fatal("protected output/readback failed")
	}
	before, err := os.ReadFile(filepath.Join(root, r.Name))
	if err != nil {
		t.Fatal("cipher fixture unavailable")
	}
	if v, err := SealProtectedAgeFile(ctx, tool, r, strings.NewReader("data"), time.Now().UTC()); err == nil || v.Status != "" {
		t.Fatal("overwrite accepted")
	}
	after, _ := os.ReadFile(filepath.Join(root, r.Name))
	if Hash(before) != Hash(after) {
		t.Fatal("existing file changed")
	}
	_, _, _, otherRecipient := actualAgeFixture(t)
	bad := r
	bad.Name = "wrong-key.age"
	bad.Stream.Recipient = otherRecipient
	if v, err := SealProtectedAgeFile(ctx, tool, bad, strings.NewReader("data"), time.Now().UTC()); err == nil || v.Status != "" {
		t.Fatal("wrong-key output receipt accepted")
	}
	if _, err := os.Stat(filepath.Join(root, bad.Name)); err != nil {
		t.Fatal("failed ciphertext was not retained")
	}
	bad = r
	bad.Name = "short.age"
	if v, err := SealProtectedAgeFile(ctx, tool, bad, strings.NewReader("abc"), time.Now().UTC()); err == nil || v.Status != "" {
		t.Fatal("partial source promoted")
	}
	bad = r
	bad.Name = "public.age"
	bad.Directory = actualACLDirectory(t, true)
	if v, err := SealProtectedAgeFile(ctx, tool, bad, strings.NewReader("data"), time.Now().UTC()); err == nil || v.Status != "" {
		t.Fatal("public directory accepted")
	}
	if _, err := os.Stat(filepath.Join(bad.Directory, bad.Name)); !os.IsNotExist(err) {
		t.Fatal("public path got an artifact")
	}
	alias := filepath.Join(filepath.Dir(identity), "identity-alias.txt")
	if os.Link(identity, alias) != nil {
		t.Fatal("hardlink fixture unavailable")
	}
	if protectedIdentity(root, identity) == nil {
		t.Fatal("multiply-linked identity accepted")
	}
}

func actualAuthenticatedFixture(t *testing.T) (context.Context, AuthenticatedPackageRequest, time.Time) {
	t.Helper()
	ctx, tool, original, recipient := actualAgeFixture(t)
	p, now := packageFixture(t)
	oldRoot := p.Intake.ArtifactRoot
	p.Intake.ArtifactRoot = actualACLDirectory(t, false)
	p.Intake.Capacity.DumpBytes = 4 << 20
	p.Intake.Capacity.SQLBytes = 4 << 20
	p.Intake.Capacity.ObjectBytes = 4 << 20
	p.Intake.Capacity.RestoreBytes = 16 << 20
	r := AuthenticatedPackageRequest{Version: 1, Package: p, Tool: tool, IdentityPath: actualPrivateIdentity(t, original), Artifacts: []AuthenticatedArtifact{}}
	objectPlain := []byte("synthetic private object bytes; not a recovered business file")
	r.Package.Artifacts = append(r.Package.Artifacts, PackageArtifact{Role: "object_bundle", Name: "objects.bundle.age", SnapshotID: p.Snapshot.ID, Encryption: "age-v1"})
	for i := range r.Package.Artifacts {
		a := &r.Package.Artifacts[i]
		if a.Role == "schema_inventory" || a.Role == "file_inventory" {
			raw, err := os.ReadFile(filepath.Join(oldRoot, a.Name))
			if err != nil {
				t.Fatal("metadata fixture read")
			}
			if os.WriteFile(filepath.Join(r.Package.Intake.ArtifactRoot, a.Name), raw, 0600) != nil {
				t.Fatal("metadata fixture copy")
			}
			continue
		}
		plain := objectPlain
		if a.Role != "object_bundle" {
			var err error
			plain, err = os.ReadFile(filepath.Join(oldRoot, a.Name))
			if err != nil {
				t.Fatal("plain fixture unavailable")
			}
			a.Name += ".age"
		}
		a.Encryption = "age-v1"
		digest := StreamDigest{int64(len(plain)), Hash(plain)}
		binding, err := PackageAgeBinding(r.Package, *a, digest)
		if err != nil {
			t.Fatal("binding fixture")
		}
		v, err := SealProtectedAgeFile(ctx, tool, ProtectedAgeRequest{Stream: AgeStreamRequest{binding, recipient, digest.Bytes, digest.Bytes + 65536},
			Directory: r.Package.Intake.ArtifactRoot, Name: a.Name, PlainSHA256: digest.SHA256, IdentityPath: r.IdentityPath, CustodyRef: "fixture-custody-only"}, strings.NewReader(string(plain)), now)
		if err != nil {
			t.Fatal("protected fixture encryption failed")
		}
		a.Bytes, a.SHA256 = v.Receipt.Cipher.Bytes, v.Receipt.Cipher.SHA256
		r.Artifacts = append(r.Artifacts, AuthenticatedArtifact{a.Role, a.Name, v.Receipt})
	}
	bundle := r.Package.Artifacts[len(r.Package.Artifacts)-1]
	rewritePackageMetadata(t, &r.Package, "file_inventory", func(m map[string]any) {
		m["no_objects_review_ref"] = ""
		m["bundle_sha256"] = bundle.SHA256
		m["objects"] = []any{map[string]any{"ref": "fixture-private-object", "bytes": len(objectPlain), "sha256": Hash(objectPlain)}}
	})
	return ctx, r, now
}

func TestAgeActualAuthenticatedPackage(t *testing.T) {
	ctx, r, now := actualAuthenticatedFixture(t)
	p := AuthenticatePackage(ctx, r, now)
	if p.Status != "OFFLINE_PACKAGE_AUTHENTICATED_NOT_AUTHORIZED" || p.AuthenticatedArtifactCount != 3 || !p.ArchiveAuthentication || !p.LocalProtectionVerified || p.AuthorizesExecution || p.ActualRestoreVerified || p.RealBaselineVerified || p.IndependentCustodyVerified || len(p.ExternalGates) == 0 {
		t.Fatal("authenticated package proof boundary or positive fixture failed")
	}
	r.Artifacts[0], r.Artifacts[1] = r.Artifacts[1], r.Artifacts[0]
	if second := AuthenticatePackage(ctx, r, now); second.Status != p.Status || second.PlanSHA256 != p.PlanSHA256 {
		t.Fatal("receipt order changes semantic plan")
	}
	binary := os.Getenv("QA_BASELINE_PACKAGE_BINARY")
	if !filepath.IsAbs(binary) {
		t.Fatal("compiled baseline-package CLI required; not skipped")
	}
	command := exec.CommandContext(ctx, binary, "--authenticate-only")
	command.Env = ageEnvironment()
	raw, _ := json.Marshal(r)
	command.Stdin = strings.NewReader(string(raw))
	output, err := command.Output()
	var cli AuthenticatedPackageReport
	if err != nil || json.Unmarshal(output, &cli) != nil || cli.Status != p.Status || cli.PlanSHA256 != p.PlanSHA256 || cli.AuthorizesExecution || cli.ActualRestoreVerified {
		t.Fatal("actual authenticated package CLI contract failed")
	}
}

func TestAgeActualPackageTamperAndBinding(t *testing.T) {
	ctx, r, now := actualAuthenticatedFixture(t)
	clone := func() AuthenticatedPackageRequest {
		raw, _ := json.Marshal(r)
		var c AuthenticatedPackageRequest
		json.Unmarshal(raw, &c)
		return c
	}
	bad := clone()
	bad.Artifacts = bad.Artifacts[:2]
	if p := AuthenticatePackage(ctx, bad, now); p.Status != "BLOCKED" || p.ArchiveAuthentication {
		t.Fatal("incomplete encrypted coverage accepted")
	}
	bad = clone()
	v := &bad.Artifacts[0]
	v.Receipt.Plain.SHA256 = strings.Repeat("f", 64)
	for _, a := range bad.Package.Artifacts {
		if a.Role == v.Role {
			v.Receipt.Binding, _ = PackageAgeBinding(bad.Package, a, v.Receipt.Plain)
		}
	}
	if _, err := authenticatedCoverage(bad); err != nil {
		t.Fatal("forged declaration did not reach real encrypted binding check")
	}
	if p := AuthenticatePackage(ctx, bad, now); p.Status != "BLOCKED" || p.ArchiveAuthentication {
		t.Fatal("rebound plaintext declaration accepted despite encrypted manifest")
	}
	_, _, otherIdentity, _ := actualAgeFixture(t)
	bad = clone()
	bad.IdentityPath = actualPrivateIdentity(t, otherIdentity)
	if p := AuthenticatePackage(ctx, bad, now); p.Status != "BLOCKED" || p.ArchiveAuthentication {
		t.Fatal("wrong package identity accepted")
	}
	bad = clone()
	a := &bad.Package.Artifacts[0]
	path := filepath.Join(bad.Package.Intake.ArtifactRoot, a.Name)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 1 {
		t.Fatal("tamper fixture read")
	}
	raw[len(raw)-1] ^= 1
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("tamper fixture write")
	}
	a.SHA256 = Hash(raw)
	for i := range bad.Artifacts {
		if bad.Artifacts[i].Role == a.Role {
			bad.Artifacts[i].Receipt.Cipher.SHA256 = a.SHA256
		}
	}
	if integrity := CheckPackage(ctx, bad.Package, now); integrity.Status != "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED" {
		t.Fatal("tamper test stopped before actual authentication")
	}
	if p := AuthenticatePackage(ctx, bad, now); p.Status != "BLOCKED" || p.ArchiveAuthentication {
		t.Fatal("updated outer checksums bypassed actual authentication")
	}
}
