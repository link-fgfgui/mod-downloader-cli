# mod-downloader-cli

`mod-downloader-cli` is a command-line Minecraft mod downloader. It searches
Modrinth and CurseForge metadata, shows compatible mod versions, installs mods
into a `mods` directory, lists local mods, removes local mod files, and manages
the metadata cache.

Most service and domain logic lives in `mod-downloader-core`, which is vendored
in this repository as the `core` module through a local `replace` directive.

## Requirements

- Go 1.24 or newer
- Network access for searching and downloading mods
- A CurseForge API key if you want CurseForge results

Modrinth works without an API key. CurseForge is only enabled when
`--curseforge-api-key` or `CF_API_KEY` is provided.

## Build

From the repository root:

```sh
go build ./cmd/mod-downloader-cli
```

This creates a `mod-downloader-cli` binary in the current directory.

For local development you can also run the CLI directly:

```sh
go run ./cmd/mod-downloader-cli --help
```

## Logging

The CLI is quiet by default: only warnings and errors reach stderr. Pass
`-v` / `--verbose` (before or after the subcommand) to enable debug logging.

The full-screen TUI treats the terminal as unavailable for logs, so log lines
never corrupt the screen. It writes info-level records to `mod-downloader.log`
in the working directory; debug logging stays off. Set
`[logging] enabled=false` in `mod-downloader.toml` to disable the file entirely.

## Quick Start

Open an instance's `mods` directory:

```sh
cd /path/to/.minecraft/versions/fabric-1.21.1/mods
```

Search for a mod:

```sh
mod-downloader-cli search sodium
```

Show compatible versions for a project:

```sh
mod-downloader-cli show modrinth:sodium
```

Launch the full-screen TUI (no subcommand, or `tui`):

```sh
mod-downloader-cli
mod-downloader-cli tui
```

The TUI mirrors the desktop app: a left navigation sidebar (Home / Download /
Manage / Favorites / Unpin / Settings), the instance and target selector at the
bottom-left, a single status bar, and a download-queue dock on the right. The
sidebar collapses to icons between 50 and 119 columns and is hidden below 50
columns; `1`-`6` still switch views. `t` opens the instance selector, `q` toggles
the queue dock, `?` shows the key map, and `Esc` unwinds dialogs, selections and
navigation in that order. `Tab` moves focus between a page's search field, list
and panes; the queue dock uses `Tab`/arrow keys to switch between its Downloads
and Optional tabs. Lists, tables, buttons and the sidebar support the mouse:
click a row to select it, click a button to activate it, and use the wheel to
scroll. Dialogs are centered tview pages; click their buttons but press `Esc` to
close them (clicking outside does not dismiss a dialog).

The file picker is a path input rather than a tree: type a path (with `~`,
`$HOME` or `$PWD` expansion) and press `Tab` once for the common prefix, again
to list candidates; `Ctrl+S` or `Enter` confirms. Invalid directories are
reported inside the dialog so it stays open.

| Key | Action |
| --- | --- |
| `1`-`6` | Switch view (Home, Download, Manage, Favorites, Unpin, Settings) |
| `/` | Focus the Download or Manage search field |
| `↑`/`↓`, `PgUp`/`PgDn`, `Home`/`End` | Move the list or table cursor |
| `Space` | Toggle the current row's multi-selection |
| `Enter` | Open/activate the current row |
| `e`/`d`/`i` | Manage: enable, disable, invert selection |
| `x` | Delete the selection (Manage) or remove favorites |
| `S` | Manage: scan unused dependencies |
| `v` | Open the version list for the current mod |
| `f` | Add the selection to favorites |
| `r` | Refresh the current page |
| `q` / `Q` | Toggle the queue dock / clear optional-dependency reminders |
| `t` / `?` | Instance and target selector / keyboard help |
| `Esc` | Close dialog, clear selection, then return Home |
| `Ctrl+C` | Quit |

Settings are saved as soon as a control changes (language, theme, release
variant, cleanup, cache/Minecraft directory, MCIM, network fields and API
keys); the language switch rebuilds every widget even inside the form.

Install a mod from inside an instance's `mods` directory:

```sh
mod-downloader-cli install modrinth:sodium
mod-downloader-cli i sodium            # `i` is an exact alias of `install`
```

List installed mods:

```sh
mod-downloader-cli list
```

Remove an installed mod:

```sh
mod-downloader-cli remove sodium
```

## Runtime Options

Global flags can be placed before the command:

```sh
mod-downloader-cli --mc-version 1.21.1 --loader fabric search sodium
```

| Flag | Environment variable | Description |
| --- | --- | --- |
| `--mc-version`, `--minecraft-version`, `--version` | `MINECRAFT_VERSION` | Minecraft version used for search, show, and install |
| `--loader` | `MOD_LOADER` | Mod loader: `fabric`, `forge`, or `neoforge` |
| `--cache-dir` | `MOD_DOWNLOADER_CACHE_DIR` | Directory that stores `mods.gob.zst` metadata cache |
| `--curseforge-api-key` | `CF_API_KEY` | Enables CurseForge provider access |
| `--modrinth-api-key` | `MODRINTH_API_KEY` | Reserved Modrinth API key override |
| `--dir`, `-C` | `MOD_DOWNLOADER_DIR` | Target mods directory, or its parent version / instance directory |
| `--json` | - | Writes structured JSON output |

The CLI does not read or create `mod-downloader.toml`. Persistent settings are
stored by the [`config`](#config) command under the operating system's standard
user configuration directory.

Setting precedence, highest first: command-line flags, environment variables,
the `config` file, then automatic detection.

### Automatic Version Detection

When the current directory is named `mods`, `--mc-version` and `--loader` are
optional if the CLI can infer them from the instance path.

Detection reuses the same core launcher scanner used by the GUI:

1. Standard launcher layout:

   ```text
   .minecraft/versions/<version-id>/mods
   ```

2. Prism Launcher layout with a `.minecraft` game directory:

   ```text
   instances/<instance>/.minecraft/versions/<version-id>/mods
   ```

3. Prism Launcher layout using the instance root as the game directory:

   ```text
   instances/<instance>/versions/<version-id>/mods
   ```

If launcher-root detection does not find a matching instance, the CLI falls
back to reading a version manifest directly from the parent directory, for
example `../fabric-1.21.1.json`.

Explicit flags and environment variables always take precedence over automatic
detection.

If automatic detection is not possible, pass the settings explicitly:

```sh
mod-downloader-cli --mc-version 1.21.1 --loader fabric search sodium
```

### Targeting Another Directory

Use `--dir` / `-C` to operate on a mods directory without changing the current
directory. The value may be the `mods` folder itself, a version directory that
contains `mods/`, or a Minecraft game / Prism instance directory:

```sh
mod-downloader-cli --dir /path/to/.minecraft/versions/fabric-1.21.1 list
mod-downloader-cli -C /path/to/instance/mods install modrinth:sodium
```

When an instance directory is given, the CLI detects the mods folder and infers
the Minecraft version and loader from it.

## Commands

### `search`

Searches providers for mods compatible with the selected Minecraft version and
loader.

```sh
mod-downloader-cli search [--platform modrinth|curseforge] [--limit 10] [--offset 0] <query>
mod-downloader-cli search --slug <slug>
mod-downloader-cli search --id <project-id>
```

You can also pass the query with `--query`. Query searches are regular text
searches only; use `--slug` or `--id` when you want an exact project match.

Human output columns:

```text
ID  SLUG  TITLE  PLATFORM
```

Use project references from the `ID` column, such as `modrinth:sodium`, with
`show` and `install`.

### `show`

Shows matching project versions for a project ID, slug, or platform-prefixed
reference.

```sh
mod-downloader-cli show modrinth:sodium
mod-downloader-cli show --platform modrinth sodium
```

Human output includes project metadata followed by version rows:

```text
VERSION_ID  NAME  FILE  DOWNLOADS
```

### `install` / `i`

Installs one or more **projects** into the current directory. `install` and `i`
are exact aliases: argument parsing, flags, environment variables and error
messages are identical for both spellings.

```sh
mod-downloader-cli install [flags] <project...>
mod-downloader-cli i       [flags] <project...>
```

With no project arguments on a terminal, `install` starts an interactive wizard:
confirm the instance/target, type a search term, pick one or more results with
`Space`, then confirm the plan. `Esc` cancels with exit code 130. The wizard is
skipped for `--json`, `--dry-run`, `--yes` and `--no-input`.

By default the current directory must be named `mods`. This is intentional so
downloads do not accidentally land in the wrong folder. Pass `--force` to allow
installing into another current directory.

**Accepted project references**

| Form | Example | Behaviour |
| :--- | :--- | :--- |
| Slug | `sodium` | Exact lookup across every enabled platform (Modrinth + CurseForge) |
| Project ID | `AANobbMI` (Modrinth) / `394468` (CurseForge) | ID-shape aware lookup, cross-platform |
| Platform prefix | `modrinth:sodium`, `curseforge:394468` | Locks the lookup to that platform |

**Version and file identifiers are rejected.** `install` / `i` declares *which
projects* to bring into the instance; the concrete version is derived from the
active `[MinecraftVersion \| ModLoader]` plus the preferred release variant.
Passing a `version:` / `version-id:` / `file:` / `file-id:` reference, or a
CurseForge file ID (a 7+ digit numeric value), fails fast with actionable
guidance instead of silently bypassing version resolution.

> Note: a bare 8-character Base62 string is treated as a *project ID*, because
> Modrinth project IDs and version IDs share that exact shape (e.g. `AANobbMI`).
> Use the explicit `version-id:` prefix if you really mean a version.

**Resolution pipeline**

1. Exact lookup on the prefixed platform, or concurrently across both platforms.
2. A single hit installs directly; multiple hits open a create-vite-style
   `Select` prompt (TTY only) unless `--platform` resolves the ambiguity, in
   which case the filter wins, and otherwise Modrinth is preferred with a
   stderr notice.
3. No exact hit falls back to a keyword search; results are offered in a
   filterable multi-select prompt. In non-interactive mode this is a hard error
   (never a hang) telling you to pass an exact slug or prefixed ID.
4. Downloads render a single live line with an overall progress bar, the current
   file, `i/N`, and speed. Non-TTY output stays plain; `--json` stays silent.

**Flags**

| Flag | Purpose |
| :--- | :--- |
| `--platform <name>` | Disambiguate to `modrinth` or `curseforge` |
| `-y`, `--yes` | Non-interactive: accept automatic choices |
| `--no-input` | Non-interactive: never open the selector |
| `--dry-run` | Print the resolved plan only; no network writes |
| `--json` | Structured report (`targets`, `resolved`, …) |
| `--version-id <id>` | Advanced: single-project explicit version |
| `--timeout 2m` | Optional overall install timeout |
| `--force` | Allow running outside a directory named `mods` |

Examples:

```sh
mod-downloader-cli i sodium
mod-downloader-cli i iris fabric-api          # multiple targets
mod-downloader-cli i sodium --dry-run          # trailing flags are honoured
mod-downloader-cli install --platform modrinth sodium lithium
mod-downloader-cli i "mini map" --no-input     # fails fast, no hang
mod-downloader-cli i curseforge:394468 modrinth:AANobbMI
```

When a compatible version has required dependencies, the downloader queues the
missing required dependencies as well.

### `list` / `mods`

Lists local mods in the current directory.

```sh
mod-downloader-cli list
mod-downloader-cli mods --json
```

By default the current directory must be named `mods`. Pass `--force` to scan a
different current directory.

Human output columns:

```text
ID  NAME  VERSION  FILE  ENABLED
```

### `remove`

Removes installed mod files from the current directory.

```sh
mod-downloader-cli remove [--dry-run] <mod...>
```

Targets are matched case-insensitively against local mod ID, name, file name, or
file name without `.jar` / `.disabled`.

Preview removals without deleting files:

```sh
mod-downloader-cli remove --dry-run sodium
```

By default the current directory must be named `mods`. Pass `--force` to remove
from a different current directory.

### `cache clean`

Deletes the metadata cache file.

```sh
mod-downloader-cli cache clean
```

If `--cache-dir` is set, the CLI removes `<cache-dir>/mods.gob.zst`. Otherwise
it removes the default cache file under the system temp directory:
`<temp>/mod-downloader/mods.gob.zst`.

### `enable` / `disable`

Enable or disable installed mods by renaming files using the community
convention: `disable` renames `<file>.jar` to `<file>.jar.disabled`, and
`enable` renames it back.

```sh
mod-downloader-cli disable [--dry-run] <mod...>
mod-downloader-cli enable [--dry-run] <mod...>
```

Targets match case-insensitively against mod ID, name, file name, or the file
name without `.jar` / `.disabled`. Repeating the same operation is a friendly
no-op.

```sh
mod-downloader-cli disable sodium
mod-downloader-cli enable --dry-run sodium
```

### `check-update` / `outdated`

Check installed mods against the latest compatible versions.

```sh
mod-downloader-cli check-update [--platform modrinth|curseforge]
```

Human output columns:

```text
ID  NAME  CURRENT_VERSION  LATEST_VERSION  PLATFORM
```

When every installed mod is already current, the command prints
`all mods are up to date`.

### `upgrade`

Download the latest compatible versions and remove the superseded jars.

```sh
mod-downloader-cli upgrade [--all] [--dry-run] [--timeout 2m] [<mod...>]
```

Pass at least one mod name, or `--all` to upgrade every mod with a newer
version. The old version is removed only after the new download succeeds, and a
disabled mod stays disabled after the upgrade. Use `--dry-run` to preview the
downloads and deletions.

### `verify` / `check`

Check installed mods for missing dependencies, duplicate mod IDs, and loader /
Minecraft version incompatibilities.

```sh
mod-downloader-cli verify [--strict]
```

Missing dependencies are reported with a suggested install command. The command
exits with code `1` when blocking issues (missing dependencies or duplicate IDs)
are found. `--strict` also fails on compatibility warnings.

### `info` / `inspect`

Show detailed metadata for an installed mod or a local jar file.

```sh
mod-downloader-cli info <mod-or-jar-file>
mod-downloader-cli info sodium
mod-downloader-cli info ./some-mod.jar
```

Output includes base metadata, supported Minecraft version and loader,
dependencies (depends / recommends / breaks), file path, size, SHA1 and SHA512
hashes, and any online project association.

### `export`

Export installed mods to a portable requirements-style manifest.

```sh
mod-downloader-cli export [-o mods.txt]
```

Each line is `platform:project[==version]`. Mods that cannot be matched to a
platform are listed as `# local: <file>` comments. The output defaults to
stdout; `-o` / `--output` writes it to a file.

### `import`

Install mods listed in a manifest file.

```sh
mod-downloader-cli import [--skip-existing] <file>
```

Blank lines and `#` comments are ignored. A line with an explicit version
(`==1.2.3`) installs that version; otherwise the latest compatible version is
installed. A failing entry does not stop the remaining entries; a summary of
installed / skipped / failed counts is printed at the end.

### `config`

Manage persistent user configuration.

```sh
mod-downloader-cli config get <key>
mod-downloader-cli config set <key> <value>
mod-downloader-cli config unset <key>
mod-downloader-cli config list [--show-secrets]
mod-downloader-cli config path
```

Configuration is stored under the operating system's standard user config
directory and never in the current working directory:

```text
Linux:   ~/.config/mod-downloader-cli/config.json
macOS:   ~/Library/Application Support/mod-downloader-cli/config.json
Windows: %APPDATA%\mod-downloader-cli\config.json
```

Supported keys:

| Key | Description |
| --- | --- |
| `mc-version` | Default Minecraft version |
| `loader` | Default loader: `fabric`, `forge`, or `neoforge` |
| `curseforge-api-key` | CurseForge API key |
| `modrinth-api-key` | Modrinth API key |
| `cache-dir` | Custom metadata cache directory |
| `minecraft-dir` | Default Minecraft root / instance directory |

```sh
mod-downloader-cli config set loader fabric
mod-downloader-cli config set mc-version 1.21.1
mod-downloader-cli config list
```

Secret values are masked in `config get` / `config list` (for example
`****abcd`); pass `--show-secrets` to print them in plain text.

## JSON Output

Every command supports `--json`. For example:

```sh
mod-downloader-cli --mc-version 1.21.1 --loader fabric search sodium --json
```

JSON output is indented and uses the same data structures returned by the core
service.

## Development

Run all tests:

```sh
go test ./...
```

Run CLI tests only:

```sh
go test ./cliapp ./cmd/mod-downloader-cli
```

Run core tests:

```sh
cd core
go test ./...
```
