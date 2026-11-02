# mod-downloader-cli

Command-line and full-screen terminal client for downloading and managing
Minecraft mods. It resolves projects through Modrinth and CurseForge, installs
compatible versions into a `mods` directory, manages local mod files, and shares
its entire domain layer with the Wails GUI.

- **CLI**: `show`, `install`, `list`, `remove`, `enable` / `disable`,
  `check-update`, `upgrade`, `verify`, `info`, `freeze`, `config`, `cache clean`.
- **TUI**: run with no subcommand, or `mod-downloader-cli tui`.

## Requirements

- Go 1.26 for the workspace (the shared `core` module requires 1.24)
- Network access for searching and downloading
- A CurseForge API key for CurseForge results (`--curseforge-api-key` or
  `CF_API_KEY`); Modrinth works without a key

## Layout and build

The CLI, GUI, and shared core are three modules in two sibling checkouts:

```text
mod-downloader-proj/
  mod-downloader-cli/        this module
  mod-downloader-gui-v3/     Wails GUI module
    core/                    shared core module
```

The committed `go.work` resolves the CLI, GUI, and `core` from the sibling
checkout, so `go build ./...` and `go test ./...` need no symlink. The `core`
symlink (`core -> ../mod-downloader-gui-v3/core`) remains a fallback for tools
that build one module in isolation; restore it with `make bootstrap`.

```sh
make build                              # go build ./cmd/mod-downloader-cli
go run ./cmd/mod-downloader-cli --help
```

On Windows, enable symlinks before cloning
(`git config --global core.symlinks true`). CI must check out both repositories
side by side or run `make bootstrap`.

## Quick start

Run these inside an instance's `mods` directory:

```sh
cd /path/to/.minecraft/versions/fabric-1.21.1/mods

mod-downloader-cli install sodium          # exact slug/ID, keyword fallback
mod-downloader-cli show modrinth:sodium    # compatible versions
mod-downloader-cli list                    # installed mods
mod-downloader-cli remove sodium           # delete a mod file
mod-downloader-cli                         # launch the TUI
```

`--mc-version` and `--loader` are optional when the CLI can infer them from the
instance path (standard `.minecraft/versions/<id>/mods` or Prism Launcher
layouts). Explicit flags always win:

```sh
mod-downloader-cli --mc-version 1.21.1 --loader fabric show modrinth:sodium
mod-downloader-cli -C /path/to/instance install modrinth:sodium
```

## Commands

| Command | Aliases | Purpose |
| --- | --- | --- |
| `show <project>` | — | Show matching project versions |
| `install <project...>` | `i` | Install projects (exact lookup, then keyword search fallback) |
| `list` | `mods` | List installed mods |
| `remove <mod...>` | — | Delete installed mod files |
| `enable <mod...>` | — | Rename `<file>.jar.disabled` back to `<file>.jar` |
| `disable <mod...>` | — | Rename `<file>.jar` to `<file>.jar.disabled` |
| `check-update` | `outdated` | Compare installed mods with the latest compatible versions |
| `upgrade [<mod...>]` | — | Download newer versions and remove superseded jars |
| `verify` | `check` | Check missing dependencies, duplicate IDs, and incompatibilities |
| `info <mod-or-jar>` | `inspect` | Show metadata, dependencies, hashes, and online project link |
| `freeze` | — | Print a pip-style requirements list |
| `config <get\|set\|unset\|list\|path>` | — | Manage persistent user configuration |
| `cache clean` | — | Delete the metadata cache file |

Most filesystem commands require the current directory to be named `mods`; pass
`--force` to override. All commands support `--json`.

### `install` / `i`

```sh
mod-downloader-cli install [flags] <project...>
mod-downloader-cli install [flags] -r <requirements-file>
```

Accepted project references:

| Form | Example | Behavior |
| --- | --- | --- |
| Slug | `sodium` | Exact lookup across enabled platforms |
| Project ID | `AANobbMI`, `394468` | ID-shape-aware, cross-platform lookup |
| Platform prefix | `modrinth:sodium`, `curseforge:394468` | Locks the platform |
| Keyword query | `mini map` | Falls back to a top-10 keyword search |

Key flags: `--platform`, `-y/--yes`, `--no-input`, `--dry-run`, `--json`,
`--version-id`, `-r/--requirement`, `--timeout`, `--force`, `--server`.

- With multiple arguments, every token is resolved exactly first; if any token
  misses, the whole input is retried as one keyword search.
- Version/file identifiers (`version:`, `file:`, a 7+ digit CurseForge file ID)
  are rejected: `install` chooses the version from the active
  `[Minecraft version | loader]` and preferred release channel.
- `-r <file>` installs every entry from `freeze`; blank lines and `#` comments
  are ignored, `==version` pins a version, and a failing entry does not stop the
  rest. It cannot be combined with project arguments, `--platform`,
  `--version-id`, `--dry-run`, or `--server`.
- `--server` starts the shared HTTP receiver on `127.0.0.1:18801` and blocks
  until `Ctrl+C`; it accepts browser-extension payloads and queues them for
  install. See [`docs/specs/cli/target-resolution.md`](docs/specs/cli/target-resolution.md).

Resolution order, in short: platform prefix → `--platform` filter → concurrent
lookup on both platforms → exact hit, interactive disambiguation, or keyword
fallback. Non-interactive mode never hangs; it returns an actionable error.

### Other commands

```sh
mod-downloader-cli show --platform modrinth sodium
mod-downloader-cli list --json
mod-downloader-cli remove --dry-run sodium
mod-downloader-cli disable sodium
mod-downloader-cli check-update --platform modrinth
mod-downloader-cli upgrade --all --dry-run --timeout 2m
mod-downloader-cli verify --strict
mod-downloader-cli info sodium
mod-downloader-cli info ./some-mod.jar
mod-downloader-cli freeze > mods.txt
mod-downloader-cli install -r mods.txt
```

`verify` exits `1` when missing dependencies or duplicate IDs are found;
`--strict` also fails on compatibility warnings. `upgrade` removes the old jar
only after the new download succeeds and keeps a disabled mod disabled.

## Configuration

The CLI does **not** read or create `mod-downloader.toml`. Persistent settings
live in the OS user config directory:

```text
Linux:   ~/.config/mod-downloader-cli/config.json
macOS:   ~/Library/Application Support/mod-downloader-cli/config.json
Windows: %APPDATA%\mod-downloader-cli\config.json
```

Supported keys: `mc-version`, `loader`, `curseforge-api-key`,
`modrinth-api-key`, `cache-dir`, `minecraft-dir`. Secret values are masked in
`config get` / `config list`; `--show-secrets` prints them.

Precedence, highest first: command-line flags → environment variables → config
file → automatic detection.

## TUI

The TUI mirrors the desktop app: left navigation (Home / Download / Manage /
Favorites / Unpin / Settings), instance and target selector, one status bar, and
a right-side queue dock. The sidebar collapses to icons between 50 and 119
columns and hides below 50 columns; `1`–`6` still switch views.

| Key | Action |
| --- | --- |
| `1`–`6` | Switch view |
| `/` | Focus the Download or Manage search field |
| `↑`/`↓`, `PgUp`/`PgDn`, `Home`/`End` | Move the list/table cursor |
| `Space` | Toggle multi-selection |
| `Enter` | Open/activate the current row |
| `e` / `d` / `i` | Manage: enable, disable, invert selection |
| `x` | Delete the selection (Manage) or remove favorites |
| `S` | Manage: scan unused dependencies |
| `v` | Open the version list |
| `f` | Add the selection to favorites |
| `r` | Refresh the current page |
| `q` / `Q` | Toggle the queue dock / clear optional-dependency reminders |
| `t` / `?` | Instance and target selector / keyboard help |
| `Esc` | Close dialog, clear selection, then return Home |
| `Ctrl+C` | Quit |

Dialogs are centered tview pages; click their buttons, but press `Esc` to close
them. The file picker is a path input with `~`, `$HOME`, and `$PWD` expansion:
`Tab` completes the common prefix, press again to list candidates, and `Enter`
or `Ctrl+S` confirms. Invalid directories are reported inside the dialog.

Logging is quiet by default. Pass `-v` / `--verbose` for debug logs; the TUI
writes info-level records to the per-user cache directory
(`~/.cache/mod-downloader/logs/mod-downloader.log` on Linux), rotating at 6 MiB
with two backups. Set `[logging] enabled=false` in `mod-downloader.toml` to
disable file logging.

## Development

```sh
make test           # CLI + core
make test-race      # both modules with the race detector
make vet            # go vet for both modules
make test-live-api  # real Mojang/Modrinth network tests
make test-tui-smoke # build the binary and drive it through a tmux PTY
```

See [`docs/specs/tui/testing.md`](docs/specs/tui/testing.md) for the test
matrix, harness contracts, and coverage gates.

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — layering, shared core, and
  remaining backlog
- [`docs/specs/cli/target-resolution.md`](docs/specs/cli/target-resolution.md) —
  install resolution, selector, JSON report, and HTTP receiver
- [`docs/specs/tui/`](docs/specs/tui/) — host adapter, view state, and testing
