//go:build !windows

package backup

import "os"

func intakeReparse(path string) bool {
	info, err := os.Lstat(path)
	return err != nil || info.Mode()&os.ModeSymlink != 0
}
