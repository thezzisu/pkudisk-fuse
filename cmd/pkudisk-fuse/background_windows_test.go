package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thezzisu/pkudisk-fuse/internal/accounts"
	"github.com/thezzisu/pkudisk-fuse/internal/auth"
	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "mount" || os.Args[1] == "service") {
		err := execute(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(exitCode(err))
	}
	os.Exit(m.Run())
}

func TestWindowsMountHelpAndValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{{"service"}, {"service", "install"}, {"service", "start"}, {"service", "stop"}, {"service", "status"}, {"service", "remove"}, {"unmount"}} {
		out, stderr, err := testExecute(append(args, "--help")...)
		if err != nil || stderr != "" || !strings.Contains(out, "Usage:") {
			t.Fatalf("offline help for %v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"service", "install", "P:", "--password-stdin"},
		{"service", "install", "P:", "--cache-dir", "relative"},
		{"service", "install", "P:", "--background"},
		{"service", "install", "a-directory"},
		{"mount", "P:", "--background", "--password-stdin"},
	} {
		_, _, err := testExecute(args...)
		if exitCode(err) != 2 {
			t.Fatalf("invalid flags reached accounts/service manager: %v: %v", args, err)
		}
	}
	for _, point := range []string{"", "AB:", "1:", "P:\\", "P"} {
		if _, err := driveLetter(point); err == nil {
			t.Fatalf("accepted invalid drive %q", point)
		}
	}
	if point, err := driveLetter("p:"); err != nil || point != "P:" {
		t.Fatal("drive normalization failed")
	}
}

func TestInheritedMountHandlesAndCancellation(t *testing.T) {
	for _, value := range []string{"", "0,1,2", "-1,2,3", "1,2", "a,b,c", "18446744073709551616,1,2"} {
		if _, err := parseMountHandles(value); err == nil {
			t.Fatal("accepted invalid inherited handles")
		}
	}
	handle, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := waitMountHandles(ctx, []windows.Handle{handle}); err == nil {
		t.Fatal("unsignaled event ignored cancellation")
	}
	if err := windows.SetEvent(handle); err != nil {
		t.Fatal(err)
	}
	if err := waitMountHandles(context.Background(), []windows.Handle{handle}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeBackgroundMountLifecycle(t *testing.T) {
	if err := checkMountSupport(); err != nil {
		if os.Getenv("PKUDISK_FUSE_REQUIRE_KERNEL") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatal(err)
	}
	point := ""
	for letter := byte('Y'); letter >= 'Q'; letter-- {
		if drives&(1<<uint(letter-'A')) == 0 {
			point = string(letter) + ":"
			break
		}
	}
	if point == "" {
		t.Skip("no free test drive")
	}
	newMountTestAccount(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := startBackground(ctx, []string{"mount", point, "--non-interactive", "--read-only"}, point, &output); err != nil {
		log, _ := os.ReadFile(filepath.Join(defaultCache(), "background", point[:1]+".log"))
		t.Fatalf("%v; child log: %s", err, log)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopBackground(stop, point)
	})
	if !strings.Contains(output.String(), "Mounted in background") {
		t.Fatal("missing readiness result")
	}
	entries, err := os.ReadDir(point + `\`)
	if err != nil || len(entries) != 1 || entries[0].Name() != "test-library" {
		t.Fatalf("detached mount unusable: %v", err)
	}
	if err := startBackground(ctx, []string{"mount", point}, point, &bytes.Buffer{}); err == nil {
		t.Fatal("duplicate mount accepted")
	}
	if err := stopBackground(ctx, point); err != nil {
		t.Fatal(err)
	}
	drives, err = windows.GetLogicalDrives()
	if err != nil || drives&(1<<uint(point[0]-'A')) != 0 {
		t.Fatal("unmount returned before drive disappeared")
	}
	if _, _, err := testExecute("mount", point, "--background", "--all-accounts", "--read-only"); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(point + `\`)
	if err != nil || len(entries) != 1 || entries[0].Name() != "test" {
		t.Fatalf("background mount lost --all-accounts: %v", err)
	}
	entries, err = os.ReadDir(point + `\test`)
	if err != nil || len(entries) != 1 || entries[0].Name() != "test-library" {
		t.Fatalf("background account libraries unusable: %v", err)
	}
	if err := stopBackground(ctx, point); err != nil {
		t.Fatal(err)
	}
	if err := startBackground(ctx, []string{"mount", point, "--account=missing", "--non-interactive"}, point, &bytes.Buffer{}); err == nil {
		t.Fatal("startup authentication failure reported success")
	}
	log, err := os.ReadFile(filepath.Join(defaultCache(), "background", point[:1]+".log"))
	if err != nil || !strings.Contains(string(log), "saved account not found") {
		t.Fatal("startup error was not logged")
	}
	if strings.Contains(string(log), "private-fixture") || strings.Contains(string(log), "test-refresh") {
		t.Fatal("credential leaked into background log")
	}
}

func newMountTestAccount(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config with spaces & characters"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache with spaces & characters"))
	var refreshed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/efast/v1/quota/doc-lib/") {
			fmt.Fprint(w, `{"allocated":1048576,"used":0}`)
			return
		}
		switch r.URL.Path {
		case "/anyshare/oauth2/login/refreshToken":
			http.SetCookie(w, &http.Cookie{Name: "client.oauth2_token", Value: fmt.Sprintf("test-access-%d", refreshed.Add(1)), Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "client.oauth2_refresh_token", Value: "test-refresh", Path: "/"})
		case "/api/efast/v1/entry-doc-lib":
			fmt.Fprint(w, `[{"id":"gns://test","name":"test-library","size":-1,"type":"user_doc_lib"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	store, err := accounts.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(accounts.Account{Name: "test", Username: "test", Password: "private-fixture", Server: server.URL, Session: auth.State{AccessToken: "old-access", RefreshToken: "test-refresh"}}, true, false); err != nil {
		t.Fatal(err)
	}
}
