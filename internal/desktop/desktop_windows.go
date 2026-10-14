package desktop

import (
	"errors"

	"github.com/inconshreveable/mousetrap"
	"golang.org/x/sys/windows"
)

func trayAvailable() error                 { return nil }
func installReopen(func()) (func(), error) { return func() {}, nil }
func StartedFromDesktop() bool             { return mousetrap.StartedByExplorer() }

// FreeConsole detaches only this process; unlike ShowWindow(SW_HIDE), it
// cannot hide a terminal window shared with the invoking shell.
func DetachConsole() {
	_, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole").Call()
}

func OpenBrowser(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	address, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return errors.New("invalid browser address")
	}
	if err := windows.ShellExecute(0, verb, address, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return errors.New("default browser launch failed")
	}
	return nil
}
