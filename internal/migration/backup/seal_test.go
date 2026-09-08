package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBackupSealing(t *testing.T) {
	now := time.Now().UTC()
	b := Binding{"run-safe", strings.Repeat("a", 64), strings.Repeat("b", 64), now}
	key := bytes.Repeat([]byte{17}, 32)
	plain := []byte("synthetic SQL only")
	sealed, err := Seal(plain, key, b, now)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Unseal(sealed, key, b, b.Target, now)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatal("roundtrip failed")
	}
	second, _ := Seal(plain, key, b, now)
	if bytes.Equal(sealed, second) {
		t.Fatal("nonce reused")
	}
	bad := append([]byte(nil), sealed...)
	bad[len(bad)-1] ^= 1
	changed := b
	changed.RunID = "other-run"
	for _, c := range []struct {
		data, key []byte
		b         Binding
		target    string
		now       time.Time
	}{
		{bad, key, b, b.Target, now}, {sealed, bytes.Repeat([]byte{18}, 32), b, b.Target, now}, {sealed, key, changed, b.Target, now},
		{sealed, key, b, strings.Repeat("c", 64), now}, {sealed, key, b, b.Target, now.Add(25 * time.Hour)}, {sealed, key, b, b.Target, now.Add(-time.Second)},
		{sealed[:3], key, b, b.Target, now},
	} {
		if _, err := Unseal(c.data, c.key, c.b, c.target, c.now); err == nil {
			t.Fatal("unsafe backup accepted")
		}
	}
	if _, err := Seal(nil, key, b, now); err == nil {
		t.Fatal("empty backup accepted")
	}
}

func TestBackupHostAndExclusiveWrite(t *testing.T) {
	root := t.TempDir()
	// testing.TempDir's child directory may inherit the process umask. Set
	// the private mode explicitly; the production guard must not be relaxed.
	if runtime.GOOS == "linux" {
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	free, err := AvailableBytes(root)
	if err != nil || free == 0 {
		t.Fatal("actual capacity unavailable")
	}
	if ProtectedDestination(root, ^uint64(0)) == nil {
		t.Fatal("capacity overflow accepted")
	}
	err = ProtectedDestination(root, 1)
	if runtime.GOOS == "linux" && err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && err == nil {
		t.Fatal("unverified Windows ACL accepted")
	}
	path := filepath.Join(root, "artifact.sealed")
	if err := WriteNew(path, []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if WriteNew(path, []byte("overwrite")) == nil {
		t.Fatal("overwrite accepted")
	}
	if runtime.GOOS == "linux" {
		if err := os.Chmod(root, 0755); err != nil {
			t.Fatal(err)
		}
		if ProtectedDestination(root, 1) == nil {
			t.Fatal("public directory accepted")
		}
	}
}
