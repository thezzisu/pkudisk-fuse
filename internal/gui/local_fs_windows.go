package gui

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func localDrives() []localRoot {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	roots := make([]localRoot, 0, 26)
	for drive := byte('A'); drive <= 'Z'; drive++ {
		occupied := mask&(1<<(drive-'A')) != 0
		roots = append(roots, localRoot{Kind: "drive", Name: fmt.Sprintf("%c:", drive),
			Path: fmt.Sprintf("%c:\\", drive), Available: occupied, Free: !occupied})
	}
	return roots
}
