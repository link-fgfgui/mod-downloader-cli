# TUI Host Adapter Contract

The terminal client is split into two layers:

- `internal/tuihost` — UI-neutral host: core service, HTTP server, lifecycle and
  an event sink. It has no terminal or toolkit dependency.
- `internal/tviewui` — the only TUI (tview/tcell): application shell, pages,
  dialogs and the right-side queue dock.

The old Bubble Tea package (`tui/`) has been removed; see
[tview-migration.md](tview-migration.md) for the migration record.

## Entry point

`cmd/mod-downloader-cli` decides the mode:

- no subcommand, or `tui` → `tviewui.Run(ctx, Options{Version, LoadMinecraftReleases: true})`
  (full screen, alternate screen, mouse reporting).
- any other subcommand (`i`, `install`, `update`, `verify`, `cache`, `config`, …)
  → the existing `cliapp` workflow, untouched.

`-ldflags "-X main.appVersion=…"` injects the version. `currentAppVersion()`
normalizes empty values to `"dev"`, matching the GUI. The compile-time CurseForge
key defaults via `configs.DefaultCurseforgeAPIKey`, also identical to the GUI.

`tviewui.Run` calls `prepareLogging` before any startup work, builds the host,
attaches the simulation/real screen, registers the event sink, starts the core,
runs the tview event loop, and finally stops the event pump before shutting the
host down. Tests use the unexported `run`/`runWith` variants to inject a
SimulationScreen and wait for initial refreshes.

## Lifecycle (`internal/tuihost`)

`tuihost.New(Options{Version, LoadMinecraftReleases, Config, LogLevel, LogToFileOnly})`
returns an unstarted host. `Start(ctx)` mirrors the GUI's startup:

1. `appcore.New(Options{Config, Runtime, Version, LoadMinecraftReleases, OnEvent})`
   followed by `Startup(ctx)`. On failure the config is still captured and the
   error is returned so `Run` can exit cleanly.
2. `httpserver.New(httpserver.DefaultAddr, Options{OnEvent})` + `Start()`. A
   listen failure (e.g. the GUI already owns port 18801) is logged and surfaced
   as a non-fatal `ServerNotice()` in the status bar — the TUI keeps running.
3. `Runtime.DefaultCacheDir` is `os.Getwd()`, exactly as the GUI.

Shutdown order is fixed: `server.Stop()` → `core.Shutdown()` so the selected
`.minecraft` directory, background tasks, SQLite and log files are flushed.

### Logging

`prepareLogging()` configures the core logger with `NoStderr: true`: tview owns
the terminal, so log lines must go to the log file only. The file path follows
the loaded config (`mod-downloader.toml`, default `mod-downloader.log` in the
working directory). `TestRunKeepsStderrClean` starts the real `Run` path against
a SimulationScreen with `os.Stderr` redirected and asserts that nothing
(including an emitted log record) reaches stderr.

## Event bridge

`Host.SetEventSink(func(Event))` is the only UI dependency. Core events are
translated to UI-neutral values; the adapter decides how to marshal them:

| core event | `tuihost.EventKind` | payload |
| :--- | :--- | :--- |
| `searchModsUpdated` | `EventSearchModsUpdated` | `appstructs.SearchModsUpdate` |
| `downloadStatesUpdated` | `EventDownloadStatesUpdated` | nil |
| `downloadQueueUpdated` | `EventDownloadQueueUpdated` | `appstructs.DownloadQueueState` |
| `downloadFailed` | `EventDownloadFailed` | `appstructs.DownloadFailedEvent` |
| `downloadCompleted` | `EventDownloadCompleted` | nil |
| `minecraftDirChanged` | `EventMinecraftDirChanged` | simplified dir string |
| `selectedVersionChanged` | `EventSelectedVersionChanged` | `structs.VersionInfo` |
| `usageStatsUpdated` | `EventUsageStatsUpdated` | nil |
| `httpserver.ExtensionModsAcceptedEvent` | `EventExtensionModsAccepted` | `appstructs.SearchModsUpdate` |

The tview adapter registers `App.enqueueEvent`, which never blocks the emitter:
events are buffered and applied in batches by a pump goroutine through
`Application.QueueUpdate`/`ForceDraw`. Queue snapshots in a batch collapse to the
newest one, so download progress bursts cost one redraw.

`App.uiUpdate(f)` carries UI mutations requested by controller goroutines
(search results, settings saves, favorite lists, …) through the same pump. This
keeps those goroutines from blocking on tview's unbuffered update channel during
shutdown. `Ctrl+C`/`Quit` stop the pump first, let the in-flight batch finish,
then stop the event loop; `TestRunLifecycleNoLeaks` verifies no goroutine is left
behind.

**Threading rule:** every `appcore.Service` call is made from a controller
goroutine (views call `go app.<controller>`), never from a draw or key handler.
Results mutate widgets only on the UI goroutine via `uiUpdate`.

## Native-dialog replacement (`openPathDialog`)

The GUI's three Wails-runtime methods are replaced by the tview path dialog:

| GUI method | TUI replacement |
| :--- | :--- |
| `ChooseMinecraftDir(locale)` | `openPathDialog(..., save=false)` → `setMinecraftDir` |
| `ChooseCacheDir(locale)` | `openPathDialog(..., save=false)` → `saveCacheDir` |
| `ExportFavoriteListPackwizZip(...)` | `openPathDialog(..., save=true)` → `exportPackwiz` |

Contract:

- directory mode and save-file mode (save mode also lists files);
- a single-line path input with `~`, `$HOME`, `$PWD` and `%VAR%` expansion;
- `Tab` completes the common prefix first and lists candidates on the second
  press; `Shift+Tab` moves focus to the buttons; `Enter`/`Ctrl+S` confirms;
- Packwiz export pre-fills `service.FavoriteListPackwizDefaultFilename(listID)`
  and appends `.zip` when the typed path has no extension;
- an invalid or missing directory is reported inside the dialog, which stays
  open; `Esc` cancels without raising an error;
- there is intentionally no directory tree browser.

Other dialogs (confirm, prompt, select, version list, target/instance, help,
add-favorite, references, disable-impact) are centered tview pages layered on
`Pages`. `Esc` closes only the top dialog; clicks outside a dialog are blocked.

## Clipboard and completion feedback

- Manage copy actions use `atotto/clipboard` (`copySelection`); there is no
  OSC 52 fallback, so remote sessions depend on the terminal's clipboard
  integration.
- The GUI's Web Audio completion sound is not reproduced. Queue completion is
  surfaced by the auto-opened dock, the status-bar capsule and its reminder
  count; `uistate.QueueState.ApplyQueue` still exposes the once-per-cycle ring
  signal for future use.
