package tui

import (
	"errors"
	"os"

	"golang.org/x/term"
)

func openTerminal() (*os.File, *os.File, error) {
	// Bubble Tea's Windows console reader requires the original stdin handle
	// to cancel an outstanding read. A fresh CONIN$ handle leaves a blocked
	// reader consuming input after a prompt has submitted.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, nil, errors.New("standard input is not a console")
	}
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stdin, os.Stderr, nil
	}
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	return os.Stdin, output, err
}
