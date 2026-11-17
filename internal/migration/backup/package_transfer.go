package backup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

type SealPackageArtifactRequest struct {
	Version         int            `json:"version"`
	Package         PackageRequest `json:"package"`
	Tool            AgeTool        `json:"tool"`
	IdentityPath    string         `json:"identity_path"`
	Role            string         `json:"role"`
	SourceDirectory string         `json:"source_directory"`
	SourceName      string         `json:"source_name"`
	Plain           StreamDigest   `json:"plain"`
	Recipient       string         `json:"recipient"`
	CustodyRef      string         `json:"custody_ref"`
}

type RecoverPackageArtifactRequest struct {
	Version        int                         `json:"version"`
	Authentication AuthenticatedPackageRequest `json:"authentication"`
	Role           string                      `json:"role"`
	Directory      string                      `json:"directory"`
	Name           string                      `json:"name"`
}

type RecoveredArtifactReport struct {
	Status                     string       `json:"status"`
	Name                       string       `json:"name"`
	Plain                      StreamDigest `json:"plain"`
	AuthenticatedPackageSHA256 string       `json:"authenticated_package_sha256"`
	ArchiveAuthentication      bool         `json:"archive_authentication_verified"`
	LocalProtectionVerified    bool         `json:"local_protection_verified"`
	ActualRestoreVerified      bool         `json:"actual_restore_verified"`
	AuthorizesExecution        bool         `json:"authorizes_execution"`
	DatabaseAccess             string       `json:"database_access"`
}

func DecodeSealPackageArtifact(r io.Reader) (SealPackageArtifactRequest, error) {
	var v SealPackageArtifactRequest
	if decodePackageJSON(r, MaxPackageJSON, &v) != nil || v.Version != 1 {
		return SealPackageArtifactRequest{}, errPackage
	}
	return v, nil
}
func DecodeRecoverPackageArtifact(r io.Reader) (RecoverPackageArtifactRequest, error) {
	var v RecoverPackageArtifactRequest
	if decodePackageJSON(r, MaxPackageJSON, &v) != nil || v.Version != 1 {
		return RecoverPackageArtifactRequest{}, errPackage
	}
	return v, nil
}

// Seals supplied private local bytes; does not export a database or prove the
// source identity/snapshot declarations. Other ciphertext hashes may be pending.
func SealPackageArtifact(ctx context.Context, r SealPackageArtifactRequest, now time.Time) (ProtectedAgeReport, error) {
	if r.Version != 1 || !validPackageContext(ctx) || !streamArtifactName.MatchString(r.SourceName) ||
		privateDirectory(r.SourceDirectory) != nil || !identityOutsideArtifacts(r.Package.Intake.ArtifactRoot, filepath.Join(r.SourceDirectory, r.SourceName)) {
		return ProtectedAgeReport{}, errProtection
	}
	p := CheckIntake(r.Package.Intake, now)
	if p.Status != "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED" || p.AuthorizesExecution {
		return ProtectedAgeReport{}, errProtection
	}
	var artifact PackageArtifact
	found := 0
	for _, a := range r.Package.Artifacts {
		if a.Role == r.Role {
			artifact = a
			found++
		}
	}
	if found != 1 {
		return ProtectedAgeReport{}, errPackage
	}
	binding, err := PackageAgeBinding(r.Package, artifact, r.Plain)
	if err != nil {
		return ProtectedAgeReport{}, errPackage
	}
	limit := map[string]uint64{"db_custom": r.Package.Intake.Capacity.DumpBytes, "db_sql": r.Package.Intake.Capacity.SQLBytes, "object_bundle": r.Package.Intake.Capacity.ObjectBytes}[r.Role]
	// Honor the declared ciphertext budget, including age/tar overhead for
	// large files; a fixed extra MiB is not a general streaming-size bound.
	if limit > uint64(MaxStreamArtifactBytes) {
		limit = uint64(MaxStreamArtifactBytes)
	}
	maximum := int64(limit)
	if maximum < r.Plain.Bytes+65536 {
		return ProtectedAgeReport{}, errProtection
	}
	root, file, err := openPackageFile(r.SourceDirectory, PackageArtifact{Name: r.SourceName, Bytes: r.Plain.Bytes})
	if err != nil {
		return ProtectedAgeReport{}, errProtection
	}
	defer root.Close()
	defer file.Close()
	if privateHandle(file) != nil || !singleLink(file) {
		return ProtectedAgeReport{}, errProtection
	}
	return SealProtectedAgeFile(ctx, r.Tool, ProtectedAgeRequest{Stream: AgeStreamRequest{binding, r.Recipient, r.Plain.Bytes, maximum},
		Directory: r.Package.Intake.ArtifactRoot, Name: artifact.Name, PlainSHA256: r.Plain.SHA256, IdentityPath: r.IdentityPath, CustodyRef: r.CustodyRef}, file, now)
}

// Recovery authenticates before file creation, then authenticates again while
// writing ONLY to a new private file. Failed second-pass output remains partial
// and unaccepted. This explicit operation never restores a database or prints
// plaintext. Trusted immutable operator-controlled input/host remains required.
func RecoverPackageArtifact(ctx context.Context, r RecoverPackageArtifactRequest, now time.Time) (RecoveredArtifactReport, error) {
	fail := func() (RecoveredArtifactReport, error) { return RecoveredArtifactReport{}, errProtection }
	auth := r.Authentication
	if r.Version != 1 || !validPackageContext(ctx) || !streamArtifactName.MatchString(r.Name) ||
		!identityOutsideArtifacts(auth.Package.Intake.ArtifactRoot, filepath.Join(r.Directory, r.Name)) ||
		!identityOutsideArtifacts(r.Directory, auth.IdentityPath) {
		return fail()
	}
	p := AuthenticatePackage(ctx, auth, now)
	if p.Status != "OFFLINE_PACKAGE_AUTHENTICATED_NOT_AUTHORIZED" || !p.ArchiveAuthentication || p.AuthorizesExecution {
		return fail()
	}
	var a PackageArtifact
	var expected AgeStreamReceipt
	for _, v := range auth.Package.Artifacts {
		if v.Role == r.Role {
			a = v
		}
	}
	for _, v := range auth.Artifacts {
		if v.Role == r.Role {
			expected = v.Receipt
		}
	}
	if a.Role == "" || expected.Plain.Bytes <= 0 || ProtectedDestination(r.Directory, uint64(expected.Plain.Bytes)+ProtectedReserveBytes) != nil {
		return fail()
	}
	sourceRoot, source, err := openPackageFile(auth.Package.Intake.ArtifactRoot, a)
	if err != nil {
		return fail()
	}
	defer sourceRoot.Close()
	defer source.Close()
	if privateHandle(source) != nil || !singleLink(source) {
		return fail()
	}
	if _, err = VerifyAgeStream(ctx, auth.Tool, auth.IdentityPath, expected, source, now); err != nil {
		return fail()
	}
	if _, err = source.Seek(0, 0); err != nil {
		return fail()
	}
	original, err := os.Lstat(r.Directory)
	if err != nil {
		return fail()
	}
	root, err := os.OpenRoot(r.Directory)
	if err != nil {
		return fail()
	}
	defer root.Close()
	if !protectedRootStable(root, r.Directory, original) {
		return fail()
	}
	file, err := root.OpenFile(r.Name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fail()
	}
	defer file.Close()
	if privateHandle(file) != nil || !singleLink(file) {
		return fail()
	}
	if _, err = verifyAgeStreamTo(ctx, auth.Tool, auth.IdentityPath, expected, source, now, file); err != nil || file.Sync() != nil {
		return fail()
	}
	if _, err = file.Seek(0, 0); err != nil {
		return fail()
	}
	digest, err := DigestStream(ctx, file, expected.Plain.Bytes, expected.Plain.Bytes)
	if err != nil || digest != expected.Plain || !protectedRootStable(root, r.Directory, original) || privateHandle(file) != nil || !singleLink(file) {
		return fail()
	}
	opened, err := file.Stat()
	named, err2 := root.Lstat(r.Name)
	if err != nil || err2 != nil || !os.SameFile(opened, named) || intakeReparse(filepath.Join(r.Directory, r.Name)) || protectedIdentity(r.Directory, auth.IdentityPath) != nil {
		return fail()
	}
	if file.Sync() != nil || file.Close() != nil {
		return fail()
	}
	return RecoveredArtifactReport{Status: "OFFLINE_ARTIFACT_RECOVERED_NOT_AUTHORIZED", Name: r.Name, Plain: digest, AuthenticatedPackageSHA256: p.PlanSHA256,
		ArchiveAuthentication: true, LocalProtectionVerified: true, DatabaseAccess: "NONE"}, nil
}
