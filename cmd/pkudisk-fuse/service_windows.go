package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/thezzisu/pkudisk-fuse/internal/accounts"
	"github.com/thezzisu/pkudisk-fuse/internal/localfile"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "pkudisk-fuse"

type serviceContextKey struct{}
type serviceConfig struct {
	Args       []string `json:"args"`
	OwnerSID   string   `json:"owner_sid"`
	ServiceSID string   `json:"service_sid"`
	ready      func() error
}

func newServiceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Manage a native Windows service that mounts at boot", Args: cobra.NoArgs,
		Long: "Install requires an administrator terminal and a saved account. It copies\nthe binary and selected accounts into protected storage. A dedicated Windows\nvirtual account runs the mount before logon without a Windows password.\nThe installing user and SYSTEM can access the mounted files.\nStop the service before removing it; removal preserves accounts and recovery\nfiles. Reinstall while stopped to update the binary and account selection.",
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	install := newRemoteCommand("mount")
	install.Use, install.Short = "install DRIVE:", "Install or update automatic boot mounting (administrator required)"
	install.Long = "Install or update the service using the saved default account, --account, or\n--all-accounts. Mount tuning flags apply. Explicit credentials, --background,\nand --cache-dir are unavailable: service credentials and cache are isolated.\nThe service is not started by install; run 'service start' after installation."
	install.Example = "  pkudisk-fuse service install P:\n  pkudisk-fuse service install P: --account work --read-only"
	cmd.AddCommand(install)
	for _, action := range []string{"start", "stop", "status", "remove"} {
		cmd.AddCommand(&cobra.Command{Use: action, Short: action + " the Windows mount service", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return manageService(cmd.Context(), cmd.Name(), cmd.OutOrStdout())
			},
		})
	}
	var directory string
	run := &cobra.Command{Use: "run", Hidden: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runService(cmd.Context(), directory) },
	}
	run.Flags().StringVar(&directory, "directory", "", "private service data directory")
	cmd.AddCommand(run)
	return cmd
}

func connectService(access uint32) (*mgr.Mgr, error) {
	handle, err := windows.OpenSCManager(nil, nil, access)
	if err != nil {
		return nil, fmt.Errorf("open Windows service manager (install/remove require an administrator terminal): %w", err)
	}
	return &mgr.Mgr{Handle: handle}, nil
}

func openMountService(manager *mgr.Mgr, access uint32) (*mgr.Service, error) {
	name, _ := windows.UTF16PtrFromString(serviceName)
	handle, err := windows.OpenService(manager.Handle, name, access)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: serviceName, Handle: handle}, nil
}

func installService(cmd *cobra.Command, opts mountOptions) (result error) {
	point, err := driveLetter(opts.mountpoint)
	if err != nil {
		return usageFailure(cmd, err.Error())
	}
	for _, flag := range []string{"username", "server", "password-file", "credentials-file", "password-stdin", "no-save", "cache-dir", "background", "mount-handles"} {
		if cmd.Flags().Changed(flag) {
			return usageFailure(cmd, "service install cannot use --"+flag+"; save an account first")
		}
	}
	if _, err := windowsMountpoint(point); err != nil {
		return err
	}
	store, err := accounts.Default()
	if err != nil {
		return err
	}
	config, err := store.Load()
	if err != nil {
		return err
	}
	if !opts.allAccounts {
		name, _ := cmd.Flags().GetString("account")
		if name == "" {
			name = config.Default
		}
		account, ok := config.Find(name)
		if !ok {
			return errors.New("no saved account selected; run account add first")
		}
		config.Default, config.Accounts = name, []accounts.Account{account}
	}
	if len(config.Accounts) == 0 {
		return errors.New("no saved accounts; run account add first")
	}
	manager, err := connectService(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return err
	}
	root = filepath.Join(root, "pkudisk-fuse-service")
	directory := filepath.Join(root, "data")
	binary := filepath.Join(root, "pkudisk-fuse.exe")
	service, err := openMountService(manager, windows.SERVICE_ALL_ACCESS)
	created := false
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		service, err = manager.CreateService(serviceName, binary, mgr.Config{
			DisplayName: "PKU NetDisk mount", Description: "Mount PKU NetDisk using WinFsp",
			StartType: mgr.StartAutomatic, DelayedAutoStart: true, ErrorControl: mgr.ErrorNormal,
			Dependencies: []string{"WinFsp.Launcher"}, ServiceStartName: `NT SERVICE\` + serviceName,
		}, "service", "run", "--directory", directory)
		created = err == nil
	}
	if err != nil {
		return err
	}
	defer service.Close()
	defer func() {
		if result != nil && created {
			service.Delete()
		}
	}()
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		return errors.New("stop the existing service before updating it")
	}
	registered, err := service.Config()
	if err != nil {
		return err
	}
	if registered.ServiceStartName != `NT SERVICE\`+serviceName {
		return errors.New("existing service uses another Windows account; remove it explicitly before installing")
	}
	registered.StartType, registered.DelayedAutoStart = mgr.StartAutomatic, true
	registered.Dependencies = []string{"WinFsp.Launcher"}
	registered.BinaryPathName = windows.ComposeCommandLine([]string{binary, "service", "run", "--directory", directory})
	if err := service.UpdateConfig(registered); err != nil {
		return err
	}
	serviceSID, _, _, err := windows.LookupSID("", `NT SERVICE\`+serviceName)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	restore, err := enableServiceStoragePrivileges()
	if err != nil {
		return err
	}
	defer restore()
	programSD, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;BA)(A;OICI;FA;;;SY)(A;OICI;FRFX;;;" + serviceSID.String() + ")")
	if err != nil {
		return err
	}
	dataSD, err := windows.SecurityDescriptorFromString("O:" + serviceSID.String() + "D:P(A;OICI;FA;;;" + serviceSID.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	if err := serviceDirectory(root, programSD); err != nil {
		return err
	}
	for _, dir := range []string{directory, filepath.Join(directory, "config"), filepath.Join(directory, "config", "pkudisk-fuse"), filepath.Join(directory, "cache")} {
		if err := serviceDirectory(dir, dataSD); err != nil {
			return err
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if err := writeServiceFile(binary, data, programSD); err != nil {
		return err
	}
	data, err = json.Marshal(config)
	if err != nil {
		return err
	}
	defer clear(data)
	if err := writeServiceFile(filepath.Join(directory, "config", "pkudisk-fuse", "accounts.json"), data, dataSD); err != nil {
		return err
	}
	args := mountArguments(cmd, point)
	if !opts.allAccounts && !cmd.Flags().Changed("account") {
		args = append(args, "--account="+config.Default)
	}
	data, err = json.Marshal(serviceConfig{Args: args, OwnerSID: user.User.Sid.String(), ServiceSID: serviceSID.String()})
	if err != nil {
		return err
	}
	if err := writeServiceFile(filepath.Join(directory, "service.json"), data, dataSD); err != nil {
		return err
	}
	if err := service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: time.Minute}}, 24*60*60); err != nil {
		return err
	}
	// The installing user may control the service but cannot change its executable.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;CCLCRPWP;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(service.Handle, windows.SE_SERVICE, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Installed automatic service for %s as NT SERVICE\\%s; data %s. Run 'pkudisk-fuse service start'.\n", point, serviceName, directory)
	return nil
}

func runService(ctx context.Context, directory string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return errors.New("service run is only available to the Windows service manager")
	}
	if err := localfile.CheckPrivateDir(directory); err != nil {
		return err
	}
	data, err := privateFile(filepath.Join(directory, "service.json"))
	if err != nil {
		return err
	}
	var config serviceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if len(config.Args) < 2 || config.Args[0] != "mount" {
		return errors.New("invalid service mount arguments")
	}
	if _, err := driveLetter(config.Args[1]); err != nil {
		return err
	}
	for _, sid := range []string{config.OwnerSID, config.ServiceSID} {
		if _, err := windows.StringToSid(sid); err != nil {
			return err
		}
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if user.User.Sid.String() != config.ServiceSID {
		return errors.New("service account does not match installed configuration")
	}
	processSD, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + config.ServiceSID + ")(A;;0x00100000;;;" + config.OwnerSID + ")")
	if err != nil {
		return err
	}
	processACL, _, err := processSD.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(windows.CurrentProcess(), windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, processACL, nil); err != nil {
		return err
	}
	if err := os.Setenv("XDG_CONFIG_HOME", filepath.Join(directory, "config")); err != nil {
		return err
	}
	if err := os.Setenv("XDG_CACHE_HOME", filepath.Join(directory, "cache")); err != nil {
		return err
	}
	log, err := localfile.OpenPrivate(filepath.Join(directory, "service.log"), os.O_CREATE|os.O_RDWR)
	if err != nil {
		return err
	}
	defer log.Close()
	if _, err := log.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	fmt.Fprintf(log, "\nStarting service at %s\n", time.Now().Format(time.RFC3339))
	// SCM expects the WinFsp dispatcher promptly, including quota/driver setup.
	startup := time.AfterFunc(25*time.Second, func() {
		fmt.Fprintln(log, "Service startup timed out before mounting; Windows will retry.")
		log.Sync()
		os.Exit(1)
	})
	defer startup.Stop()
	config.ready = func() error { startup.Stop(); return nil }
	ctx = context.WithValue(ctx, serviceContextKey{}, config)
	err = execute(ctx, config.Args, strings.NewReader(""), log, log)
	if err != nil {
		fmt.Fprintln(log, "pkudisk-fuse:", err)
	}
	return err
}

func manageService(ctx context.Context, action string, output io.Writer) error {
	manager, err := connectService(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	access := uint32(windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG)
	switch action {
	case "start":
		access |= windows.SERVICE_START
	case "stop":
		access |= windows.SERVICE_STOP
	case "remove":
		access |= windows.DELETE
	}
	service, err := openMountService(manager, access)
	if err != nil {
		return err
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return err
	}
	switch action {
	case "start":
		if status.State != svc.Running {
			if err := service.Start(); err != nil {
				return err
			}
			for status.State == svc.StartPending || status.State == svc.Stopped {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
				status, err = service.Query()
				if err != nil {
					return err
				}
				if status.State == svc.Stopped {
					return fmt.Errorf("service failed to start (Windows exit %d); inspect its service.log", status.Win32ExitCode)
				}
			}
			if status.State != svc.Running {
				return fmt.Errorf("unexpected service state %d", status.State)
			}
		}
	case "stop":
		if status.State != svc.Stopped {
			process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, status.ProcessId)
			if err != nil {
				return err
			}
			defer windows.CloseHandle(process)
			if _, err := service.Control(svc.Stop); err != nil {
				return err
			}
			// WinFsp reports STOPPED before the Go cache has finished closing.
			if err := waitMountHandles(ctx, []windows.Handle{process}); err != nil {
				return err
			}
		}
	case "remove":
		if status.State != svc.Stopped {
			return errors.New("run service stop before removing it")
		}
		if err := service.Delete(); err != nil {
			return err
		}
		fmt.Fprintln(output, "Removed service; protected accounts and recovery cache were retained.")
		return nil
	}
	status, err = service.Query()
	if err != nil {
		return err
	}
	config, err := service.Config()
	if err != nil {
		return err
	}
	state := "stopped"
	if status.State == svc.Running {
		state = "running"
	}
	fmt.Fprintf(output, "%s: %s (Windows state %d, PID %d); account %s; automatic=%t.\n", serviceName, state, status.State, status.ProcessId, config.ServiceStartName, config.StartType == mgr.StartAutomatic)
	return nil
}
