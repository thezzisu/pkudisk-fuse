package desktop

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

type testWatcher struct{ registrations chan string }

func (w *testWatcher) RegisterStatusNotifierItem(service string) *dbus.Error {
	w.registrations <- service
	return nil
}

func TestTrayWithoutDesktop(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	tray, err := New(func() { t.Error("unexpected activation") }, func() {})
	if err == nil || tray != nil {
		t.Fatal("missing desktop should select browser fallback")
	}
}

// Exercise the real systray backend on a private bus, never the user's desktop.
// The fake watcher implements the desktop's SNI host protocol; icon/menu
// export, activation callbacks and destruction use the actual library.
func TestLinuxTrayDBusLifecycle(t *testing.T) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is required for native D-Bus tray validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	config := filepath.Join(dir, "session.conf")
	data := `<busconfig><type>session</type><listen>unix:tmpdir=/tmp</listen><auth>EXTERNAL</auth><policy context="default"><allow send_destination="*"/><allow receive_sender="*"/><allow own="*"/></policy></busconfig>`
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, daemon, "--config-file="+config, "--nofork", "--print-address=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	address, err := bufio.NewReader(pipe).ReadString('\n')
	if err != nil {
		_ = cmd.Wait()
		t.Fatalf("start private D-Bus daemon: %v: %s", err, stderr.String())
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(address))
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if _, err := bus.RequestName("org.kde.StatusNotifierWatcher", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	watcher := &testWatcher{registrations: make(chan string, 4)}
	if err := bus.Export(watcher, "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher"); err != nil {
		t.Fatal(err)
	}
	_, err = prop.Export(bus, "/StatusNotifierWatcher", prop.Map{
		"org.kde.StatusNotifierWatcher": {"IsStatusNotifierHostRegistered": {Value: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, quit := make(chan struct{}, 1), make(chan struct{}, 1)
	tray, err := New(func() { opened <- struct{}{} }, func() { quit <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer tray.Close()
	finished := make(chan error, 1)
	go func() { finished <- tray.Run() }()
	var service string
	select {
	case service = <-watcher.registrations:
	case <-ctx.Done():
		t.Fatal("tray did not register")
	}
	item := bus.Object(service, "/StatusNotifierItem")
	status, err := item.GetProperty("org.kde.StatusNotifierItem.Status")
	if err != nil || status.Value() != "Active" {
		t.Fatal("tray was not shown", err)
	}
	if err := item.CallWithContext(ctx, "org.kde.StatusNotifierItem.Activate", 0, int32(0), int32(0)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("tray activation was not delivered")
	}
	var revision uint32
	var layout struct {
		ID         int32
		Properties map[string]dbus.Variant
		Children   []dbus.Variant
	}
	menu := bus.Object(service, "/MenuBar")
	if err := menu.CallWithContext(ctx, "com.canonical.dbusmenu.GetLayout", 0, int32(0), int32(-1), []string{}).Store(&revision, &layout); err != nil {
		t.Fatal(err)
	}
	var exitID int32
	for _, child := range layout.Children {
		var entry struct {
			ID         int32
			Properties map[string]dbus.Variant
			Children   []dbus.Variant
		}
		if err := dbus.Store([]any{child.Value()}, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Properties["label"].Value() == "Exit" {
			exitID = entry.ID
		}
	}
	if exitID == 0 {
		t.Fatal("tray menu has no Exit item")
	}
	if err := menu.CallWithContext(ctx, "com.canonical.dbusmenu.Event", 0, exitID, "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-quit:
	case <-ctx.Done():
		t.Fatal("tray Exit was not delivered")
	}
	tray.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("tray loop did not exit")
	}
}
