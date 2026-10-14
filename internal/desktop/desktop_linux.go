package desktop

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

func trayAvailable() error {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return errors.New("system tray unavailable: no desktop D-Bus session; using the browser")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return errors.New("system tray unavailable: cannot connect to desktop D-Bus; using the browser")
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var registered dbus.Variant
	err = conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").CallWithContext(ctx,
		"org.freedesktop.DBus.Properties.Get", 0, "org.kde.StatusNotifierWatcher", "IsStatusNotifierHostRegistered").Store(&registered)
	if err != nil || registered.Value() != true {
		return errors.New("system tray unavailable: desktop has no StatusNotifier host; using the browser")
	}
	return nil
}

func OpenBrowser(url string) error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errors.New("no graphical desktop session")
	}
	return openCommand("xdg-open", url)
}

func installReopen(func()) (func(), error) { return func() {}, nil }
func StartedFromDesktop() bool             { return os.Getenv("PKUDISK_DESKTOP_LAUNCH") == "1" }
func DetachConsole()                       {}
