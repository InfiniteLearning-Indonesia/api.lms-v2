//go:build qaage && !linux && !windows

package backup

import "testing"

func actualACLDirectory(t *testing.T, broad bool) string {
	t.Helper()
	t.Fatal("actual protection host unsupported; not skipped")
	return ""
}
