# install / i Target Resolution Contract

`install` and `i` are declared as an exact alias pair (`Aliases: []string{"i"}` in
`cliapp/app.go`). Both share argument parsing, `--project` compatibility, trailing
flag re-parsing (`cliapp/flags.go`) and every error message.

## 1. Parser (`cliapp/target.go`)

`ParseInstallTarget(raw, defaultPlatform) (InstallTarget, error)` delegates prefix
parsing to the shared `models.ParseProjectKey`, so platform prefixes are
case-insensitive. `InstallTarget{Raw, Platform, Query, IsID}` where `IsID` marks a
bare numeric identifier (CurseForge project/file IDs).

`ParseInstallTargets(refs, defaultPlatform)` validates, then parses every
positional argument. Zero targets produce `emptyTargetsMessage`, which lists
example invocations.

## 2. Interceptor (`ValidateProjectTarget`)

Rejects, with an actionable multi-line message:

- explicit `version:`, `version-id:`, `file:`, `file-id:` prefixes;
- CurseForge file IDs: pure-numeric identifiers of **7+ digits**.

**Deliberate exclusion:** a bare 8-character Base62 string is *not* rejected.
Modrinth project IDs and version IDs share that shape — the spec's own example
project ID `AANobbMI` is 8 characters — so a static rule would reject valid
projects. This deviation is documented in code, README and here.

`versionIDExclusivity(targetCount, versionID)` rejects `--version-id` combined
with multiple targets (a single version ID cannot apply to several projects).

## 3. Decision state machine (`cliapp/resolver.go`)

`resolveInstallTarget` returns `[]models.ModProject` (1 for exact/auto picks, N for
a multi-select search pick):

1. **Platforms** — an explicit prefix locks one platform; otherwise `--platform`
   acts as the filter; otherwise both platforms are queried concurrently.
2. **0 exact hits + 1 hit** → return it.
3. **>1 exact hits** → interactive disambiguation; non-interactively the
   `--platform` candidate wins, else Modrinth, with a stderr notice.
4. **0 exact hits** → keyword search fallback (`SearchModsTextCollect`, top 10 by
   downloads). One result auto-selects; multiple results open the selector;
   non-interactive mode returns a hard error instructing the user to pass an exact
   slug or prefixed ID (never hangs).

Platform ordering is deterministic (`platformRank`: Modrinth → CurseForge).

## 4. Async metadata pipeline (Phase 3)

`buildInstallPlanEntry` (CLI) and `enrichCmd` (selector) resolve, per candidate:

- **Step A** `listMatchingVersionsFn` → preferred version for the active tuple.
- **Step B** `resolveModIDs` → `modbridge.VersionModIDs` (cache + HTTP range
  probe, persisted by the core), falling back to the version's parsed IDs, then
  the project slug.
- **Step C** `localStateFor` → `modbridge.LocalModPathsForModIDs` against the
  target dir instance.

Network-facing calls sit behind package variables (`*Fn` seams, mirroring
`resolveProjectsByHashesFn` in `update.go`) so tests inject deterministic fakes.

## 5. Selector (`cliapp/selector.go`)

`InteractiveSelectModel` runs inline (no alt-screen). Keys: `↑/k`, `↓/j`, `1`–`9`,
`Space` (multi-select), `Enter` (confirm), `q`/`Esc` (skip), `Ctrl+C` (abort).
Metadata arrives as `metadataEnrichedMsg` and refreshes the row in place; a braille
`selectorSpinnerMsg` tick animates pending rows. On exit the frame is erased via
`clearSelectorFrame` and a single `✔ Selected: [Platform] Title (modid: …, v…)`
line remains.

## 6. Plan, dry-run and JSON (Phase 5)

After resolution the command prints a `TARGET / PLATFORM / MOD_ID / TITLE /
VERSION / SIZE` table (`writeInstallPlan`). `--dry-run` stops there, printing
`dry-run: no files were downloaded or written.` (and emitting the unified report
under `--json`); otherwise the installs run.

### 6.1 Live progress (Phase 5.2)

Each install runs on a background goroutine while `installWithProgress` polls
`GetDownloadQueueState()` every 200 ms and overwrites a single line:

```
[████████░░░░░░░░░░░░░░░░]  42%  1.23 MiB/s  ·  2 任务进行中
```

The line is erased (`\r\x1b[K`) when the install finishes. Progress is skipped
entirely under `--json` so machine-readable output stays clean.

### 6.2 Unified JSON report (Phase 5.3)

`--json` emits one object for both dry-run and executed installs:

```json
{ "targets": [...], "resolved": [...], "skipped": [{ "target": ..., "reason": ... }], "errors": [...] }
```

Failures/skips no longer abort the batch: every target is accounted for, then the
command exits non-zero with the joined reasons when any target failed.

## 7. Per-candidate metadata timeout (Phase 3.3)

`metadataTimeout` (3 s, a variable so tests can shorten it) bounds every metadata
probe, in both the CLI plan builder and the selector worker:

- `preferredVersionWithin` runs the version lookup under a deadline and reports
  `versionFound` / `versionMissing` / `versionTimeout`.
- On timeout the candidate degrades gracefully: `ModIDs` fall back to the project
  slug, `LocalState` becomes `metadata_timeout`, and the selector renders the slug
  with a `metadata_timeout` badge instead of a spinner. A slow provider therefore
  never blocks the other candidates or the user's `Enter` confirmation.
