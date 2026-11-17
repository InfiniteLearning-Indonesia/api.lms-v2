package backup

import "syscall"

func intakeReparse(path string) bool {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := syscall.GetFileAttributes(ptr)
	return err != nil || attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
