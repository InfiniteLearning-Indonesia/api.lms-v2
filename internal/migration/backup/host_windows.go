package backup

import (
	"os"
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
func currentPrivateSID() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", errProtection
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", errProtection
	}
	return user.User.Sid.String()
}

func privateHandle(file *os.File) error {
	current, err := currentPrivateSID()
	if err != nil {
		return errProtection
	}
	api := syscall.NewLazyDLL("advapi32.dll")
	var descriptor, owner, dacl unsafe.Pointer
	code, _, _ := api.NewProc("GetSecurityInfo").Call(file.Fd(), 1, 5,
		uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 || descriptor == nil {
		return errProtection
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	if owner == nil || dacl == nil {
		return errProtection
	}
	valid, _, _ := api.NewProc("IsValidAcl").Call(uintptr(dacl))
	if valid == 0 {
		return errProtection
	}
	ownerName, err := (*syscall.SID)(owner).String()
	if err != nil {
		return errProtection
	}
	acl := (*struct {
		Revision, Reserved     byte
		Size, Count, Reserved2 uint16
	})(dacl)
	if acl.Count == 0 || acl.Count > 64 {
		return errProtection
	}
	grants := make([]privateGrant, 0, acl.Count)
	for i := 0; i < int(acl.Count); i++ {
		var ace unsafe.Pointer
		ok, _, _ := api.NewProc("GetAce").Call(uintptr(dacl), uintptr(i), uintptr(unsafe.Pointer(&ace)))
		if ok == 0 || ace == nil {
			return errProtection
		}
		header := (*struct {
			Kind, Flags byte
			Size        uint16
			Mask        uint32
		})(ace)
		if header.Kind != 0 || header.Size < 16 {
			return errProtection
		}
		sid, err := (*syscall.SID)(unsafe.Add(ace, 8)).String()
		if err != nil {
			return errProtection
		}
		grants = append(grants, privateGrant{sid, header.Kind, header.Flags, header.Mask})
	}
	if !privateACL(ownerName, current, grants) {
		return errProtection
	}
	return nil
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

func singleLink(file *os.File) bool {
	var info syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info) == nil && info.NumberOfLinks == 1
}
