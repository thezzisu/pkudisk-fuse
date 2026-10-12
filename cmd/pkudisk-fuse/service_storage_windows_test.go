package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestServiceStorageRejectsPublicACLAndLinks(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "private")
	if err := serviceDirectory(directory, sd); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "config.json")
	if err := writeServiceFile(file, []byte("{}"), sd); err != nil {
		t.Fatal(err)
	}
	if err := checkServiceStorage(file, sd, false); err != nil {
		t.Fatal(err)
	}
	public, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := public.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err := writeServiceFile(file, []byte("overwritten"), sd); err == nil {
		t.Fatal("unsafe file permissions accepted")
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "{}" {
		t.Fatal("rejected file was modified")
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(file, link); err == nil {
		if err := writeServiceFile(link, []byte("overwritten"), sd); err == nil {
			t.Fatal("service storage followed a link")
		}
	}
}
