package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func enableServiceStoragePrivileges() (func(), error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return nil, err
	}
	var previous []windows.Tokenprivileges
	restore := func() {
		for i := len(previous) - 1; i >= 0; i-- {
			windows.AdjustTokenPrivileges(token, false, &previous[i], 0, nil, nil)
		}
		token.Close()
	}
	for _, privilege := range []string{"SeRestorePrivilege", "SeBackupPrivilege"} {
		name, _ := windows.UTF16PtrFromString(privilege)
		state := windows.Tokenprivileges{PrivilegeCount: 1}
		if err := windows.LookupPrivilegeValue(nil, name, &state.Privileges[0].Luid); err != nil {
			restore()
			return nil, err
		}
		state.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
		var old windows.Tokenprivileges
		var size uint32
		if err := windows.AdjustTokenPrivileges(token, false, &state, uint32(unsafe.Sizeof(old)), &old, &size); err != nil {
			restore()
			return nil, err
		}
		previous = append(previous, old)
	}
	return restore, nil
}

func checkServiceStorage(path string, expected *windows.SECURITY_DESCRIPTOR, directory bool) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return fmt.Errorf("service storage is a link or unexpected file type: %s", path)
	}
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := actual.Owner()
	if err != nil {
		return err
	}
	expectedOwner, _, err := expected.Owner()
	if err != nil {
		return err
	}
	if !windows.EqualSid(owner, expectedOwner) {
		return fmt.Errorf("unsafe service storage owner: %s", path)
	}
	acl, _, err := actual.DACL()
	if err != nil || acl == nil {
		return errors.New("service storage must have a restricted ACL")
	}
	allowed, _, err := expected.DACL()
	if err != nil || allowed == nil {
		return errors.New("invalid service storage ACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unexpected service storage ACL entry")
		}
		matched := false
		for j := uint32(0); j < uint32(allowed.AceCount); j++ {
			var want *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(allowed, j, &want); err != nil {
				return err
			}
			if windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), (*windows.SID)(unsafe.Pointer(&want.SidStart))) && ace.Mask&^want.Mask == 0 {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("unsafe service storage ACL: %s", path)
		}
	}
	runtime.KeepAlive(actual)
	runtime.KeepAlive(expected)
	return nil
}

func serviceDirectory(path string, sd *windows.SECURITY_DESCRIPTOR) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(name, &sa)
	runtime.KeepAlive(sd)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	return checkServiceStorage(path, sd, true)
}

func writeServiceFile(path string, data []byte, sd *windows.SECURITY_DESCRIPTOR) error {
	if _, err := os.Lstat(path); err == nil {
		if err := checkServiceStorage(path, sd, false); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
