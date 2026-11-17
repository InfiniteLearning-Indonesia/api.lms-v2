package backup

import (
	"errors"
	"os"
	"syscall"
)

func AvailableBytes(path string) (uint64, error) {
	var s syscall.Statfs_t
	err := syscall.Statfs(path, &s)
	return s.Bavail * uint64(s.Bsize), err
}
func privateDirectory(path string) error {
	if !intakeRoot(path) {
		return errProtection
	}
	f, err := os.Open(path)
	if err != nil {
		return errProtection
	}
	defer f.Close()
	return privateHandle(f)
}
func privateHandle(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return errProtection
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return errors.New("BACKUP_HOST_PROTECTION_UNVERIFIED")
	}
	return nil
}
func singleLink(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
