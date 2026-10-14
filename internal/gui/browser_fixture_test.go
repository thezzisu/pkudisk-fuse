package gui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
)

// This helper exists only in the test executable. It never adds a mock switch
// or unauthenticated fixture endpoints to the distributed application.
func TestGUIBrowserFixtureProcess(t *testing.T) {
	capture := os.Getenv("PKUDISK_GUI_BROWSER_CAPTURE")
	if capture == "" {
		return
	}
	f := newBusinessFixture(t)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	info := map[string]string{"directory": filepath.Join(f.root, "gui"), "downloads": f.root, "upload": filepath.Join(f.root, "upload.txt")}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(capture+".info", data, 0600); err != nil {
		t.Fatal(err)
	}
	err := Run(ctx, Options{Directory: info["directory"], Version: "GUI test", Output: io.Discard,
		NewApplication: func(ctx context.Context) (*Application, error) { return NewApplication(ctx, f.options()), nil },
		NewTray:        func(func(), func()) (Tray, error) { return nil, errors.New("test browser session") },
		OpenBrowser:    func(address string) error { return os.WriteFile(capture, []byte(address), 0600) },
	})
	if err != nil {
		t.Fatal(err)
	}
}
