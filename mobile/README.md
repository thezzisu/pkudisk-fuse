# Shared mobile runtime

Mobile code is optional and isolated from the CLI/TUI/desktop module:

- `bridge/`: platform-independent Go library exposing `NewBridge`, `Open`,
  `Running`, `Status`, `Suspended`, `Suspend`, `QuitRequested`, `Stop`, and `Close`.
  It reuses the root account store, IAAA login, AnyShare client, transfers,
  authenticated HTTP server and embedded Nuxt UI.
- `android/`: Android System WebView host, native task notifications and lifecycle adapter.
  Gradle uses `gomobile bind` to build an AAR with Java bindings and JNI libraries.
- `scripts/build-ios.sh`: builds the same package into an XCFramework for iOS
  devices and simulators, using generated Objective-C bindings.
- `go.mod` / `go.sum`: separate dependency graph, including pinned `gomobile` and
  `gobind` tools. The root `go.work` includes `.` and `./mobile` for shared
  development. Standalone root builds and published CLI installs do not acquire
  mobile tools; `GOWORK=off` selects that independent module graph.

The runtime is an **in-process library on both platforms**. There is no packaged
Go executable, `ProcessBuilder`, child-process control protocol or Android-only
Go entry point. Generated bindings, AARs, APKs and frameworks are ignored.
Android SDK/NDK/JDK and emulator installations belong in shared user locations.

## Bridge contract

`NewBridge(dataDirectory, cacheDirectory, version)` receives the native app version
(e.g. Android `BuildConfig.VERSION_NAME`) for the shared UI.

Create one bridge with app-private persistent-data and cache directories. Call
`Open` on a worker queue and load the returned one-use URL in the app's
WebView/WKWebView on the UI thread. Repeated `Open` calls reuse the service and
issue fresh tickets; never log the URLs. `Running` is nonblocking. `Stop` refuses
to interrupt active operations; `Close` force-cancels work for host destruction
or platform lifecycle limits. Blocking calls must run off the UI thread. A closed
bridge can be opened again. Keep one bridge and serialize host lifecycle commands.

`Status` returns a portable JSON snapshot with `running`, `quitRequested`,
`busy`, and `tasks`. The busy flag includes active tasks, incoming requests and
login operations. Native hosts present those records using their own APIs;
Go contains no Android notification or foreground-service policy.

`Suspend` releases an idle runtime without cancelling work or requesting app
exit. It uses the same atomic `BeginShutdown` check as graceful shutdown, so new
work cannot race an idle check. `Suspended` identifies this intentional release;
only that state may automatically call `Open` again when the user returns.
Android waits one second after backgrounding without work, suspends the bridge
and stops its Service. A pending system file picker keeps the service available.
Unexpected failures and platform timeouts are not idle suspension.

After an authenticated GUI quit, `QuitRequested` distinguishes an intentional
exit from a service failure. Android's existing service monitor observes this
flag and calls `ActivityManager.AppTask.finishAndRemoveTask` to close the app and
remove it from Recents. Unexpected service loss keeps the blocking exit overlay:
**服务未运行，请退出应用** / **The service is not running. Please exit the app.**
This lifecycle signal uses the existing Go binding and requires no JavaScript
bridge.

The browser's URL carries navigation state. Native hosts should retain its
trusted relative path/query and combine it with the next activation origin and
fresh fragment; do not persist activation links or session credentials. Android
uses its standard splash plus a loading overlay until the document's
`data-pkudisk-phase` and first rendered frame are ready. This readiness marker is
read-only and can also be observed by a WKWebView host without exposing a native
JavaScript action bridge.

Android's `TaskNotifications.java` renders per-task native progress and groups
multiple active tasks. Its Service promotes to `dataSync` foreground operation
while the shared snapshot is busy, removes ongoing notifications when work ends,
and remains a normal Service during idle visible use. Future iOS hosts can
consume the same bridge snapshot and suspension API with their own lifecycle
and notification rules.

The native host supplies the platform's file picker and download integration.
Android implements standard WebView `onShowFileChooser` with Activity Result.
A standard `DownloadListener` handles HTTP attachments and starts DownloadManager
with the attachment's path-scoped cookie from `CookieManager`; there is no custom
JavaScript download channel. iOS should use WKWebView file inputs and WKDownload
with its destination delegate. Neither host duplicates NetDisk business APIs
or adds platform dialogs to Go. The shared host `flags` explicitly report
`isMobile: true` on Android and iOS, with `canMount: false` and
`canBrowseLocalFS: false`. The UI uses the platform flag directly for mobile
presentation and exit behavior.

Authentication uses `internal/session` through the same connector as desktop.
The bridge owns only runtime lifetime and app-private directory selection;
`internal/anyshare`, `internal/transfer`, and `internal/gui` provide the operations.

## Build and validate

See [Android](android/README.md) for the installed toolchain, APK build and
emulator E2E commands. Root `go test ./...` does not recurse into this module;
shared mobile tests run from this directory:

```sh
go test -race ./...
go vet ./...
```

On a Mac with Xcode and both iOS SDKs installed:

```sh
./scripts/build-ios.sh
# Output: build/PKUDiskGUI.xcframework
```

The script uses the same pinned gomobile version as Android. Import the generated
framework into an iOS target and use `PKUDiskMobileguiNewBridge` /
`PKUDiskMobileguiBridge` from Swift/Objective-C. A WKWebView shell still needs iOS
lifecycle and download adapters; Android foreground services do not translate to
iOS background execution. No indefinite iOS background-service behavior is assumed.

On Linux, Objective-C API generation can be checked without claiming an iOS build:

```sh
go tool gobind -lang=objc -prefix=PKUDisk -outdir=build/_ios-api ./bridge
```

Full XCFramework linking, an iOS host and iOS device/simulator tests require
macOS/Xcode and have not been validated in this Linux environment.
