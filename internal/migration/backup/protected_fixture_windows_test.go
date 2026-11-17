//go:build qaage

package backup

import (
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

// Only newly-created synthetic fixtures receive an explicit DACL. This never
// calls SetSecurityInfo, changes an existing ACL/owner, or enables privileges.
func actualACLDirectory(t *testing.T, broad bool) string {
	t.Helper()
	sid, err := currentPrivateSID()
	if err != nil {
		t.Fatal("fixture user unavailable")
	}
	sddl := "O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)"
	if broad {
		sddl += "(A;OICI;FR;;;WD)"
	}
	text, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		t.Fatal("fixture descriptor input")
	}
	var descriptor unsafe.Pointer
	ok, _, _ := syscall.NewLazyDLL("advapi32.dll").NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW").Call(
		uintptr(unsafe.Pointer(text)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 || descriptor == nil {
		t.Fatal("fixture descriptor unavailable")
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	attributes := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: uintptr(descriptor)}
	path := filepath.Join(t.TempDir(), "private")
	ptr, _ := syscall.UTF16PtrFromString(path)
	if syscall.CreateDirectory(ptr, &attributes) != nil {
		t.Fatal("new private fixture directory unavailable")
	}
	return path
}
