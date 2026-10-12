# Platform support

The native CLI and TUI build for Linux, macOS and Windows on amd64 and arm64 without CGO. Account management, browsing, uploads and downloads do not need a filesystem driver.

| Platform | Mount backend | Requirement |
| --- | --- | --- |
| Linux | go-fuse | `/dev/fuse` and `fusermount3` or `fusermount` (normally the `fuse3` package) |
| macOS | go-fuse / macFUSE | [macFUSE](https://macfuse.github.io/) with its kernel extension enabled; the FSKit backend is not used |
| Windows | WinFsp through cgofuse | [WinFsp](https://winfsp.dev/rel/) matching the binary architecture; WSL is not required |

The executable checks mount dependencies before opening a saved account or asking for credentials. `mount --help` contains instructions for the current platform.

## Mount examples

Linux and macOS:

```sh
mkdir -p ~/PKUNetDisk
pkudisk-fuse mount ~/PKUNetDisk
```

Windows, from PowerShell or Command Prompt, with an unused drive letter:

```powershell
pkudisk-fuse.exe account add
pkudisk-fuse.exe mount P:
```

An empty directory can also be used as a Windows mountpoint. Ctrl-C unmounts the filesystem. Other mount flags, including `--root`, `--read-only`, `--cache-limit` and `--io-timeout`, have the same meaning on each platform.

`mount --all-accounts` works with both drivers. Saved account names become the
first-level directories, with independent remote/cache identities and sessions.
Account names must be valid local directory names; Windows rejects reserved
names and case-insensitive collisions. Cross-account rename is rejected before
any remote mutation. Shared library capacity is counted once per server.

`tui --all-accounts` uses the same account directories and requires no mount
driver. Every saved account is restored and its libraries checked before
opening the browser. Transfers and sharing use the selected object's owning
client; internal account prefixes are never sent to server APIs. Directory
and ZIP transfers, overwrite/revision checks and extraction work through the
existing transfer service. Account directories are virtual and read-only.

Use `mount P: --verbose 2> mount.log` from a local folder to diagnose application freezes. Verbose logs record WinFsp callbacks before handle/node locks, plus metadata decisions, cached and remote reads, HTTP connection/header/body phases and operation durations. A numbered `START` without its matching `END` identifies outstanding work in this process. `--debug` adds driver protocol diagnostics when needed; neither flag can observe a request that never reaches this process. Linux/macOS provide the same verbose cache/HTTP diagnostics and go-fuse callback logging.

WinFsp opens directory handles for attribute/security probes as well as enumeration. These opens do not fetch directory contents; the first `Readdir` loads a snapshot. Known paths avoid ancestor listing walks. Metadata from a recent listing satisfies opens and attributes until the existing `--cache-ttl` expires. Concurrent metadata/listing requests share a fetch, and metadata HTTP calls run outside the file lock. An external rename/deletion invalidates obsolete path/listing state, and older enumeration snapshots cannot replace newer metadata.

Both drivers use the shared AnyShare backend and disk cache. Reads fetch files up to 16 MiB whole, start larger-file reads with 4 MiB ranges, and grow sequential fetches up to 16 MiB. Seeks reset growth; overlapping requests share one fetch. The default concurrency is four, and network reads do not hold the file metadata lock or block warm reads. `--read-whole-file`, `--read-block-size`, `--read-ahead` and `--read-concurrency` tune this behavior identically on each platform. Random writes, append and truncation are cached locally and published on flush/fsync. Clean reads and flushed files share the same budget and are evicted before pending writes. Revision checks prevent overwriting a concurrent remote edit; failed uploads retain recovery journals. Windows uses WinFsp's POSIX operation callbacks and maps remote errors to the driver's errno values.

Mounted reads consume incoming range data progressively: a header read can finish while the rest of its shared range continues filling. The range must pass final size/EOF validation before entering the clean disk cache. Cancelled waiters do not interrupt useful shared fills; unused cold work is cancelled when abandoned, and unmount stops and joins every fill. Native WinFsp tests require a 64 KiB file read to return while the remainder of a 5 MiB range is deliberately withheld.

Remote paths always use forward slashes, including on Windows. Local paths use the host's syntax. `~`, `~/...` and `~\...` expand to the current user's home directory. Windows reserved names, alternate-data-stream syntax, backslashes within a remote filename and names ending in a dot or space cannot be used as local filenames. The mount omits such names; CLI `ls` can show them, and `download REMOTE EXPLICIT-LOCAL-FILE` can save their contents under a valid name.

## Windows background and boot mounts

The default mount stays in the foreground. PowerShell's `&` invokes a command;
it does not detach the process. To mount an unused drive letter without keeping
a terminal open:

```powershell
pkudisk-fuse mount P: --background
pkudisk-fuse unmount P:
```

Background mounting starts the same executable with Windows `CREATE_NO_WINDOW`.
There is one Go process per mount, with no launcher script or extra executable.
Startup waits for the actual drive letter. Authentication/driver failures return
an error and a log path. Logs are private files at
`%LOCALAPPDATA%\pkudisk-fuse\background\P.log` (the cache-home override applies).
Authentication is noninteractive; run `account add` first if verification is
needed. `--password-stdin` cannot be used with `--background`. `unmount` signals
the existing mount to follow its normal WinFsp/cache shutdown path.

For mounting at boot, including before logon, use an administrator terminal:

```powershell
pkudisk-fuse service install P:
pkudisk-fuse service start
pkudisk-fuse service status
pkudisk-fuse service stop
pkudisk-fuse service remove
```

Installation registers the native `pkudisk-fuse` Windows service with delayed
automatic startup and a dependency on `WinFsp.Launcher`. WinFsp provides the
service protocol and global drive mapping. Windows retries an unexpected exit
after one minute; startup must finish within 25 seconds or exits for a retry.
The installed executable is protected below `%PROGRAMDATA%\pkudisk-fuse-service`.
The service runs as the dedicated virtual account `NT SERVICE\pkudisk-fuse`,
without requiring or storing a Windows password. It still uses one Go process.
The installing user can start, stop and query the service; changing its
installation requires administrator privileges.

Install copies the selected saved account (or `--all-accounts`) into a private
service-owned store. It does not broaden the original account file's ACL.
Service configuration, accounts, logs (`service.log`) and cache reside below
`%PROGRAMDATA%\pkudisk-fuse-service\data`; only the virtual account and SYSTEM
access that data. The mounted files allow the installing user, the service
account and SYSTEM. The service does not prompt for verification. Stop it,
complete `account add --replace` interactively and reinstall to refresh its
account copy when required. Separate account stores do not share token-rotation
locks; avoid running a user mount of the same account alongside the service.

Mount tuning flags such as `--root` and `--read-only` also apply to
`service install`. Services use their own cache and saved credentials;
`--cache-dir`, explicit credential flags and `--background` are rejected.
Stop before reinstalling to update the binary or accounts. Close applications
using the drive before stopping. The stop command waits for the mount process
to exit; interrupted writes remain subject to the existing recovery behavior.
Removal keeps service accounts and the cache so recovery data is not deleted.
Accessing service recovery files requires the service account or administrative
backup access; the ordinary user's `recover` cannot bypass those ACLs.

HTTP requests use Go's standard `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`
environment variables. Background mounts inherit the launching terminal's
environment. Services use their Windows service environment; temporary proxy
settings in the installing terminal are not copied into the service.

## Account storage and permissions

| Platform | Default account file | Default mount-cache parent |
| --- | --- | --- |
| Linux | `~/.config/pkudisk-fuse/accounts.json` | `~/.cache/pkudisk-fuse` |
| macOS | `~/Library/Application Support/pkudisk-fuse/accounts.json` | `~/Library/Caches/pkudisk-fuse` |
| Windows | `%APPDATA%\pkudisk-fuse\accounts.json` | `%LOCALAPPDATA%\pkudisk-fuse` |

`XDG_CONFIG_HOME` and `XDG_CACHE_HOME` override the parent directories on all three platforms. Files remain bound to the selected saved account and server.

Unix requires current-user ownership, mode 0600 for credential files and mode 0700 for their directories. Windows creates a protected ACL allowing the current user and SYSTEM only, checks that ACL and ownership before reading credentials, and rejects reparse points. A POSIX `chmod` command does not substitute for a private Windows ACL. Interactive `account add` creates the appropriate permissions automatically; `--password-stdin` is another way to avoid preparing a credential file.

Store updates and refresh-token rotation use process locks (`flock` on Unix; `LockFileEx` on Windows). Replacement files are synced and installed using the platform's rename operation; Windows uses `MoveFileEx` with write-through. Downloads install only after validation, and refuse to overwrite a destination created during the transfer unless `--overwrite` was requested.

## Validation

CI uses one native test job each for Linux, macOS and Windows. Formatting, dependency metadata and vulnerability checks run once on Linux, which also cross-builds all six OS/architecture combinations. Go tests run their standard vet checks; Linux/macOS also use the race detector. Linux requires its kernel FUSE tests to execute, including memory-mapped reads. Windows installs WinFsp and requires actual mounted-drive tests with writes, reads, Chinese filenames, rename/removal, JPEG decoding through Go and Windows GDI+, and read-only file mapping; the 5 MiB photo test requires one fetch. Shared tests cover sequential growth, seek reset, range batching, concurrent fetches and admission, cancellation, bounded reads of a generated 20 GiB file, revision changes, eviction and pending-write protection. Permission tests verify private ACLs and process-lock contention.

Windows regressions replay 6,648 directory metadata/open/close probes with expired listings, verify lazy enumeration snapshots and external rename/replacement, and exercise native `CreateFile` attribute-only directory opens. Shared tests cover coalesced directory/metadata requests, cancellation, invalidation during an in-flight listing, and cached reads or local writes during a blocked metadata refresh.

The macOS hosted runner tests the native CLI/storage code and compiles the go-fuse adapter. Kernel-extension approval is unavailable on the hosted runner, so macFUSE mounting needs acceptance on a Mac with the driver enabled. Run `PKUDISK_FUSE_REQUIRE_KERNEL=1 go test -race ./internal/vfs` on that Mac to require the actual mount tests; the default macOS suite skips them.
