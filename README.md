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

## Quick Start

Open an instance's `mods` directory:

```sh
cd /path/to/minecraft-instance/versions/fabric-1.21.1/mods
```

Search for a mod:

```sh
mod-downloader-cli search sodium
```

Show compatible versions for a project:

```sh
mod-downloader-cli show modrinth:sodium
```

Install a mod from inside an instance's `mods` directory:

```sh
mod-downloader-cli install modrinth:sodium
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
| `--json` | - | Writes structured JSON output |

The CLI does not read or create `mod-downloader.toml`. Runtime settings are
read from flags and environment variables only.

When the current directory is named `mods`, the CLI also tries to infer missing
runtime settings from the parent directory (`..`). It looks for a Minecraft
version manifest such as `../fabric-1.21.1.json` and reuses the core manifest
parser to detect the Minecraft version and loader. Explicit flags and
environment variables always take precedence.

If automatic detection is not possible, pass the settings explicitly:

```sh
mod-downloader-cli --mc-version 1.21.1 --loader fabric search sodium
```

## Commands

### `search`

Searches providers for mods compatible with the selected Minecraft version and
loader.

```sh
mod-downloader-cli search [--platform modrinth|curseforge] [--limit 10] [--offset 0] <query>
```

You can also pass the query with `--query`.

Human output columns:

```text
ID  PLATFORM  TITLE  SLUG
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

### `install`

Installs one or more projects into the current directory.

```sh
mod-downloader-cli install [--version-id <id>] [--timeout 2m] <project...>
```

By default the current directory must be named `mods`. This is intentional so
downloads do not accidentally land in the wrong folder. Pass `--force` to allow
installing into another current directory.

Examples:

```sh
mod-downloader-cli install modrinth:sodium
mod-downloader-cli install --platform modrinth sodium lithium
mod-downloader-cli install --version-id <version-id> modrinth:sodium
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
