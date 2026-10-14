//go:build !windows

package tui

import "os"

func openTerminal() (*os.File, *os.File, error) {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	return file, file, err
}
