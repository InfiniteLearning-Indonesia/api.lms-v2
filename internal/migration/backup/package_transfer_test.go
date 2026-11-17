package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalTransferStrictInput(t *testing.T) {
	fixture := authDeclarationFixture(t)
	seal := SealPackageArtifactRequest{Version: 1, Package: fixture.Package}
	raw, _ := json.Marshal(seal)
	if _, err := DecodeSealPackageArtifact(strings.NewReader(string(raw))); err != nil {
		t.Fatal("structured seal request")
	}
	recover := RecoverPackageArtifactRequest{Version: 1, Authentication: fixture}
	raw, _ = json.Marshal(recover)
	if _, err := DecodeRecoverPackageArtifact(strings.NewReader(string(raw))); err != nil {
		t.Fatal("structured recover request")
	}
	for _, bad := range []string{"null", "{}", `{"version":1,"version":1}`, `{"version":1,"private_key":"PRIVATE_FIXTURE"}`, string(raw) + "{}"} {
		if _, err := DecodeSealPackageArtifact(strings.NewReader(bad)); err == nil {
			t.Fatal("ambiguous seal input")
		}
		if _, err := DecodeRecoverPackageArtifact(strings.NewReader(bad)); err == nil {
			t.Fatal("ambiguous recover input")
		}
	}
}

func TestLocalTransferFailsBeforeOutput(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	root := t.TempDir()
	auth := authDeclarationFixture(t)
	r := RecoverPackageArtifactRequest{Version: 1, Authentication: auth, Role: "db_custom", Directory: root, Name: "../escape.dump"}
	if p, err := RecoverPackageArtifact(ctx, r, time.Now().UTC()); err == nil || p.Status != "" {
		t.Fatal("escape recovered")
	}
	r.Name = "database.dump"
	if p, err := RecoverPackageArtifact(ctx, r, time.Now().UTC()); err == nil || p.Status != "" {
		t.Fatal("unverified package recovered")
	}
	if _, err := os.Stat(filepath.Join(root, r.Name)); !os.IsNotExist(err) {
		t.Fatal("refused recovery created output")
	}
	if p, err := SealPackageArtifact(ctx, SealPackageArtifactRequest{Version: 1, SourceName: "../source"}, time.Now().UTC()); err == nil || p.Status != "" {
		t.Fatal("unprotected source sealed")
	}
}
