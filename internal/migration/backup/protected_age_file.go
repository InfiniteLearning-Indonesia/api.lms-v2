package backup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ProtectedReserveBytes uint64 = 8 << 30

type ProtectedAgeRequest struct {
	Stream       AgeStreamRequest `json:"stream"`
	Directory    string           `json:"directory"`
	Name         string           `json:"name"`
	PlainSHA256  string           `json:"plain_sha256"`
	IdentityPath string           `json:"identity_path"`
	CustodyRef   string           `json:"custody_ref"`
}

type ProtectedAgeReport struct {
	Status                     string           `json:"status"`
	Receipt                    AgeStreamReceipt `json:"receipt"`
	Name                       string           `json:"name"`
	CustodyRef                 string           `json:"custody_ref"`
	ProtectionVerified         bool             `json:"protection_verified"`
	ArchiveAuthentication      bool             `json:"archive_authentication_verified"`
	IndependentCustodyVerified bool             `json:"independent_custody_verified"`
	ActualRestoreVerified      bool             `json:"actual_restore_verified"`
	AuthorizesExecution        bool             `json:"authorizes_execution"`
}

func identityOutsideArtifacts(directory, identity string) bool {
	if !intakeRoot(directory) || !filepath.IsAbs(identity) || filepath.Clean(identity) != identity {
		return false
	}
	rel, err := filepath.Rel(directory, identity)
	// Different local Windows volumes are necessarily outside each other.
	// The identity directory still needs actual private/root/reparse checks.
	if err != nil && filepath.VolumeName(directory) != "" && filepath.VolumeName(identity) != "" &&
		!strings.EqualFold(filepath.VolumeName(directory), filepath.VolumeName(identity)) && !strings.HasPrefix(identity, `\\`) {
		return true
	}
	return err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Actual local protection, not a declaration or independent custody proof.
func protectedIdentity(directory, identity string) error {
	if !identityOutsideArtifacts(directory, identity) || privateDirectory(filepath.Dir(identity)) != nil {
		return errProtection
	}
	root, file, err := openPackageFile(filepath.Dir(identity), PackageArtifact{Name: filepath.Base(identity), Bytes: identitySize(identity)})
	if err != nil {
		return errProtection
	}
	defer root.Close()
	defer file.Close()
	if privateHandle(file) != nil || !singleLink(file) || validateAgeIdentity(identity) != nil {
		return errProtection
	}
	return nil
}

func identitySize(path string) int64 {
	i, err := os.Lstat(path)
	if err != nil || i.Size() <= 0 || i.Size() > 16<<10 {
		return -1
	}
	return i.Size()
}

func protectedRootStable(root *os.Root, path string, original os.FileInfo) bool {
	if !intakeRoot(path) {
		return false
	}
	anchored, err := root.Stat(".")
	current, err2 := os.Lstat(path)
	if err != nil || err2 != nil || !os.SameFile(original, anchored) || !os.SameFile(original, current) {
		return false
	}
	f, err := root.Open(".")
	if err != nil {
		return false
	}
	defer f.Close()
	return privateHandle(f) == nil
}

// SealProtectedAgeFile creates ciphertext exclusively in an existing protected
// operator-owned directory. It never changes ACLs, overwrites, renames, deletes
// or writes plaintext. Any failed/partial file is retained and has no receipt.
// Host/path control must be trusted; this is not a public endpoint or exporter.
func SealProtectedAgeFile(ctx context.Context, tool AgeTool, request ProtectedAgeRequest, source io.Reader, now time.Time) (ProtectedAgeReport, error) {
	fail := func() (ProtectedAgeReport, error) { return ProtectedAgeReport{}, errProtection }
	s := request.Stream
	if !validPackageContext(ctx) || source == nil || !streamArtifactName.MatchString(request.Name) ||
		!strings.HasSuffix(request.Name, ".age") || !fingerprint.MatchString(request.PlainSHA256) || !token.MatchString(request.CustodyRef) ||
		!s.Binding.valid(s.Binding.Target, now) || len(s.Recipient) < 24 || len(s.Recipient) > 4100 || !ageRecipient.MatchString(s.Recipient) ||
		s.PlainBytes <= 0 || s.PlainBytes > MaxStreamArtifactBytes-(1<<20) || s.MaxCipherBytes < s.PlainBytes+65536 || s.MaxCipherBytes > MaxStreamArtifactBytes ||
		ProtectedDestination(request.Directory, uint64(s.MaxCipherBytes)+ProtectedReserveBytes) != nil ||
		protectedIdentity(request.Directory, request.IdentityPath) != nil || ValidateAgeTool(ctx, tool, "age") != nil {
		return fail()
	}
	original, err := os.Lstat(request.Directory)
	if err != nil {
		return fail()
	}
	root, err := os.OpenRoot(request.Directory)
	if err != nil {
		return fail()
	}
	defer root.Close()
	if !protectedRootStable(root, request.Directory, original) {
		return fail()
	}
	// O_EXCL refuses existing files, links and retry-overwrite. Inherited ACL
	// is checked before payload bytes; a non-inheriting ACL safely leaves an
	// empty retained file and fails rather than relaxing permissions.
	file, err := root.OpenFile(request.Name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fail()
	}
	defer file.Close()
	if privateHandle(file) != nil || !singleLink(file) || !protectedRootStable(root, request.Directory, original) {
		return fail()
	}
	r, err := SealAgeStream(ctx, tool, s, source, file, now)
	if err != nil || r.Plain.SHA256 != request.PlainSHA256 || file.Sync() != nil {
		return fail()
	}
	if _, err = file.Seek(0, 0); err != nil {
		return fail()
	}
	r, err = VerifyAgeStream(ctx, tool, request.IdentityPath, r, file, now)
	if err != nil || protectedIdentity(request.Directory, request.IdentityPath) != nil || !protectedRootStable(root, request.Directory, original) || privateHandle(file) != nil || !singleLink(file) {
		return fail()
	}
	opened, err := file.Stat()
	named, err2 := root.Lstat(request.Name)
	if err != nil || err2 != nil || !os.SameFile(opened, named) || opened.Size() != r.Cipher.Bytes || intakeReparse(filepath.Join(request.Directory, request.Name)) {
		return fail()
	}
	if file.Sync() != nil || file.Close() != nil {
		return fail()
	}
	return ProtectedAgeReport{Status: "PASS_PROTECTED_ENCRYPTED_FILE_VERIFIED_ONLY", Receipt: r, Name: request.Name,
		CustodyRef: request.CustodyRef, ProtectionVerified: true, ArchiveAuthentication: true}, nil
}
