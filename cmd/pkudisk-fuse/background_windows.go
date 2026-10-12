package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/thezzisu/pkudisk-fuse/internal/localfile"
	"golang.org/x/sys/windows"
)

func addPlatformMountFlags(cmd *cobra.Command, opts *mountOptions) {
	cmd.Flags().BoolVar(&opts.background, "background", false, "mount an unused drive letter without a console; use unmount to stop")
	cmd.Flags().StringVar(&opts.workerHandles, "mount-handles", "", "inherited background control handles")
	cmd.Flags().MarkHidden("mount-handles")
}

func platformMountCommands() []*cobra.Command {
	return []*cobra.Command{newServiceCommand(), {
		Use: "unmount DRIVE:", Short: "Stop a background Windows mount and wait for it to exit", GroupID: "local",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return stopBackground(cmd.Context(), args[0]) },
	}}
}

func driveLetter(point string) (string, error) {
	if len(point) != 2 || point[1] != ':' || !strings.ContainsAny(point[:1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", errors.New("background mounts and services require a drive letter such as P:")
	}
	return strings.ToUpper(point), nil
}

func runPlatformMount(cmd *cobra.Command, credentials authFlags, opts mountOptions) error {
	if cmd.Name() == "install" {
		return installService(cmd, opts)
	}
	if opts.background {
		if opts.workerHandles != "" || credentials.stdin {
			return usageFailure(cmd, "--background cannot be combined with --password-stdin or --mount-handles; save an account first")
		}
		args := mountArguments(cmd, opts.mountpoint)
		return startBackground(cmd.Context(), args, opts.mountpoint, cmd.OutOrStdout())
	}
	if opts.workerHandles != "" {
		return runBackgroundWorker(cmd, credentials, opts)
	}
	if service, ok := cmd.Context().Value(serviceContextKey{}).(serviceConfig); ok {
		// Virtual account SIDs need an explicit POSIX mapping in WinFsp.
		opts.driverOptions = []string{"-o", "uid=4096,gid=4096,uidmap=4096:" + service.OwnerSID,
			"-o", "FileSecurity=D:P(A;;FA;;;" + service.OwnerSID + ")(A;;FA;;;SY)(A;;FA;;;" + service.ServiceSID + ")"}
		opts.ready = service.ready
		// WinFsp connects to SCM after authentication; leave time for that handshake.
		startup, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		source, err := prepareAccounts(startup, credentials, accountOptions{allAccounts: opts.allAccounts, root: opts.root, partSize: opts.partSize, timeout: opts.timeout, trace: opts.trace})
		cancel()
		if err != nil {
			return err
		}
		defer source.Close()
		return mountFileSystem(cmd.Context(), source, opts, cmd.ErrOrStderr())
	}
	return runMount(cmd, credentials, opts)
}

func mountArguments(cmd *cobra.Command, point string) []string {
	args := []string{"mount", point, "--non-interactive"}
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "background", "mount-handles", "non-interactive":
		default:
			args = append(args, "--"+flag.Name+"="+flag.Value.String())
		}
	})
	return args
}

func backgroundEventName(point, suffix string) (string, error) {
	point, err := driveLetter(point)
	if err != nil {
		return "", err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return `Local\pkudisk-fuse-` + user.User.Sid.String() + "-" + point[:1] + "-" + suffix, nil
}

func createMountEvent(point, suffix string) (windows.Handle, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return 0, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)")
	if err != nil {
		return 0, err
	}
	name, err := backgroundEventName(point, suffix)
	if err != nil {
		return 0, err
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd, InheritHandle: 1}
	handle, err := windows.CreateEvent(&sa, 1, 0, namePtr)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return 0, errors.New("a background mount already owns this drive; use unmount first")
		}
		return 0, err
	}
	return handle, nil
}

func startBackground(ctx context.Context, args []string, point string, output io.Writer) error {
	point, err := driveLetter(point)
	if err != nil {
		return err
	}
	if _, err := windowsMountpoint(point); err != nil {
		return err
	}
	var events []windows.Handle
	defer func() {
		for _, handle := range events {
			windows.CloseHandle(handle)
		}
	}()
	for _, name := range []string{"stop", "ready", "done"} {
		handle, err := createMountEvent(point, name)
		if err != nil {
			return err
		}
		events = append(events, handle)
	}
	if err := localfile.EnsurePrivateDir(defaultCache()); err != nil {
		return err
	}
	dir := filepath.Join(defaultCache(), "background")
	if err := localfile.EnsurePrivateDir(dir); err != nil {
		return err
	}
	log, err := localfile.OpenPrivate(filepath.Join(dir, point[:1]+".log"), os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer log.Close()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args = append(args, fmt.Sprintf("--mount-handles=%d,%d,%d", events[0], events[1], events[2]))
	child := exec.Command(executable, args...)
	child.Stdout, child.Stderr = log, log
	inherited := make([]syscall.Handle, len(events))
	for i, handle := range events {
		inherited[i] = syscall.Handle(handle)
	}
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, AdditionalInheritedHandles: inherited}
	if err := child.Start(); err != nil {
		return err
	}
	defer child.Process.Release()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(child.Process.Pid))
	if err != nil {
		windows.SetEvent(events[0])
		return err
	}
	defer windows.CloseHandle(process)
	startup, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	err = waitMountHandles(startup, []windows.Handle{events[1], process})
	if err != nil {
		windows.SetEvent(events[0])
		return fmt.Errorf("background mount failed: %w; see %s", err, log.Name())
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil {
		return err
	}
	if code != 259 { // STILL_ACTIVE
		return fmt.Errorf("background mount exited with code %d; see %s", code, log.Name())
	}
	fmt.Fprintf(output, "Mounted in background at %s (PID %d); log %s. Use 'pkudisk-fuse unmount %s' to stop.\n", point, child.Process.Pid, log.Name(), point)
	return nil
}

func waitMountHandles(ctx context.Context, handles []windows.Handle) error {
	for {
		result, err := windows.WaitForMultipleObjects(handles, false, 100)
		if err != nil {
			return err
		}
		if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		if result == windows.WAIT_OBJECT_0+1 {
			return errors.New("mount process exited before becoming ready")
		}
		if result != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("unexpected Windows wait result %d", result)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func parseMountHandles(value string) ([]windows.Handle, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return nil, errors.New("expected three inherited mount handles")
	}
	handles := make([]windows.Handle, 3)
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, strconv.IntSize)
		if err != nil || value == 0 {
			return nil, errors.New("invalid inherited mount handle")
		}
		handles[i] = windows.Handle(value)
		if _, err := windows.WaitForSingleObject(handles[i], 0); err != nil {
			return nil, err
		}
	}
	return handles, nil
}

func runBackgroundWorker(cmd *cobra.Command, credentials authFlags, opts mountOptions) error {
	handles, err := parseMountHandles(opts.workerHandles)
	if err != nil {
		return err
	}
	defer func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	}()
	defer windows.SetEvent(handles[2])
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if waitMountHandles(ctx, handles[:1]) == nil {
			cancel()
		}
	}()
	defer func() { cancel(); <-stopped }()
	cmd.SetContext(ctx)
	credentials.nonInteractive = true
	opts.ready = func() error { return windows.SetEvent(handles[1]) }
	return runMount(cmd, credentials, opts)
}

func stopBackground(ctx context.Context, point string) error {
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	}()
	for _, suffix := range []string{"stop", "done"} {
		name, err := backgroundEventName(point, suffix)
		if err != nil {
			return err
		}
		namePtr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		handle, err := windows.OpenEvent(windows.SYNCHRONIZE|windows.EVENT_MODIFY_STATE, false, namePtr)
		if err != nil {
			return fmt.Errorf("no background mount for %s: %w", point, err)
		}
		handles = append(handles, handle)
	}
	if err := windows.SetEvent(handles[0]); err != nil {
		return err
	}
	return waitMountHandles(ctx, handles[1:])
}
