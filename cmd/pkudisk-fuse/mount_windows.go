package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/thezzisu/pkudisk-fuse/internal/localfile"
	"github.com/thezzisu/pkudisk-fuse/internal/vfs"
	"github.com/winfsp/cgofuse/fuse"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const mountPlatformHelp = "Windows requires WinFsp (https://winfsp.dev/rel/). Choose an unused drive\nletter such as P:, or an empty local directory. Ctrl-C unmounts.\nFor a drive letter, --background returns after mounting without a console;\nuse 'unmount P:' to stop it. 'service install P:' enables mounting at boot."
const mountPlatformExample = "  pkudisk-fuse mount P:\n  pkudisk-fuse mount P: --account work --read-only\n  pkudisk-fuse mount P: --root '/My library/Reports'"
const mountPlatformPoint = "P:"

func checkMountSupport() error {
	dllName := "winfsp-x64.dll"
	if runtime.GOARCH == "arm64" {
		dllName = "winfsp-a64.dll"
	}
	for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WinFsp`, registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		dir, _, err := key.GetStringValue("InstallDir")
		key.Close()
		if err != nil {
			continue
		}
		dll, err := windows.LoadDLL(filepath.Join(dir, "bin", dllName))
		if err == nil {
			dll.Release()
			return nil
		}
	}
	return errors.New("mount requires WinFsp for this architecture; install it from https://winfsp.dev/rel/, then run mount again")
}

func windowsMountpoint(value string) (string, error) {
	if len(value) == 2 && value[1] == ':' && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) {
		value = strings.ToUpper(value)
		drives, err := windows.GetLogicalDrives()
		if err != nil {
			return "", err
		}
		if drives&(1<<uint(value[0]-'A')) != 0 {
			return "", fmt.Errorf("drive %s is already in use; choose an unused drive letter", value)
		}
		return value, nil
	}
	point, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(point)
	if errors.Is(err, os.ErrNotExist) {
		return point, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("mountpoint must be an unused drive letter or an empty directory")
	}
	entries, err := os.ReadDir(point)
	if err != nil {
		return "", err
	}
	if len(entries) > 0 {
		return "", errors.New("mountpoint directory must be empty")
	}
	return point, nil
}

func mountFileSystem(ctx context.Context, source *accountSource, opts mountOptions, output io.Writer) (result error) {
	point, err := windowsMountpoint(localfile.ExpandPath(opts.mountpoint))
	if err != nil {
		return err
	}
	core, err := vfs.New(source.backend, vfs.Options{Root: source.root, ReadOnly: opts.readOnly, CacheDir: localfile.ExpandPath(opts.cache), CacheLimit: opts.cacheLimit, Read: &opts.read, TTL: opts.ttl, IOTimeout: opts.timeout, Trace: opts.trace})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, core.Close()) }()
	// Keep the operation context alive until the driver finishes releasing its
	// handles, so Ctrl-C can unmount without cancelling an in-flight flush.
	operationCtx, stopOperations := context.WithCancel(context.Background())
	defer stopOperations()
	adapter := vfs.NewWinFsp(operationCtx, core)
	host := fuse.NewFileSystemHost(adapter)
	host.SetCapOpenTrunc(true)
	host.SetCapReaddirPlus(true)
	host.SetUseIno(true)
	arguments := vfs.WinFspMountOptions()
	if len(opts.driverOptions) != 0 {
		arguments = opts.driverOptions
	}
	arguments = append(arguments, "-o", fmt.Sprintf("fsname=PKUNetDisk,volname=PKU NetDisk,attr_timeout=%g,entry_timeout=%g", opts.ttl.Seconds(), opts.ttl.Seconds()))
	if opts.readOnly {
		arguments = append(arguments, "-o", "ro")
	}
	if opts.debug {
		arguments = append(arguments, "-d")
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("WinFsp mount failed: %v", recovered)
			}
		}()
		if !host.Mount(point, arguments) {
			done <- errors.New("WinFsp mount failed; check the driver and choose an unused drive letter or empty directory")
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-adapter.Ready:
	}
	if opts.ready != nil {
		// Init runs before WinFsp publishes its drive letter.
		for {
			drives, err := windows.GetLogicalDrives()
			if err != nil {
				result = err
				break
			}
			if drives&(1<<uint(point[0]-'A')) != 0 {
				if _, err := os.Stat(point + `\`); err == nil {
					result = opts.ready()
					break
				}
			}
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				result = ctx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
			break
		}
		if result != nil {
			if host.Unmount() {
				return errors.Join(result, <-done)
			}
			return errors.Join(result, errors.New("WinFsp unmount failed"))
		}
	}
	stopHelp := "Ctrl-C unmounts."
	if opts.workerHandles != "" {
		stopHelp = "Use 'pkudisk-fuse unmount " + point + "' to stop."
	} else if len(opts.driverOptions) != 0 {
		stopHelp = "Use 'pkudisk-fuse service stop' to stop."
	}
	fmt.Fprintf(output, "Mounted PKU NetDisk at %s; cache %s. %s\n", point, core.CachePath(), stopHelp)
	maintenanceFailed, stopMaintenance := source.maintain(ctx, opts.refreshInterval, output)
	defer stopMaintenance()
	select {
	case err := <-done:
		return err
	case result = <-maintenanceFailed:
	case <-ctx.Done():
	}
	opts.trace.Printf(ctx, "unmount requested; waiting for WinFsp to release handles")
	if !host.Unmount() {
		return errors.Join(result, errors.New("WinFsp unmount failed"))
	}
	return errors.Join(result, <-done)
}
