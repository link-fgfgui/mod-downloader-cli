# View State Layer Contract

`internal/uistate` is the adapter-neutral, pure Go state layer for the TUI. It
has **no** UI or I/O dependencies, so its state machines are unit-testable in
isolation. Domain decisions (grouping, compatibility, ordering, normalization)
come from `core/appcore` and `core/configs`; `uistate` only holds view state and
small projections.

## Responsibilities

| Go file | Responsibility |
| :--- | :--- |
| `search.go` | search paging, race discard, install-state inputs, queue-membership diffing |
| `queue.go` | queue dock state, bell cycle, tab auto-switch, removable-status rule |
| `manage.go` | row keys, filtering, partition/build rows, selection helpers |
| `favorites.go` | favorite key/scope normalization helpers |
| `minecraft.go` | instance/target tuple, selected-mods key |
| `settings.go` | editable drafts projected from a normalized settings view |
| `keys.go` | API-key keep-sentinel resolution and masking |

## Invariants (must not regress)

1. **Search paging** — `BeginSearch(append)` refuses to proceed when the tuple is
   empty, when a request is in flight (`IsSearching`/`IsLoadingMore`), or when
   `HasMoreResults` is false. Append merges onto `AppendBaseResults`; a short page
   (`len < SearchPageSize`) clears `HasMoreResults`.
2. **Race discard** — `ApplySearchUpdate` drops any update whose `RequestID`
   differs from the active one, so a stale response can never overwrite a newer
   result list.
3. **Queue membership diff** — `ApplyQueueMembership` returns true only when the
   sorted `id:status` key changed, preventing redundant `GetDownloadStates`
   refreshes.
4. **Bell cycle** — `ApplyQueue` rings exactly once when the queue transitions
   `active: true → false`, and only if `CompletedInActiveCycle` was set by a
   `downloadCompleted` event. Failures/cancels never ring.
5. **Drawer presentation** — an empty queue collapses the drawer; downloads empty
   but reminders present opens the `optional` tab; clearing reminders while on
   `optional` returns to `downloads`.
6. **Queue removal** — `CanRemove` allows `pending`/`failed`/`canceled` only;
   `running` entries must be canceled first.
7. **Manage rows** — grouping, JIJ dedupe, compatibility, and search text come
   from `appview.LocalModGroup` (`BuildLocalModGroups` in core). `uistate` filters
   and partitions groups into rows; the `incompatible-toggle` header row is not
   selectable and is excluded from `Ctrl+A`.
8. **Favorites** — list ordering and scope matching come from core
   (`FavoriteOverview`, `ListFavoriteContentsForScope`); `uistate` only normalizes
   keys and scopes.
9. **API keys** — an untouched editor field resolves to `APIKeyKeepSentinel`
   (`<keep>`), an explicit clear resolves to `""`; never send `""` for an untouched
   key.
10. **Settings** — drafts are a straight projection of the normalized
    `SettingsView`; `ValidateNetworkSettings` delegates clamping to
    `configs.NormalizeNetworkSettings`. Do not re-declare boundary constants in
    the adapter.

## Shell composition contract (`internal/tviewui`)

- Root layout: `shellLayout` → `Flex` rows [content, 1-row status bar]. Content is
  a column `Flex`: sidebar (30 / 7 / 0 columns at ≥120 / ≥50 / <50), the `Pages`
  stack for the six views, and an optional right-side queue dock (width clamped
  to 28–40). Every frame fills exactly the terminal size.
- Page contract: `Primitive`, `Refresh`, `Focus`, `HandleKey`, `Hints`, plus
  optional `OnActivate` / `OnResize` / `Restyle` / `ClearFocus`. Widgets are
  projections of App state; lists/tables rebuild from one ordered slice and
  restore the cursor by stable key.
- Single source of truth: App embeds the `uistate` states plus the core settings
  and usage-stat snapshots. Views never keep a second copy of business state.
- Threading: controller methods run in goroutines and submit results through
  `uiUpdate`; the event pump applies host events and UI tasks in one
  `QueueUpdate` batch and then redraws. No service call runs on the UI goroutine
  or inside a draw/key handler.
- Global keys (no dialog, no focused text field): `1`–`6` switch views; `Tab`
  moves focus within a page and between queue tabs; `t` opens the
  instance/target switcher; `q` toggles the queue dock; `Q` clears reminder-only
  queues; `?` opens help; `Esc` unwinds dialog → dock → page input/selection →
  Home; `Ctrl+C` performs the graceful stop.
- **Instance/target parity** (`dialogs_target.go`): the switcher lists
  `GetVersions()` for `SelectVersion`, offers refresh-mod / refresh-versions
  actions, and lets the user cycle the manual `[MC version | loader]` tuple
  (which clears the selected instance). Instance rows show only
  `VersionInfo.Name` (or `ID`); the picker's gray secondary line shows
  `MC version | loader`, while the target dialog keeps them in dedicated rows.
  When `selectedVersion.ConnectorAvailable` is true, the Connector segment toggles
  `ToggleConnectorLoader()`.
- Dialogs are a LIFO stack of full-screen `Pages` containing a centered bordered
  primitive. `Esc` closes only the top dialog, background clicks are blocked, and
  there is no scrim (tview pages are opaque).
- Mouse: row/button clicks and wheel scrolling. Multi-select lists use precise hit
  testing: the left checkbox toggles one row, clicking the title text selects only
  that row, and the gray description line / other cells count as a miss. Keyboard
  multi-selection still uses `Space` rather than Ctrl/Shift click. **Right-click**
  on a reminder-only capsule clears reminders.
- Responsive: Home rebuilds its metric-card columns from the main width and the
  queue dock reflows on resize. Golden frames cover 6 pages × 5 sizes × en/zh in
  `internal/tviewui/testdata/frames`; the SimulationScreen matrix asserts every
  line stays within the terminal bounds.
