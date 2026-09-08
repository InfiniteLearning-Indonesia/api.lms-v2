package baseline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// All mutations below are ordinary temporary source fixtures, never a DB or
// any user checkout. The production collector itself remains read-only.
func fixtureRepository(t *testing.T, workspace, label string) string {
	t.Helper()
	root := filepath.Join(workspace, label)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "sample.ts"), []byte("@Entity('sample')\n@Get('')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--initial-branch=fixture"}, {"add", "src/sample.ts"}, {"-c", "user.name=Synthetic QA", "-c", "user.email=qa@example.invalid", "commit", "-m", "synthetic source fixture"}} {
		base := []string{"-c", "core.excludesFile=", "-c", "safe.directory=" + filepath.ToSlash(root), "-C", root}
		command := exec.Command("git", append(base, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("synthetic git fixture failed: %v %s", err, output)
		}
	}
	return root
}

func TestCollectLocalSourcesWithoutDatabaseClaims(t *testing.T) {
	workspace := t.TempDir()
	live := fixtureRepository(t, workspace, "backend-live")
	staging := fixtureRepository(t, workspace, "backend")
	config := []byte("DB_HOST=synthetic.invalid\nDB_PORT=5432\nDB_USER=qa\nDB_NAME=fixture\nDB_PASSWORD=DO_NOT_EMIT\n")
	for _, path := range []string{filepath.Join(live, ".env"), filepath.Join(staging, ".env.staging")} {
		if err := os.WriteFile(path, config, 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Collect(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Repositories) != 2 || len(report.Configs) != 4 || report.Configs[2].Present || report.Configs[3].Present {
		t.Fatal("missing config must remain explicitly absent")
	}
	for _, repo := range report.Repositories {
		if repo.Files != 1 || repo.DeployedCommit != "UNKNOWN" || len(repo.Commit) != 40 || len(repo.Declarations) != 2 {
			t.Fatal("incomplete source inventory")
		}
	}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "DO_NOT_EMIT") || strings.Contains(string(data), "synthetic.invalid") || report.Configs[0].VerifiedDatabase {
		t.Fatal("secret/raw identity leaked or database verification fabricated")
	}
	if !strings.Contains(strings.Join(report.Warnings, "\n"), "IDENTICAL_LOCAL_CONFIG") {
		t.Fatal("identical local config warning absent")
	}
	originalHash := report.Repositories[1].SourceHash
	if err := os.WriteFile(filepath.Join(staging, "src", "sample.ts"), []byte("@Post('changed')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := InventoryRepository(context.Background(), staging, "backend")
	if err != nil || !changed.Dirty || changed.SourceHash == originalHash {
		t.Fatal("working-tree changes not detected")
	}
}

func TestInventoryRejectsMissingGitSourceAndUnsafeConfig(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Collect(context.Background(), workspace); err == nil {
		t.Fatal("missing checkout accepted")
	}
	live := fixtureRepository(t, workspace, "backend-live")
	fixtureRepository(t, workspace, "backend")
	if err := os.Mkdir(filepath.Join(live, ".env"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(context.Background(), workspace); err == nil {
		t.Fatal("non-regular config accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InventoryRepository(ctx, live, "backend-live"); err == nil {
		t.Fatal("cancelled inventory accepted")
	}
}
