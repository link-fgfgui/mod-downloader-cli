# TUI Specifications (`docs/specs/tui/`)

Specifications for the native terminal client (`tui/`, package `tui`) that mirrors
the Wails GUI's behaviour 100% while sharing the same `./core` submodule.

- [Host Adapter](host-adapter.md): `Host` lifecycle, event bridge, native-dialog
  replacement, clipboard/BEL adapters, logging and build injection.
- [View State Layer](view-state.md): the pure `internal/uistate` controllers that
  replace the Vue/Pinia stores, plus the shell composition contract.

## Related specifications (inherited from the core/GUI repositories)

- `docs/specs/core/backend/` — core domain rules reused verbatim by the TUI.
- The GUI repository's `docs/specs/app/frontend/ui-state-lifecycle.md`,
  `download-queue-actions.md` and `language-preference.md` describe the source
  behaviour that this layer re-implements.
