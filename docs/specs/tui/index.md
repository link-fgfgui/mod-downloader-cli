# TUI Specifications

The only TUI is `internal/tviewui`; the shared host is `core/host`. Tests live in
`internal/tviewui/*_test.go` and `internal/uistate/*_test.go`.

- [Host Adapter](host-adapter.md): host lifecycle, event bridge, native-dialog
  replacement, clipboard/completion feedback, logging, and build injection.
- [View State](view-state.md): the pure `internal/uistate` controllers and the
  tview shell composition contract.
- [Testing](testing.md): test layers, harnesses, coverage gates, and the
  regression list.

Core domain rules reused by the TUI are specified in the GUI repository:
[`docs/specs/core/`](../../../../mod-downloader-gui-v3/docs/specs/core/).
