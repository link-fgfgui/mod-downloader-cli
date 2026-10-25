# TUI Specifications (`docs/specs/tui/`)

> 状态：旧 `tui` 包已按 [tview 迁移计划](tview-migration.md) 删除；当前唯一
> TUI 位于 `internal/tviewui`（Host 位于 `internal/tuihost`），测试见
> `internal/tviewui/*_test.go`。`testing.md` 中的 `tui/`、`tea.Cmd` 等描述仅作
> 历史参考。

Specifications for the native terminal client (`internal/tviewui` + `internal/tuihost`)
that mirrors the Wails GUI's behaviour while sharing the same `./core` submodule.

- [Host Adapter](host-adapter.md): host lifecycle, event bridge, native-dialog
  replacement, clipboard/completion feedback, logging and build injection.
- [View State Layer](view-state.md): the pure `internal/uistate` controllers that
  replace the Vue/Pinia stores, plus the tview shell composition contract.

## Related specifications (inherited from the core/GUI repositories)

- `docs/specs/core/backend/` — core domain rules reused verbatim by the TUI.
- The GUI repository's `docs/specs/app/frontend/ui-state-lifecycle.md`,
  `download-queue-actions.md` and `language-preference.md` describe the source
  behaviour that this layer re-implements.
