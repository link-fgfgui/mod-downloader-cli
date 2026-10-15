# Shared View State Layer Contract

`internal/uistate` is the adapter-neutral, pure Go replacement for the GUI's
Vue/Pinia stores (`frontend/src/stores/*.ts`). It holds **no** UI or I/O
dependencies, so the state machines are unit-testable in isolation and reusable
by any adapter.

## Files → source stores

| Go file | Vue source | Responsibility |
| :--- | :--- | :--- |
| `search.go` | `stores/downloadSearch.ts`, `views/Download.vue` | search paging, race discard, install state machine inputs, queue-membership diffing |
| `queue.go` | `stores/downloadQueue.ts`, `App.vue` | global queue drawer state, bell cycle, tab auto-switch |
| `manage.go` | `views/Manage.vue` | physical-JAR grouping, JIJ dedupe, incompatible folding, filters, paths |
| `favorites.go` | `stores/favorites.ts` | scope matching, list ordering/partitioning |
| `minecraft.go` | `stores/minecraft.ts` | instance/target tuple, selected-mods key |
| `settings.go` | `stores/settings.ts`, `composables/useAnimationSettings.ts` | drafts, clamping, animation normalization |
| `keys.go` | `stores/settings.ts` | API-key keep-sentinel resolution |

## Invariants (must not regress)

1. **Search paging** — `BeginSearch(append)` refuses to proceed when the tuple is
   empty, when a request is in flight (`IsSearching`/`IsLoadingMore`), or when
   `HasMoreResults` is false. Append merges onto `AppendBaseResults`; a short
   page (`len < SearchPageSize`) clears `HasMoreResults`.
2. **Race discard** — `ApplySearchUpdate` drops any update whose `RequestID`
   differs from the active one, so a stale response can never overwrite a newer
   result list.
3. **Queue membership diff** — `ApplyQueueMembership` returns true only when the
   sorted `id:status` key changed, preventing redundant
   `GetDownloadStates` refreshes.
4. **Bell cycle** — `ApplyQueue` rings exactly once when the queue transitions
   `active: true → false`, and only if `completedInActiveCycle` was set by a
   `downloadCompleted` event. Failures/cancels never ring.
5. **Drawer presentation** — empty queue ⇒ drawer collapsed; downloads empty but
   reminders present ⇒ `optional` tab; reminders cleared while on `optional` ⇒
   back to `downloads`.
6. **Queue removal** — `CanRemove` allows `pending`/`failed`/`canceled` only;
   `running` entries must be canceled first (removing `pending` leaves no
   `canceled` history — that is enforced by the core).
7. **Manage grouping** — aggregate by `fileName || path || id`, first record is
   `primary`, JIJ entries deduplicated case-insensitively. Incompatible groups
   (`groupLoaders` vs the active loader) move into a `incompatible-toggle` header
   row that is **not** selectable and therefore excluded from `Ctrl+A`.
8. **Favorites scope** — a list with neither version nor loader matches every
   scope; otherwise an active `[MC version | loader]` is required. Ordering is
   pinned-first, then `SortOrder`, then name.
9. **Favorites references** — a list's parent/child relations are managed
   explicitly (`ListFavoriteListRefs` / `AddFavoriteListReference` /
   `RemoveFavoriteListReference`); referenced mods surface in the parent's item
   list flagged as `referenced`. Icon customization supports the `mdi` mode, the
   `project` mode (resolved through `LookupProjectBySlug`), and clearing.
10. **API keys** — an untouched editor field resolves to `APIKeyKeepSentinel`
   (`<keep>`), an explicit clear resolves to `""`; never send `""` for an
   untouched key.
11. **Settings clamping** — file concurrency 1–32, concurrent downloads 1–16
    (UI bound), requests/second 0–100, target rate 0.1–5.0 MiB/s rounded to one
    decimal. Animation switching is disabled, so every stored mode normalizes to
    `vuetify` while still being persisted.

## Shell composition contract (`tui/app.go`)

- `View()` always returns exactly `height` lines, each ≤ `width` display columns
  (CJK-aware). Layout: 2 top-bar rows + `height-3` main rows + 1 bottom-bar row.
- Fallback geometry `80x24` renders before the first `tea.WindowSizeMsg`, so the
  UI appears even when the terminal size cannot be queried.
- Global keys (no modal capturing input, no focused text field): `1`–`6` / `Tab`
  / `Shift+Tab` switch views; `t` opens the instance/target switcher; `q` toggles
  the queue drawer; `Q` clears reminder-only queues; `Esc` closes drawer → top
  modal → input focus → selection → returns Home.
- **Instance/target parity** (`tui/target_modal.go`): the top-bar row-2 instance
  area is a recorded hit region — clicking it opens the switcher, which lists
  `GetVersions()` for `SelectVersion`, offers refresh-mod / refresh-versions
  actions, and lets the user cycle the manual `[MC version | loader]` tuple (which
  clears the selected instance, matching `minecraft.ts`). When
  `selectedVersion.ConnectorAvailable` is true, a Connector segment is rendered
  and both clicking it and the modal row invoke `ToggleConnectorLoader()`.
- Mouse (cell motion): wheel scrolls the active list; left-click on a top tab
  switches views; left-click toggles the queue capsule (**right-click** on a
  reminder-only capsule clears it); list rows support single / `Ctrl`-toggle /
  `Shift`-range selection.
- Modals are a LIFO stack; a modal with `CapturesInput()` swallows all keys.
  Result messages that belong to a modal (versions, pins, favorite lists,
  dependency restore) are dispatched to the top modal first, then the view.
