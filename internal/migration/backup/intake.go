package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const MaxIntakeJSON = 64 << 10
const MinimumBaselineReserve uint64 = 8 << 30

var schemaName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
var commitHash = regexp.MustCompile(`^[a-f0-9]{40}$`)

type IntakeSource struct {
	IdentitySHA256 string `json:"identity_sha256"`
	SchemaSHA256   string `json:"schema_sha256"`
	DeployedCommit string `json:"deployed_commit"`
	ReceiptRef     string `json:"receipt_ref"`
	ObservedAt     string `json:"observed_at"`
}

type IntakeTarget struct {
	IdentitySHA256 string `json:"identity_sha256"`
	Kind           string `json:"kind"`
	Namespace      string `json:"namespace"`
}

type IntakeDeclarations struct {
	TLSMode          string `json:"tls_mode"`
	SourceReadOnly   bool   `json:"source_read_only"`
	AccessReviewed   bool   `json:"access_reviewed"`
	OwnerRef         string `json:"owner_ref"`
	ReviewerRef      string `json:"reviewer_ref"`
	ApprovalRef      string `json:"approval_ref"`
	KeyCustodyRef    string `json:"key_custody_ref"`
	ArtifactScopeRef string `json:"artifact_scope_ref"`
}

type IntakeCapacity struct {
	DumpBytes    uint64 `json:"dump_bytes"`
	SQLBytes     uint64 `json:"sql_bytes"`
	ObjectBytes  uint64 `json:"object_bytes"`
	RestoreBytes uint64 `json:"restore_bytes"`
	ReserveBytes uint64 `json:"reserve_bytes"`
}

// IntakeRequest contains declarations, never a DSN, credential or trusted
// approval. No operation against a source/target database is implemented here.
type IntakeRequest struct {
	Version      int                `json:"version"`
	RunID        string             `json:"run_id"`
	Source       IntakeSource       `json:"source"`
	Target       IntakeTarget       `json:"target"`
	Schemas      []string           `json:"schemas"`
	ArtifactRoot string             `json:"artifact_root"`
	Declarations IntakeDeclarations `json:"declarations"`
	Capacity     IntakeCapacity     `json:"capacity"`
}

type IntakeReport struct {
	Status               string   `json:"status"`
	PlanSHA256           string   `json:"plan_sha256,omitempty"`
	RequiredBytes        uint64   `json:"required_bytes"`
	ObservedFreeBytes    uint64   `json:"observed_free_bytes"`
	FilesystemChecked    bool     `json:"filesystem_checked"`
	Blockers             []string `json:"blockers"`
	ExternalGates        []string `json:"external_gates"`
	AuthorizesExecution  bool     `json:"authorizes_execution"`
	RealBaselineVerified bool     `json:"real_baseline_verified"`
	DatabaseAccess       string   `json:"database_access"`
}

// DecodeIntake rejects oversized input, duplicate keys (including escaped
// duplicates), case-variant/unknown fields, nulls and trailing JSON. Never echo
// decoder errors: they can contain an operator's accidental secret input.
func DecodeIntake(r io.Reader) (IntakeRequest, error) {
	refuse := errors.New("INVALID_INTAKE_INPUT")
	raw, err := io.ReadAll(io.LimitReader(r, MaxIntakeJSON+1))
	if err != nil || len(raw) > MaxIntakeJSON {
		return IntakeRequest{}, refuse
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if scanIntakeJSON(decoder, 0) != nil {
		return IntakeRequest{}, refuse
	}
	if _, err = decoder.Token(); err != io.EOF {
		return IntakeRequest{}, refuse
	}
	var request IntakeRequest
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Version != 1 {
		return IntakeRequest{}, refuse
	}
	canonical, _ := json.Marshal(request)
	if !intakeExactKeys(raw, canonical) {
		return IntakeRequest{}, refuse
	}
	return request, nil
}

func scanIntakeJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("DEPTH")
	}
	t, err := d.Token()
	if err != nil || t == nil {
		return errors.New("TOKEN")
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return errors.New("KEY")
				}
				seen[name] = true
				if scanIntakeJSON(d, depth+1) != nil {
					return errors.New("VALUE")
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("OBJECT")
			}
		case '[':
			for d.More() {
				if scanIntakeJSON(d, depth+1) != nil {
					return errors.New("VALUE")
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("ARRAY")
			}
		default:
			return errors.New("DELIMITER")
		}
	}
	return nil
}

func intakeExactKeys(raw, canonical []byte) bool {
	var a, b map[string]json.RawMessage
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal(canonical, &b) != nil || a == nil {
		return false
	}
	for key, value := range a {
		other, exists := b[key]
		if !exists {
			return false
		}
		if len(value) > 0 && value[0] == '{' && !intakeExactKeys(value, other) {
			return false
		}
	}
	return true
}

func intakeCapacity(c IntakeCapacity) (uint64, bool) {
	if c.DumpBytes == 0 || c.SQLBytes == 0 || c.RestoreBytes == 0 || c.ReserveBytes < MinimumBaselineReserve {
		return 0, false
	}
	var total uint64
	for _, size := range []uint64{c.DumpBytes, c.SQLBytes, c.ObjectBytes, c.RestoreBytes, c.ReserveBytes} {
		if size > math.MaxUint64-total {
			return 0, false
		}
		total += size
	}
	return total, true
}

func intakeRoot(path string) bool {
	if !SafeRoot(path) || filepath.VolumeName(path) == path || bytes.ContainsAny([]byte(path), "\r\n\x00") {
		return false
	}
	for p := path; ; p = filepath.Dir(p) {
		if intakeReparse(p) {
			return false
		}
		if filepath.Dir(p) == p {
			return true
		}
	}
}

// CheckIntake performs bounded local filesystem observations only. All source
// identity/TLS/role/approval/custody fields remain unverified declarations.
func CheckIntake(r IntakeRequest, now time.Time) IntakeReport {
	return checkIntake(r, now, AvailableBytes)
}

func checkIntake(r IntakeRequest, now time.Time, free func(string) (uint64, error)) IntakeReport {
	p := IntakeReport{Status: "BLOCKED", Blockers: []string{}, DatabaseAccess: "NONE", ExternalGates: []string{
		"SOURCE_TARGET_IDENTITY_NOT_INDEPENDENTLY_VERIFIED", "DEPLOYED_ARTIFACT_NOT_VERIFIED",
		"SOURCE_TLS_ROLE_AND_APPROVAL_NOT_VERIFIED", "DESTINATION_ACL_ENCRYPTION_KEY_CUSTODY_NOT_VERIFIED",
		"CONSISTENT_SNAPSHOT_FILES_AND_RESTORE_NOT_VERIFIED",
	}}
	block := func(code string) { p.Blockers = append(p.Blockers, code) }
	if r.Version != 1 || !token.MatchString(r.RunID) {
		block("INVALID_INTAKE_VERSION_OR_RUN")
	}
	if !fingerprint.MatchString(r.Source.IdentitySHA256) || !fingerprint.MatchString(r.Target.IdentitySHA256) || r.Source.IdentitySHA256 == r.Target.IdentitySHA256 {
		block("DISTINCT_DECLARED_IDENTITIES_REQUIRED")
	}
	if !fingerprint.MatchString(r.Source.SchemaSHA256) || !commitHash.MatchString(r.Source.DeployedCommit) || !token.MatchString(r.Source.ReceiptRef) {
		block("SOURCE_REFERENCES_INCOMPLETE")
	}
	observed, err := time.Parse(time.RFC3339Nano, r.Source.ObservedAt)
	_, offset := observed.Zone()
	if err != nil || offset != 0 || observed.IsZero() || observed.After(now) || now.Sub(observed) > 24*time.Hour {
		block("SOURCE_DECLARATION_STALE_OR_INVALID")
	}
	if r.Target.Kind != "new-local-restore" || !token.MatchString(r.Target.Namespace) {
		block("NEW_LOCAL_TARGET_REQUIRED")
	}
	if len(r.Schemas) == 0 || len(r.Schemas) > 128 {
		block("EXPLICIT_BOUNDED_SCHEMA_SCOPE_REQUIRED")
	}
	seen := map[string]bool{}
	for _, s := range r.Schemas {
		if !schemaName.MatchString(s) || seen[s] || len(s) >= 3 && s[:3] == "pg_" || s == "information_schema" {
			block("INVALID_OR_DUPLICATE_SCHEMA_SCOPE")
		}
		seen[s] = true
	}
	d := r.Declarations
	if d.TLSMode != "verify-full" || !d.SourceReadOnly || !d.AccessReviewed || d.OwnerRef == d.ReviewerRef {
		block("OPERATOR_DECLARATIONS_INCOMPLETE")
	}
	for _, ref := range []string{d.OwnerRef, d.ReviewerRef, d.ApprovalRef, d.KeyCustodyRef, d.ArtifactScopeRef} {
		if !token.MatchString(ref) {
			block("OPERATOR_REFERENCES_INCOMPLETE")
			break
		}
	}
	p.RequiredBytes, _ = intakeCapacity(r.Capacity)
	if _, valid := intakeCapacity(r.Capacity); !valid {
		block("CAPACITY_INVALID_OR_OVERFLOW")
	}
	// Refuse malformed scope before any path or capacity observation.
	if len(p.Blockers) != 0 {
		return p
	}
	if !intakeRoot(r.ArtifactRoot) {
		block("UNSAFE_ARTIFACT_ROOT")
		return p
	}
	if _, err := os.Lstat(filepath.Join(r.ArtifactRoot, r.RunID)); !os.IsNotExist(err) {
		block("CANDIDATE_EXISTS_OR_UNAVAILABLE")
		return p
	}
	p.ObservedFreeBytes, err = free(r.ArtifactRoot)
	if err != nil {
		block("FILESYSTEM_CAPACITY_UNAVAILABLE")
		return p
	}
	p.FilesystemChecked = true
	if p.ObservedFreeBytes < p.RequiredBytes {
		block("FILESYSTEM_CAPACITY_INSUFFICIENT")
		return p
	}
	r.Schemas = append([]string(nil), r.Schemas...)
	sort.Strings(r.Schemas)
	r.Source.ObservedAt = observed.UTC().Format(time.RFC3339Nano)
	encoded, _ := json.Marshal(r)
	p.PlanSHA256 = Hash(encoded)
	p.Status = "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED"
	return p
}
