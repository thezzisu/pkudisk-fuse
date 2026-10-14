//go:build !windows

package gui

func localDrives() []localRoot {
	return []localRoot{{Kind: "filesystem", Name: "/", Path: "/", Available: true}}
}
