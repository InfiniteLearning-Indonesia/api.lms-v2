package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalStoreQuarantinePromotionAndIntegrity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	store, err := NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("private evidence\n")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := strings.Repeat("a", 64)
	if err = store.PutQuarantine(key, digest, data); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Read(key, digest); err == nil {
		t.Fatal("quarantined bytes exposed")
	}
	if err = store.Promote(key, digest); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(key, digest)
	if err != nil || string(read) != string(data) {
		t.Fatal("private object mismatch", err)
	}
	if err = store.PutQuarantine(key, digest, data); err != nil {
		t.Fatal("private object replay failed", err)
	}
	info, err := os.Stat(filepath.Join(root, "objects", key[:2], key))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		t.Fatal("object is not private", err)
	}
}

func TestUploadValidationAndScannerFailClosed(t *testing.T) {
	valid := Upload{ClassID: "class", OwnerUserID: "user", Purpose: "evidence", OriginalName: "proof.txt", DeclaredType: "text/plain", Data: []byte("proof\n")}
	normalized, detected, err := normalizeUpload(valid)
	if err != nil || normalized.Purpose != "EVIDENCE" || detected != "text/plain" {
		t.Fatal("valid upload rejected", err, detected)
	}
	for _, invalid := range []Upload{
		{ClassID: "class", OwnerUserID: "user", Purpose: "MATERIAL", OriginalName: "../proof.txt", DeclaredType: "text/plain", Data: []byte("proof")},
		{ClassID: "class", OwnerUserID: "user", Purpose: "MATERIAL", OriginalName: "proof.html", DeclaredType: "text/plain", Data: []byte("<html><script>alert(1)</script></html>")},
		{ClassID: "class", OwnerUserID: "user", Purpose: "MATERIAL", OriginalName: "proof.txt", DeclaredType: "text/plain", Data: make([]byte, MaxBytes+1)},
	} {
		if _, _, err = normalizeUpload(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe upload accepted", err)
		}
	}
	if err = (FixtureScanner{}).Scan(context.Background(), []byte("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR")); !errors.Is(err, ErrThreat) {
		t.Fatal("threat signature accepted", err)
	}
	if err = (FixtureScanner{}).Scan(context.Background(), []byte("SCAN_UNAVAILABLE fixture")); !errors.Is(err, ErrScanUnavailable) {
		t.Fatal("scanner failure was not fail-closed", err)
	}
}
