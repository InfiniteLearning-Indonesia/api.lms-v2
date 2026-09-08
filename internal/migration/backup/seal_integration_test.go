//go:build qaintegration

package backup

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyntheticBackupProtection(t *testing.T) {
	root := os.Getenv("QA_LAB_DIR")
	if os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" || !SafeRoot(root) || !strings.Contains(filepath.Base(root), "-postgres-lab-") {
		t.Fatal("isolated lab required")
	}
	plain, err := os.ReadFile(filepath.Join(root, "pre_v3.dump"))
	if err != nil {
		t.Fatal("fixture dump required")
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err = WriteNew(filepath.Join(root, "backup-key.private"), key); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proofBytes, err := os.ReadFile(filepath.Join(root, "pre-v3-proof.json"))
	if err != nil {
		t.Fatal("run-bound proof required")
	}
	var proof struct {
		Target string `json:"target"`
	}
	if json.Unmarshal(proofBytes, &proof) != nil {
		t.Fatal("proof invalid")
	}
	binding := Binding{filepath.Base(root), proof.Target, Hash([]byte("synthetic-pre-v3")), now}
	sealed, err := Seal(plain, key, binding, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteNew(filepath.Join(root, "pre_v3.dump.sealed"), sealed); err != nil {
		t.Fatal(err)
	}
	// Reopen key and ciphertext from retained files; not just an in-memory test.
	recoveredKey, err := os.ReadFile(filepath.Join(root, "backup-key.private"))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(root, "pre_v3.dump.sealed"))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := Unseal(ciphertext, recoveredKey, binding, binding.Target, now)
	if err != nil || !bytes.Equal(recovered, plain) {
		t.Fatal("disk recovery failed")
	}
	if err = WriteNew(filepath.Join(root, "recovered.dump"), recovered); err != nil {
		t.Fatal(err)
	}
	free, err := AvailableBytes(root)
	if err != nil || free < uint64(len(plain))*4 {
		t.Fatal("actual capacity insufficient")
	}
	protection := "VERIFIED_LOCAL_OS_CHECK"
	if ProtectedDestination(root, uint64(len(plain))*4) != nil {
		protection = "UNVERIFIED_REAL_EXPORT_BLOCKED"
	}
	receipt, _ := json.Marshal(map[string]any{"scope": "SYNTHETIC_ONLY", "cipher_sha256": Hash(sealed), "plain_sha256": Hash(plain), "binding": binding, "available_bytes": free, "host_protection": protection, "independent_key_custody": "NOT_VERIFIED", "shared_export": "BLOCKED"})
	if err = WriteNew(filepath.Join(root, "protection-receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
}
