//go:build qaage

package backup

import (
	"os"
	"testing"
)

func actualACLDirectory(t *testing.T, broad bool) string {
	t.Helper()
	path := t.TempDir()
	mode := os.FileMode(0700)
	if broad {
		mode = 0755
	}
	if os.Chmod(path, mode) != nil {
		t.Fatal("synthetic fixture mode unavailable")
	}
	return path
}
