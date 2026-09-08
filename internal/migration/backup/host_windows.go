package backup

import (
	"errors"
	"syscall"
	"unsafe"
)

func AvailableBytes(path string) (uint64, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if result == 0 {
		return 0, callErr
	}
	return free, nil
}
func privateDirectory(string) error { return errors.New("BACKUP_WINDOWS_ACL_REVIEW_REQUIRED") }
