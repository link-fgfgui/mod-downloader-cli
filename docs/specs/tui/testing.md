# TUI Testing

The terminal UI is `internal/tviewui` (tview/tcell); the host is `core/host`.
Test code locations:

- UI: `internal/tviewui/*_test.go`; golden frames in
  `internal/tviewui/testdata/frames/`
- Pure state: `internal/uistate/*_test.go`
- Host and event mapping: `core/host/host_test.go`, `host_lifecycle_test.go`
- Real event loop with key/mouse injection: `internal/tviewui/input_loop_test.go`
- Offline controller contract: `internal/tviewui/controller_offline_test.go`
- Live API (env-gated): `internal/tviewui/live_api_test.go`
- Real terminal: `scripts/tui-smoke.sh` (tmux PTY)
- i18n: `internal/i18n/i18n_test.go`

## 1. Test layers

### L0 — pure functions and components

`format_extra_test.go` (speed/percent/compact numbers/progress bars, CJK-safe
`truncate`, checkboxes), `chrome_theme_extra_test.go` (bilingual labels, theme
resolution), `status_mouse_extra_test.go` (status hints, queue capsule width,
click regions, wheel handling, `clipScreen`), `shell_extra_test.go` (breakpoints,
sidebar width, busy state, toast, Esc fallback order), plus `internal/uistate`
(search paging/races, queue bell/presentation, Manage rows, favorites scope,
settings drafts, API-key masking) and `internal/i18n` (language normalization,
key parity, interpolation, fallback).

### L1 — render invariants

`render_test.go`:

- `TestRenderMatrix`: 6 pages × 5 sizes (`50x20 / 80x24 / 100x30 / 120x40 /
  160x50`) × en/zh; asserts row count equals terminal height and no line exceeds
  the width.
- `TestDialogRenderMatrix`: confirm / prompt / select / path / target / help /
  version / favorites / refs / disable-impact at every size and language.
- `assertFrameBounds`: the shared bounds assertion.

### L2 — golden snapshots

`TestGoldenPages` generates 60 files (6 pages × 5 sizes × 2 languages). Update
with `make golden-update`, then review every diff manually; never commit
`-update` output unread. Goldens store screen text only, not color values.

### L3 — keyboard, focus, dialogs

`shell_test.go`, `shell_extra_test.go`, `dialogs_extra_test.go`: number keys,
text-input key swallowing, Tab focus cycling, multi-select, compatibility
folding, language rebuild, dialog stack, settings order; closed dropdowns must
not swallow global keys, open dropdowns own input, Esc unwinds one layer at a
time; confirm/prompt/select/path/target/version/favorites/disable-impact dialog
behavior; dialog stack dedupe, top-only close, focus restoration.

### L4 — mouse

`mouse_test.go`, `list_click_test.go`, `status_mouse_extra_test.go`: wheel
scrolling and post-scroll hit testing; multi-select click semantics (checkbox
toggles one row, title selects one row, description/status/blank areas miss);
action bar, sidebar, double-click degradation; status hint clicks; right-click
queue capsule clears reminders; background clicks are blocked while a dialog is
open; theme switch repaints the palette.

### L5 — events and async

`events_extra_test.go` (8 event kinds, bad payloads, stale search requests,
extension results, status refresh, event compaction), `lifecycle_test.go` (real
event loop start/stop, pump batching, busy spinner, goleak), and `uistate`
request-id discard, append paging, queue membership diff, once-per-cycle bell.

### L5.5 — real event loop and input injection

`input_loop_test.go` drives the real tview event loop through
`SimulationScreen.InjectKey` / `InjectMouse`: 1–6 navigation, `?`/`t`, Esc layers,
`/` search focus, Tab/Backtab, sidebar clicks, live resize (including dialogs),
settings dropdowns, Tab skipping read-only rows, Esc closing only an open
dropdown (regression: used to deadlock the UI goroutine), queue dock, and a
500-event storm that must still respond to navigation.

`controller_offline_test.go` locks the offline contract: every controller entry
must be a safe no-op when `Service() == nil`.

### L6 — page business flows

`pages_extra_test.go` covers Download, Manage, Favorites, Unpin, Settings, Home,
and the Download action bar. `controller_live_test.go` and
`controller_live_more_test.go` use a real `host.Host` + `appcore.Service` on a
SimulationScreen for settings saves, favorites/dialogs, cross-version copy, pin
operations, queue actions, instance switching, Manage batch actions, and version
dialogs.

Constraints: one process may run only one `appcore.Service`, so live tests must
not use `t.Parallel`; every live test must call `Quit` via `t.Cleanup`; controller
calls go through `doUI`, and assertions read UI state through `waitFor` /
`waitUI`.

### L7 — process, live API, real terminal

`run_test.go` covers the full `runWith` startup, `startupDone`, Ctrl+C exit, and
empty stderr. `TestApplicationLifecycleNoLeaks` /
`TestEventPumpAppliesBatchedEvents` run goleak. `live_api_test.go` is skipped by
default; with `MOD_DOWNLOADER_LIVE_API=1` it covers the Mojang manifest, real
Modrinth search, and a real download into a temporary instance.
`scripts/tui-smoke.sh` builds the binary, drives it through a tmux PTY, resizes
100x30 → 60x20 → 120x40, and checks for over-wide frames.

```sh
make test-live-api   # real network
make test-tui-smoke  # real terminal
```

## 2. DAAU (Developer As A User) contract

These behaviors are locked by tests:

- `host.Host.Start` is serialized and idempotent: repeated calls do not replace
  or leak core/HTTP server; after a failure `Service()` is still nil,
  `StartupStatus()` reports a typed failure, and a retry works. `Shutdown` is
  repeatable.
- Offline `App`: all controller methods must be safe no-ops without core. New
  controller methods must be added to `TestOfflineControllersAreNoOps`.
- `App.service()` is the only core entry point; controllers must not bypass the
  offline guard with `host.Service()`.
- Optional page interfaces (`pageActivator`, `pageResizer`, `pageRestyler`,
  `pageClearer`, `pageDropdowns`) have their own event/focus contracts; new
  implementations need tests.
- Core event contract changes must update `appcore.EventKind.Name()`, the typed
  dispatch tests in `core/host`, and `host-adapter.md`. Unknown events or bad
  payloads log Warn; never drop them silently.
- Real-input cases go through the `Application` event loop; offline `testApp` is
  only for pure rendering/focus/hit testing.
- Screen assertions use `screenText`, which skips wide-character continuation
  cells.

## 3. Harnesses

Offline `testApp` (`harness_test.go`):

```go
h := newTestApp(t, 100, 30) // no core; controller methods are no-ops
h.draw()
h.rune('3'); h.key(tcell.KeyEsc)
h.mustContain("Local Mod Management")
h.switchPage(ViewSettings)
```

Online `liveApp` (`controller_live_test.go`):

```go
l := newLiveApp(t)                 // temp HOME/XDG + SimulationScreen + real core
l.doUI(func() { l.app.saveTheme(ThemeLight) })
l.waitFor("theme", func(a *App) bool { return a.theme.Name == ThemeLight })
l.pressRune('3')                   // real InjectKey through input capture
l.resize(60, 20)                   // SetSize + QueueDraw
l.click(x, y)                      // press + release → tview MouseLeftClick
```

Use `doUI` for UI-goroutine controller calls, `waitUI` to queue an assertion after
pending events, and `waitFor` to poll on the UI goroutine (never a bare `sleep`).
Clicks must send press + release; `SetSize` must be followed by a draw.

## 4. Commands and gates

```sh
make test
make test-race
make test-live-api     # network required
make test-tui-smoke    # tmux required
make golden-update
go test ./internal/tviewui ./internal/uistate ./internal/i18n -cover
(cd core && go test ./host ./appcore ./configs -cover)
```

Gates:

- `internal/tviewui` coverage ≥ 80%; `internal/uistate` ≥ 90%.
- Every golden diff is reviewed manually.
- `-race` failures block merging.
- Live tests must not run in parallel (one core per process).
- Real API and tmux suites are manual/pre-release, not default CI.

## 5. Regression list

Keep the corresponding test when fixing these defects:

| Defect | Regression test |
| :--- | :--- |
| Version event scheduled another refresh (feedback loop) | `TestVersionChangeDoesNotScheduleRefresh` |
| Auto-opened queue dock stole focus | `TestQueueFocusRestoration` |
| Open dropdown still let global keys through | `TestTextInputFocusAndDropdownState` |
| Esc during install confirmation leaked the download key | `TestDownloadInstallConfirmationAndMetadataRetry` |
| Connector label used a missing i18n key | `TestConnectorLabelAndAvailability` |
| Favorite checkbox `[X]` was swallowed as a style tag | `TestAddFavoriteDialogSelectionAndCancel` |
| `batchUnpin` passed an empty version and unpinned nothing | `TestLivePinControllers` |
| Failed queue items were invisible in result rows | `TestDownloadQueueRowsAndItemText` |
| Empty initial path resolved to CWD instead of home | `TestInitialPathResolution` |
| Wheel scrolled only the offset, not the cursor | `TestWheelScrollsResultList` |
| Theme switch left old text backgrounds | `TestThemeSwitchRepaintsTextBackgrounds` |
| Settings focus moved outside the viewport | `TestSettingsKeyboardKeepsFocusVisible` |
| Download-rate field could not accept a decimal point | `TestSettingsFormCallbacksAndFocus` |
| Zero-option confirm dialog panicked | `TestDialogEdgeCases` |
| Repeated `Host.Start` replaced the running core | `TestSecondStartIsIdempotent` |
| `Startup` failure cached a half-initialized service | `TestStartFailureCanBeRetried` |
| Offline harness called controllers and panicked | `TestOfflineControllersAreNoOps` |
| Esc with an open settings dropdown deadlocked the UI goroutine | `TestLiveOpenDropdownConsumesEscape` |
| Tab focused read-only section headers | `TestLiveSettingsTabSkipsReadOnlyRows` |
| Clicking the description row triggered selection | `TestDownloadClickZones`, `TestManageClickZones`, `TestFavoritesClickZones` |
| CJK titles shifted click hit testing | `TestDownloadTitleClickWideCharacters` |
| Clicking after scrolling hit the wrong row | `TestDownloadClickAfterScrollingUsesOffset` |
| Event contract drift was dropped silently | `TestUnhandledEventsAreLogged` |

## 6. Out of scope

- Unit tests never perform real network downloads; live tests use local SQLite,
  config, and temporary instance directories only.
- Do not assert exact colors; assert layout, text, palette distinguishability, and
  ANSI completeness.
- 100% coverage is not a goal. Prioritize the event loop, dialogs, settings,
  favorites, queue, and mouse hit testing.
