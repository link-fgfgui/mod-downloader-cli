# Architecture

This document describes the current layering of `mod-downloader-cli`, the shared
`core` module, and the refactors already completed. It replaces the former
`architecture-improvement-plan.md` and `core-gui-tui-adaptation-plan.md`; both are
available in git history.

## Layering

```text
cmd/mod-downloader-cli
  ├─ (no subcommand | tui) ──► internal/tviewui    tview/tcell full-screen TUI
  │                              ├─ internal/uistate   pure UI state
  │                              ├─ internal/i18n      zh/en dictionaries
  │                              └─ core/host          lifecycle + HTTP + typed events
  │
  └─ other subcommands ──────► cliapp               urfave/cli commands + huh wizard
                                   │
                                   ▼
                         core/appcore.Service        adapter-neutral facade
                         (search / favorites / settings / downloads / instances / events)
                                   │
        ┌──────────────────────────┼─────────────────────────────┐
        ▼                          ▼                             ▼
   providers ───────────────► modbridge ───────────────► downloader
        │                          │                             │
        └──────────────► storage / minecraft / packwiz ◄─────────┘
                                   │
                    global / configs / logging / networking / httpserver
```

- `cliapp` owns flags, config precedence, table/JSON output, and exit codes.
- `internal/tviewui` owns layout, focus, dialogs, the queue dock, and the event
  pump. `internal/uistate` holds only UI state (pagination, drafts, selection);
  domain decisions come from `appcore` / `configs`.
- `core/host` is the lifecycle path shared by the TUI and GUI. It owns the
  single-instance port guard, the extension HTTP server, and typed event
  dispatch. One-shot CLI commands create an `appcore.Service` directly through
  `withService` and keep the HTTP server disabled.
- `core/appcore.Service` is the process-wide facade. Domain packages do not
  import any adapter.

## Shared core

`core` is a standalone module at `../mod-downloader-gui-v3/core`, consumed through
the `core` symlink and the committed `go.work`. Import path:

```text
github.com/link-fgfgui/mod-downloader/core
```

Development uses the workspace. Release builds are expected to consume a tagged
`core` version and drop the `replace` directives; until then, both repositories
must be checked out side by side (or `make bootstrap` must restore the symlink).

Rules:

- `core` must not import adapter packages.
- Event kinds and wire names are defined once in `appcore`; adapters register
  typed `host.On` / `host.OnEvent` handlers.
- Any rule that writes core state or must behave identically in the GUI and TUI
  lives in core. Adapters only render projections.
- `appcore.Service` remains a process-wide singleton. `core/host` wraps one
  service and does not change that constraint.

## Completed refactors

| Area | Result |
| --- | --- |
| Core single source (Phase 0) | Submodule replaced by the sibling symlink; CLI import path unified with the GUI; `go.work` added |
| TUI migration | The Bubble Tea `tui/` package was removed; `internal/tviewui` (tview/tcell) is the only TUI, and its host moved to `core/host` |
| `core/host` (Phase 1) | TUI `internal/tuihost` removed; GUI and TUI share lifecycle, HTTP, and startup status |
| Event contract (Phase 2) | One `appcore.EventKind`; `EventKind.Name()` provides kebab-case wire names; single typed assertion inside `core/host` |
| Settings normalization (Phase 3) | `configs.Normalize*` is the only clamp/boundary implementation; TUI and GUI no longer duplicate limits |
| Favorites projection (Phase 4) | `FavoriteOverview` and scope filtering live in core; ordering is `pinned DESC, sort_order ASC, created_at ASC, name ASC` |
| Manage grouping (Phase 5) | `BuildLocalModGroups` owns grouping, JIJ dedupe, compatibility, and search text; GUI `lib/modGroups.ts` deleted |
| CLI slimming (Phase 6) | `cliapp/jarmeta.go` deleted; jar parsing, version ranges, verify/upgrade workflows, and target discovery moved to core |
| Command surface | `install` multi-target and requirements support, `enable`/`disable`, `check-update`/`upgrade`, `verify`/`info`, `freeze`, `config`, `cache clean` |
| Testing | Golden frames, SimulationScreen input/mouse tests, goleak, live API and tmux smoke suites |

## Remaining backlog

These are known, non-blocking items. None is required for the current feature set.

| Item | Current state | Trigger / next step |
| --- | --- | --- |
| Single config resolver | CLI reads `config.json`; the TUI/GUI read `mod-downloader.toml`; precedence is implemented in two places | Introduce an `EffectiveSettings` resolver in core and map the six CLI keys to it |
| Core de-globalization | `downloader`, `storage`, `global`, and `providers` still hold package-level state | Start when multi-instance or parallel-test interference appears; follow the storage → providers → global → downloader order |
| i18n externalization | `internal/i18n/i18n.go` is an 800-line Go map | Move `zh`/`en` maps to embedded JSON and add key-parity/used-key tests |
| Contract type relocation | `internal/tviewui` still imports `core/storage` DTOs | Move `PinnedMod`, `FavoriteList`, and related types to `core/structs` when the GUI binding can change in lockstep |
| Release modules | `go.work` + `replace` during development | Tag `core`, update both modules, and remove `replace` before publishing binaries |

## Verification baseline

```sh
go build ./... && go vet ./... && go test ./...            # CLI
go test github.com/link-fgfgui/mod-downloader/core/...     # core via workspace
go test -race ./...                                        # CLI race
make test-tui-smoke                                        # real terminal
```

The GUI repository runs its own build/test baseline; any change to core public
APIs or events must be verified in both repositories.
