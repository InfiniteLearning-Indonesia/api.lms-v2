//go:build qaage

package backup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func transferCLI(t *testing.T, flag string, value any) ([]byte, error) {
	t.Helper()
	raw, _ := json.Marshal(value)
	command := exec.Command(os.Getenv("QA_BASELINE_PACKAGE_BINARY"), flag)
	command.Env = ageEnvironment()
	command.Stdin = strings.NewReader(string(raw))
	return command.Output()
}

func TestAgeActualProtectedRecoveryAndCLI(t *testing.T) {
	ctx, r, now := actualAuthenticatedFixture(t)
	out := actualACLDirectory(t, false)
	v := RecoverPackageArtifactRequest{1, r, "db_custom", out, "database.dump"}
	p, err := RecoverPackageArtifact(ctx, v, now)
	if err != nil || p.Status != "OFFLINE_ARTIFACT_RECOVERED_NOT_AUTHORIZED" || !p.ArchiveAuthentication || !p.LocalProtectionVerified || p.ActualRestoreVerified || p.AuthorizesExecution || p.DatabaseAccess != "NONE" {
		t.Fatal("protected recovery failed/proof boundary wrong")
	}
	raw, err := os.ReadFile(filepath.Join(out, v.Name))
	if err != nil || !strings.HasPrefix(string(raw), "PGDMP") || Hash(raw) != p.Plain.SHA256 {
		t.Fatal("recovered payload wrong")
	}
	if q, err := RecoverPackageArtifact(ctx, v, now); err == nil || q.Status != "" {
		t.Fatal("recovery overwrite accepted")
	}
	after, _ := os.ReadFile(filepath.Join(out, v.Name))
	if Hash(after) != Hash(raw) {
		t.Fatal("existing recovered file changed")
	}
	v.Role = "db_sql"
	v.Name = "database.sql"
	body, err := transferCLI(t, "--recover-only", v)
	var cli RecoveredArtifactReport
	if err != nil || json.Unmarshal(body, &cli) != nil || cli.Status != p.Status || cli.AuthorizesExecution || cli.ActualRestoreVerified {
		t.Fatal("compiled recovery CLI failed")
	}
	// Re-seal recovered bytes via compiled producer, using only local files.
	_, _, _, recipient := actualAgeFixture(t)
	seal := SealPackageArtifactRequest{Version: 1, Package: r.Package, Tool: r.Tool, IdentityPath: r.IdentityPath, Role: "db_custom",
		SourceDirectory: out, SourceName: "database.dump", Plain: p.Plain, Recipient: recipient, CustodyRef: "synthetic-only"}
	// Recipient must correspond to the recovery identity; read its public comment.
	key, _ := os.ReadFile(r.IdentityPath)
	for _, line := range strings.Split(string(key), "\n") {
		if strings.HasPrefix(line, "# public key: ") {
			seal.Recipient = strings.TrimPrefix(line, "# public key: ")
		}
	}
	seal.Package.Intake.ArtifactRoot = actualACLDirectory(t, false)
	body, err = transferCLI(t, "--seal-only", seal)
	var produced ProtectedAgeReport
	if err != nil || json.Unmarshal(body, &produced) != nil || produced.Status != "PASS_PROTECTED_ENCRYPTED_FILE_VERIFIED_ONLY" || produced.Receipt.Plain != p.Plain || produced.AuthorizesExecution {
		t.Fatal("compiled local producer CLI failed")
	}
}

func TestAgeActualRecoveryRejectsCorruptionBeforeCreation(t *testing.T) {
	ctx, r, now := actualAuthenticatedFixture(t)
	out := actualACLDirectory(t, false)
	a := &r.Package.Artifacts[0]
	path := filepath.Join(r.Package.Intake.ArtifactRoot, a.Name)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 1 {
		t.Fatal("cipher fixture read")
	}
	raw[len(raw)-1] ^= 1
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("cipher fixture corruption")
	}
	a.SHA256 = Hash(raw)
	for i := range r.Artifacts {
		if r.Artifacts[i].Role == a.Role {
			r.Artifacts[i].Receipt.Cipher.SHA256 = a.SHA256
		}
	}
	v := RecoverPackageArtifactRequest{1, r, a.Role, out, "refused.dump"}
	if p, err := RecoverPackageArtifact(ctx, v, now); err == nil || p.Status != "" {
		t.Fatal("corrupt package recovered")
	}
	if _, err := os.Stat(filepath.Join(out, v.Name)); !os.IsNotExist(err) {
		t.Fatal("unauthenticated package created plaintext")
	}
}
