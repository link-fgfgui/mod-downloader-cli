# TUI Host Adapter Contract

The terminal client is split into two layers:

- `core/host` — UI-neutral host shared with the GUI: core service, HTTP server,
  lifecycle, startup status and typed event dispatch. It has no terminal or
  toolkit dependency.
- `internal/tviewui` — the only TUI (tview/tcell): application shell, pages,
  dialogs and the right-side queue dock.
- `internal/uistate` — UI state only (pagination, drafts, selection); domain
  decisions come from `appcore`/`configs`.

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
attaches the simulation/real screen, registers typed event handlers, starts the
core, runs the tview event loop, and finally stops the event pump before
shutting the host down. Tests use the unexported `run`/`runWith` variants to
inject a SimulationScreen and wait for initial refreshes.

## Lifecycle (`core/host`)

`host.New(host.Options{Version, LoadMinecraftReleases, Config, Runtime, LogLevel, LogToFileOnly, DisableHTTPServer, HTTPAddr})`
returns an unstarted host. `Start(ctx)` mirrors the GUI's startup:

1. `appcore.New(Options{Config, Runtime, Version, LoadMinecraftReleases, OnEvent})`
   followed by `Startup(ctx)`. The core is cached only after a successful
   startup; on failure the partially initialized resources are released and
   the error is returned so `Run` can exit cleanly. `Host.Service()` therefore
   returns nil until `Start()` succeeds, and `Host.Started()` exposes that
   state. Callers must not use the service before then. `Start` is serialized
   and idempotent: a second call keeps the running core/server instead of
   replacing them. Tests may override the internal `startCore` seam to exercise
   startup failures and retries.
2. `httpserver.New(httpserver.DefaultAddr, Options{Accept: core.AcceptExtensionProjects})`
   + `Start()`. The HTTP layer only decodes and encodes; extension resolution
   and the `appcore.EventExtensionModsAccepted` event live in `appcore`. A
   listen failure (e.g. the GUI already owns port 18801) is logged and surfaced
   as a non-fatal `ServerNotice()` in the status bar — the TUI keeps running.
3. `Runtime.DefaultCacheDir` defaults to `os.Getwd()` when the caller leaves it
   empty, exactly as the GUI.

`Host.StartupStatus()` exposes the typed `appstructs.StartupStatus`
(`Ready`/`Stage`/`Error`/`ExtensionServerError`) so both adapters present the
same failure surface.

`appcore.Service.Startup`/`Shutdown`/`Close` are idempotent. A failed `Startup`
tears down anything it initialized and may be retried. Shutdown order is fixed:
`server.Stop()` → `core.Shutdown()`, which stops the mods watcher, drains
Service-owned metadata work and provider background refreshes, then flushes the
selected `.minecraft` directory, SQLite and log files. Core logs rotate without
a new dependency and are never written to the working directory unless
`[logging].file` explicitly points there.

### Logging

`prepareLogging()` configures the core logger with `NoStderr: true`: tview owns
the terminal, so log lines must go to the log file only. The default path is
`<os.UserCacheDir()>/mod-downloader/logs/mod-downloader.log`; `[logging].file`
in `mod-downloader.toml` overrides it. Files rotate at 6 MiB and keep two
historical files by default (`[logging].max_size_mb`,
`[logging].max_backups`). `TestRunKeepsStderrClean` starts the real `Run` path
against a SimulationScreen with `os.Stderr` redirected and asserts that nothing
(including an emitted log record) reaches stderr.

## Event bridge

There is exactly one event taxonomy: `appcore.EventKind`. Adapters register
typed handlers with `host.On[T](h, kind, func(T))` (or `host.OnEvent` for
payload-less events); the payload assertion happens once inside `core/host`.
`appcore.EventKind.Name()` returns the stable kebab-case wire name used by the
Wails frontend.

| core event | wire name | payload |
| :--- | :--- | :--- |
| `EventSearchModsUpdated` | `search-mods-updated` | `appstructs.SearchModsUpdate` |
| `EventDownloadStatesUpdated` | `download-states-updated` | — |
| `EventDownloadQueueUpdated` | `download-queue-updated` | `appstructs.DownloadQueueState` |
| `EventDownloadFailed` | `download-failed` | `appstructs.DownloadFailedEvent` |
| `EventDownloadCompleted` | `download-completed` | — |
| `EventMinecraftDirChanged` | `minecraft-dir-changed` | simplified dir string |
| `EventSelectedVersionChanged` | `selected-version-changed` | `structs.VersionInfo` |
| `EventUsageStatsUpdated` | `usage-stats-updated` | — |
| `EventExtensionModsAccepted` | `extension-mods-accepted` | `appstructs.SearchModsUpdate` |

The tview adapter registers handlers that call `App.enqueueEvent`, which never
blocks the emitter: updates are buffered and applied in batches by a pump
goroutine through `Application.QueueUpdate`/`ForceDraw`. Queue snapshots in a
batch collapse to the newest one, so download progress bursts cost one redraw.
Unknown core event kinds and payloads that fail the contract assertion are
logged at Warn instead of being dropped silently
(`core/host` `TestUnhandledEventsAreLogged`).

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
