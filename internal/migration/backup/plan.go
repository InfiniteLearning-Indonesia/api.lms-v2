// Package backup validates offline plans. Caller declarations are NOT signed
// operational approval; this package never opens a database or invokes a shell.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var token = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var fingerprint = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Request struct {
	RunID             string   `json:"run_id"`
	Source            string   `json:"source_fingerprint"`
	Target            string   `json:"target_fingerprint"`
	Scope             []string `json:"schemas"`
	ArtifactRoot      string   `json:"artifact_root"`
	RequiredBytes     int64    `json:"required_bytes"`
	AvailableBytes    int64    `json:"available_bytes"`
	TLSVerified       bool     `json:"tls_verified"`
	ReadOnlyRole      bool     `json:"read_only_role"`
	AccessReviewed    bool     `json:"access_reviewed"`
	ApprovalReference string   `json:"approval_reference"`
}

type Plan struct {
	Status              string   `json:"status"`
	RunID               string   `json:"run_id"`
	ScopeSHA256         string   `json:"scope_sha256"`
	Archive             string   `json:"archive"`
	SQL                 string   `json:"sql"`
	Blockers            []string `json:"blockers"`
	AuthorizesExecution bool     `json:"authorizes_execution"`
}

func Decode(r io.Reader) (Request, error) {
	var request Request
	decoder := json.NewDecoder(io.LimitReader(r, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, errors.New("INVALID_PLAN_INPUT")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Request{}, errors.New("TRAILING_PLAN_INPUT")
	}
	return request, nil
}

func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// SafeRoot rejects symlinks in every existing path component. Host ACL/encryption
// and available space remain independently verified operational prerequisites.
func SafeRoot(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, `\\`) {
		return false
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
	}
	return filepath.Dir(path) != path
}

func Build(r Request) Plan {
	p := Plan{Status: "BLOCKED", RunID: r.RunID, Blockers: []string{}}
	block := func(s string) { p.Blockers = append(p.Blockers, s) }
	if !token.MatchString(r.RunID) {
		block("INVALID_RUN_ID")
	}
	if !fingerprint.MatchString(r.Source) || !fingerprint.MatchString(r.Target) || r.Source == r.Target {
		block("DISTINCT_IDENTITIES_REQUIRED")
	}
	if len(r.Scope) == 0 {
		block("EXPLICIT_SCOPE_REQUIRED")
	}
	seen := map[string]bool{}
	for _, schema := range r.Scope {
		if !token.MatchString(schema) || seen[schema] || strings.HasPrefix(schema, "pg_") || schema == "information_schema" {
			block("INVALID_OR_DUPLICATE_SCHEMA")
		}
		seen[schema] = true
	}
	if !SafeRoot(r.ArtifactRoot) {
		block("UNSAFE_ARTIFACT_ROOT")
	}
	if r.RequiredBytes <= 0 || r.AvailableBytes < r.RequiredBytes {
		block("CAPACITY_NOT_DECLARED_SUFFICIENT")
	}
	if !r.TLSVerified || !r.ReadOnlyRole || !r.AccessReviewed || !token.MatchString(r.ApprovalReference) {
		block("OPERATOR_PREREQUISITES_MISSING")
	}
	if len(p.Blockers) > 0 {
		return p
	}
	base := filepath.Join(r.ArtifactRoot, r.RunID)
	if _, err := os.Lstat(base); !os.IsNotExist(err) {
		block("DESTINATION_ALREADY_EXISTS_OR_UNAVAILABLE")
		return p
	}
	data, _ := json.Marshal(struct {
		Source, Target string
		Scope          []string
	}{r.Source, r.Target, r.Scope})
	p.ScopeSHA256 = Hash(data)
	p.Archive = filepath.Join(base, "database.dump")
	p.SQL = filepath.Join(base, "database.sql")
	p.Status = "PLAN_VALID_NOT_AUTHORIZED"
	return p
}

type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type Receipt struct {
	RunID       string     `json:"run_id"`
	ScopeSHA256 string     `json:"scope_sha256"`
	CreatedAt   time.Time  `json:"created_at"`
	Artifacts   []Artifact `json:"artifacts"`
}

// VerifyArtifacts proves bytes against an offline manifest only: NOT archive
// readability, snapshot completeness, isolated restore, or migration authority.
func VerifyArtifacts(root string, r Receipt, scope string, now time.Time) error {
	refuse := errors.New("ARTIFACT_VERIFICATION_FAILED")
	if !SafeRoot(root) || !token.MatchString(r.RunID) || !fingerprint.MatchString(scope) || r.ScopeSHA256 != scope || now.Before(r.CreatedAt) || now.Sub(r.CreatedAt) > 24*time.Hour || len(r.Artifacts) != 2 {
		return refuse
	}
	seen := map[string]bool{}
	for _, a := range r.Artifacts {
		if (a.Name != "database.dump" && a.Name != "database.sql") || seen[a.Name] || !fingerprint.MatchString(a.SHA256) || a.Bytes <= 0 {
			return refuse
		}
		seen[a.Name] = true
		path := filepath.Join(root, a.Name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != a.Bytes {
			return refuse
		}
		file, err := os.Open(path)
		if err != nil {
			return refuse
		}
		h := sha256.New()
		n, err := io.Copy(h, file)
		closeErr := file.Close()
		if err != nil || closeErr != nil || n != a.Bytes || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
			return refuse
		}
	}
	return nil
}
