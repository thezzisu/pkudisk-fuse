package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestNativeWindowsServiceLifecycle(t *testing.T) {
	require := os.Getenv("PKUDISK_FUSE_REQUIRE_SERVICE") == "1"
	skip := func(reason string) {
		t.Helper()
		if require {
			t.Fatal(reason)
		}
		t.Skip(reason)
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		skip("service test requires an administrator token")
	}
	if err := checkMountSupport(); err != nil {
		skip(err.Error())
	}
	manager, err := connectService(windows.SC_MANAGER_CONNECT)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Disconnect()
	existing, err := openMountService(manager, windows.SERVICE_QUERY_STATUS)
	if err == nil {
		existing.Close()
		skip("an existing mount service must not be modified by tests")
	}
	if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		t.Fatal(err)
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "pkudisk-fuse-service")); !errors.Is(err, os.ErrNotExist) {
		skip("existing service data must not be modified by tests")
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
		skip("no free service test drive")
	}
	newMountTestAccount(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// TestMain lets SCM run the copied test executable as the real CLI.
	if _, _, err := testExecute("service", "install", point, "--read-only"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		manageService(stop, "stop", io.Discard)
		manageService(stop, "remove", io.Discard)
	})
	service, err := openMountService(manager, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	config, err := service.Config()
	if err != nil || config.StartType != mgr.StartAutomatic || !config.DelayedAutoStart || config.ServiceStartName != `NT SERVICE\pkudisk-fuse` {
		t.Fatalf("wrong boot configuration: %v", err)
	}
	actions, err := service.RecoveryActions()
	if err != nil || len(actions) != 1 || actions[0].Type != mgr.ServiceRestart || actions[0].Delay != time.Minute {
		t.Fatal("service recovery was not registered", err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		want := "test-library"
		if repeat == 1 {
			if _, _, err := testExecute("service", "install", point, "--all-accounts", "--read-only"); err != nil {
				t.Fatal(err)
			}
			want = "test"
		}
		if err := manageService(ctx, "start", io.Discard); err != nil {
			t.Fatal(err)
		}
		status, err := service.Query()
		if err != nil || status.State != svc.Running || status.ProcessId == 0 {
			t.Fatal("service is not running", err)
		}
		entries, err := os.ReadDir(point + `\`)
		if err != nil || len(entries) != 1 || entries[0].Name() != want {
			t.Fatalf("virtual service account mount cannot be read: %v", err)
		}
		if err := manageService(ctx, "stop", io.Discard); err != nil {
			t.Fatal(err)
		}
		drives, err := windows.GetLogicalDrives()
		if err != nil || drives&(1<<uint(point[0]-'A')) != 0 {
			t.Fatal("service stop left a drive")
		}
	}
}
