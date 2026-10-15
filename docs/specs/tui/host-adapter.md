# TUI Host Adapter Contract

The `tui` package is Layer 2 of the shared architecture: it replaces the Wails
`app.go` host (Runtime + native dialogs) while reusing Layer 1
(`appcore.Service`, `httpserver.Server`) unchanged.

## Entry point

`cmd/mod-downloader-cli` decides the mode:

- no subcommand, or `tui` → `tui.Run(ctx, Options{...})` (full screen, alt-screen,
  mouse reporting).
- any other subcommand (`i`, `install`, `update`, `verify`, `cache`, `config`, …)
  → the existing `cliapp` workflow, untouched.

`-ldflags "-X main.appVersion=…"` injects the version. `currentAppVersion()`
normalizes empty values to `"dev"`, matching the GUI. The compile-time CurseForge
key defaults via `configs.DefaultCurseforgeAPIKey`, also identical to the GUI.

## Lifecycle (`Host`)

`tui/host.go` mirrors `App.startup` / `App.shutdown`:

1. `appcore.New(Options{Config, Runtime, Version, LoadMinecraftReleases: true, OnEvent})`
   followed by `Startup(ctx)`. On failure the config is still captured and the
   error is returned so `Run` can exit cleanly.
2. `httpserver.New(httpserver.DefaultAddr, Options{OnEvent})` + `Start()`. A
   listen failure (e.g. the GUI already owns port 18801) is logged and surfaced
   as a **non-fatal** notice rendered in the top bar — the TUI keeps running.
3. `Runtime.DefaultCacheDir` is `os.Getwd()`, exactly as the GUI.

Shutdown order is fixed: `server.Stop()` → `core.Shutdown()` so the selected
`.minecraft` directory, background tasks, SQLite and log files are flushed.

### Logging

`prepareLogging()` sets `LOGGING_FORCE_FILE=true` before startup. Bubble Tea owns
stdout for the full-screen UI, so core log lines must go to the log file; emitting
them to stdout corrupts the display. (Verified by pty smoke runs.)

## Event bridge

`Host.emitCoreEvent` maps every `appcore.EventKind` to a strongly typed
`tea.Msg`, delivered via `program.Send` (safe from background goroutines):

| core event | payload | tea message |
| :--- | :--- | :--- |
| `searchModsUpdated` | `appstructs.SearchModsUpdate` | `SearchModsUpdatedMsg` |
| `downloadStatesUpdated` | nil | `DownloadStatesUpdatedMsg` |
| `downloadQueueUpdated` | `appstructs.DownloadQueueState` | `DownloadQueueUpdatedMsg` |
| `downloadFailed` | `appstructs.DownloadFailedEvent` | `DownloadFailedMsg` |
| `downloadCompleted` | nil | `DownloadCompletedMsg` |
| `minecraftDirChanged` | simplified dir string | `MinecraftDirChangedMsg` |
| `selectedVersionChanged` | `structs.VersionInfo` | `SelectedVersionChangedMsg` |
| `usageStatsUpdated` | `storage.UsageStats` | `UsageStatsUpdatedMsg` |

`emitHTTPServerEvent` bridges `httpserver.ExtensionModsAcceptedEvent` (payload
`appstructs.SearchModsUpdate`) to `ExtensionModsAcceptedMsg`, which opens the
Download view with the injected snapshot.

**Rule:** every `appcore.Service` call runs inside a `tea.Cmd` goroutine (see
`tui/commands.go`). Core methods are network-bound and emit events synchronously,
so calling them inside `Update` would block the render loop. All state mutation
therefore happens on the main goroutine when the resulting `tea.Msg` arrives.

## Native-dialog replacement (`PathPickerModal`)

The GUI's three Wails-runtime methods are replaced by `tui/pathpicker.go`:

| GUI method | TUI replacement |
| :--- | :--- |
| `ChooseMinecraftDir(locale)` | `pickerDirectory` → `cmdSetMinecraftDir` |
| `ChooseCacheDir(locale)` | `pickerDirectory` → `cmdSaveCacheDir` |
| `ExportFavoriteListPackwizZip(...)` | `pickerSaveFile` → `cmdExportPackwiz` |

Contract:

- directory mode and save-file mode (`DirAllowed` / `FileAllowed` flipped);
- a typed path input with `Tab` completion (candidates are re-derived while
  cycling) and `shift+tab` to move focus to/from the `bubbles/filepicker` tree;
- `ShowHiddenFiles` is always on;
- Packwiz export pre-fills `service.FavoriteListPackwizDefaultFilename(listID)`
  and appends `.zip` when the typed path has no extension;
- `Esc` (or the cancel path) returns `Canceled` semantics — no error is raised;
- only `.zip` files are selectable in save-file mode.

## Clipboard and bell adapters

- Manage/Download copy actions use `tui/clipboard.go`: `atotto/clipboard` for the
  local clipboard **plus** an OSC 52 escape written to stdout so the copy also
  works across SSH sessions.
- The GUI's Web Audio completion sound is replaced by writing `\a` (BEL) to
  stdout, emitted **once** per drained queue cycle that observed at least one
  successful completion (see `uistate.QueueState.ApplyQueue`).
