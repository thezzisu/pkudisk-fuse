package desktop

import (
	"errors"
	"os"
)

func trayAvailable() error {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return errors.New("system tray unavailable in an SSH session")
	}
	return nil
}

func OpenBrowser(url string) error { return openCommand("/usr/bin/open", url) }
func StartedFromDesktop() bool     { return os.Getenv("PKUDISK_DESKTOP_LAUNCH") == "1" }
func DetachConsole()               {}
