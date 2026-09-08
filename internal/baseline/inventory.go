// Package baseline inventories source declarations without executing application
// code, opening a database connection, or emitting environment/secret values.
package baseline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Declaration struct {
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
	Name string `json:"name,omitempty"`
}

type Repository struct {
	Label          string        `json:"label"`
	Commit         string        `json:"checkout_commit"`
	Branch         string        `json:"branch"`
	DeployedCommit string        `json:"deployed_commit"`
	Dirty          bool          `json:"dirty"`
	SourceHash     string        `json:"source_hash"`
	Files          int           `json:"typescript_files_scanned"`
	Declarations   []Declaration `json:"declarations"`
}

type ConfigObservation struct {
	Label            string `json:"label"`
	Present          bool   `json:"present"`
	Complete         bool   `json:"connection_fields_complete"`
	Fingerprint      string `json:"local_config_fingerprint,omitempty"`
	VerifiedDatabase bool   `json:"database_identity_verified"`
}

type Report struct {
	Version      int                 `json:"version"`
	GeneratedAt  string              `json:"generated_at"`
	Mode         string              `json:"mode"`
	Repositories []Repository        `json:"repositories"`
	Configs      []ConfigObservation `json:"local_config_observations"`
	Blockers     []string            `json:"blockers"`
	Warnings     []string            `json:"warnings"`
}

var entity = regexp.MustCompile(`@Entity\(\s*['"]([a-zA-Z0-9_]+)['"]\s*\)`)
var route = regexp.MustCompile(`@(Get|Post|Put|Patch|Delete|Options|Head|All)\s*\(`)
var controller = regexp.MustCompile(`@Controller\s*\(`)
var timer = regexp.MustCompile(`\b(setInterval|setTimeout)\s*\(|@(Cron|Interval|Timeout)\s*\(`)
var cascade = regexp.MustCompile(`onDelete\s*:\s*['"](CASCADE|SET NULL)['"]`)
var primary = regexp.MustCompile(`@(PrimaryGeneratedColumn|PrimaryColumn|Unique|Index)\s*\(`)
var relation = regexp.MustCompile(`@(ManyToOne|OneToMany|OneToOne|ManyToMany|JoinColumn)\s*\(`)
var startupDDL = regexp.MustCompile(`(?i)\b(CREATE TABLE|ALTER TABLE|ALTER TYPE|CREATE INDEX)\b`)

// ScanDeclarations records locations, never arbitrary source lines (which may
// contain credentials). These are declarations, NOT a verified database schema.
func ScanDeclarations(path, content string) []Declaration {
	out := []Declaration{}
	for i, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "*") {
			continue
		}
		add := func(kind, name string) { out = append(out, Declaration{kind, path, i + 1, name}) }
		if m := entity.FindStringSubmatch(line); m != nil {
			add("entity_table", m[1])
		}
		if m := route.FindStringSubmatch(line); m != nil {
			add("route_decorator", strings.ToUpper(m[1]))
		}
		if controller.MatchString(line) {
			add("controller", "")
		}
		if timer.MatchString(line) {
			add("scheduled_or_timer_call", "")
		}
		if m := cascade.FindStringSubmatch(line); m != nil {
			add("destructive_fk_action", m[1])
		}
		if primary.MatchString(line) {
			add("key_or_index_declaration", "")
		}
		if relation.MatchString(line) {
			add("relation_declaration", "")
		}
		if startupDDL.MatchString(line) {
			add("ddl_in_source", "")
		}
		if strings.Contains(line, "synchronize:") {
			add("schema_sync_config", "")
		}
	}
	return out
}

// ParseConnectionFingerprint accepts only the limited dotenv fields necessary
// to detect obviously identical local connections. It is never identity proof.
func ParseConnectionFingerprint(label string, raw []byte) ConfigObservation {
	values := map[string]string{}
	allowed := map[string]bool{"DB_HOST": true, "DB_PORT": true, "DB_USER": true, "DB_USERNAME": true, "DB_NAME": true, "DB_DATABASE": true}
	ambiguous := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !allowed[key] {
			continue
		}
		value = strings.TrimSpace(value)
		if _, exists := values[key]; exists {
			ambiguous = true
		}
		if strings.ContainsAny(value, "$`\x00") {
			ambiguous = true
		}
		if len(value) > 1 && (value[0] == '\'' || value[0] == '"') {
			if value[len(value)-1] != value[0] {
				ambiguous = true
			} else {
				value = value[1 : len(value)-1]
			}
		} else if at := strings.Index(value, " #"); at >= 0 {
			value = strings.TrimSpace(value[:at])
		}
		values[key] = value
	}
	choose := func(a, b string) string {
		if values[a] != "" {
			return values[a]
		}
		return values[b]
	}
	user, database := choose("DB_USER", "DB_USERNAME"), choose("DB_NAME", "DB_DATABASE")
	if values["DB_USER"] != "" && values["DB_USERNAME"] != "" && values["DB_USER"] != values["DB_USERNAME"] {
		ambiguous = true
	}
	if values["DB_NAME"] != "" && values["DB_DATABASE"] != "" && values["DB_NAME"] != values["DB_DATABASE"] {
		ambiguous = true
	}
	parts := []string{strings.ToLower(values["DB_HOST"]), values["DB_PORT"], user, database}
	complete := !ambiguous
	for _, value := range parts {
		if value == "" {
			complete = false
		}
	}
	result := ConfigObservation{Label: label, Present: true, Complete: complete}
	if complete {
		sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
		result.Fingerprint = hex.EncodeToString(sum[:])
	}
	return result
}

func readGit(ctx context.Context, repo string, args ...string) (string, error) {
	// Trust only this explicitly selected local checkout for this invocation.
	// No wildcard trust and no global Git configuration mutation.
	base := []string{"-c", "core.excludesFile=", "-c", "safe.directory=" + filepath.ToSlash(repo), "-C", repo}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	data, err := cmd.Output()
	if err != nil {
		return "", errors.New("git metadata unavailable")
	}
	return strings.TrimSpace(string(data)), nil
}

func InventoryRepository(ctx context.Context, root, label string) (Repository, error) {
	result := Repository{Label: label, DeployedCommit: "UNKNOWN", Declarations: []Declaration{}}
	var err error
	if result.Commit, err = readGit(ctx, root, "rev-parse", "HEAD"); err != nil {
		return result, err
	}
	if result.Branch, err = readGit(ctx, root, "branch", "--show-current"); err != nil {
		return result, err
	}
	status, err := readGit(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return result, err
	}
	result.Dirty = status != ""
	paths := []string{}
	err = filepath.WalkDir(filepath.Join(root, "src"), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("source inventory unavailable")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("source symlink requires review")
		}
		if !d.IsDir() && strings.HasSuffix(path, ".ts") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return result, errors.New("source file unreadable")
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
		result.Declarations = append(result.Declarations, ScanDeclarations(rel, string(data))...)
	}
	result.Files = len(paths)
	result.SourceHash = hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func Collect(ctx context.Context, workspace string) (Report, error) {
	result := Report{Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Mode: "LOCAL_SOURCE_READ_ONLY", Repositories: []Repository{}, Configs: []ConfigObservation{}, Blockers: []string{
		"DEPLOYED_ARTIFACT_NOT_VERIFIED", "DATABASE_IDENTITIES_NOT_VERIFIED", "DATABASE_ROLE_AND_CA_NOT_VERIFIED", "DATABASE_CATALOG_AND_CAPACITY_NOT_VERIFIED", "OPERATOR_APPROVAL_NOT_RECORDED",
	}, Warnings: []string{"Declaration scanner is an inventory aid, not a TypeScript parser or database schema verification.", "No network, database access, dotenv execution, or application startup performed."}}
	for _, label := range []string{"backend-live", "backend"} {
		repo, err := InventoryRepository(ctx, filepath.Join(workspace, label), label)
		if err != nil {
			return result, err
		}
		result.Repositories = append(result.Repositories, repo)
	}
	for _, label := range []string{"backend-live/.env", "backend/.env.staging", "backend/.env", "backend-v3/backend-go/.env.staging"} {
		path := filepath.Join(workspace, filepath.FromSlash(label))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			result.Configs = append(result.Configs, ConfigObservation{Label: label})
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return result, errors.New("local config requires review")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return result, errors.New("local config unreadable")
		}
		result.Configs = append(result.Configs, ParseConnectionFingerprint(label, data))
	}
	for i, a := range result.Configs {
		for _, b := range result.Configs[i+1:] {
			if a.Complete && b.Complete && a.Fingerprint == b.Fingerprint {
				result.Warnings = append(result.Warnings, "IDENTICAL_LOCAL_CONFIG: "+a.Label+" and "+b.Label)
			}
		}
	}
	return result, nil
}
