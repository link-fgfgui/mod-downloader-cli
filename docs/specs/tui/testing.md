# TUI 测试方案（tview）

> 状态：现行。终端界面已迁移到 tview，旧 Bubble Tea 的 `tui/`、`tea.Cmd`、
> `AppModel.Update` 描述已移除。
>
> 测试代码位置：
>
> - 界面：`internal/tviewui/*_test.go`，golden 在 `internal/tviewui/testdata/frames/`
> - 纯状态：`internal/uistate/uistate_test.go`、`uistate_extra_test.go`
> - 宿主与事件映射：`internal/tuihost/host_test.go`、`host_lifecycle_test.go`
> - 真实事件循环（注入键鼠）：`internal/tviewui/input_loop_test.go`
> - 离线控制器契约：`internal/tviewui/controller_offline_test.go`
> - 真实 API（env 门控）：`internal/tviewui/live_api_test.go`
> - 真实终端：`scripts/tui-smoke.sh`（tmux PTY）
> - 国际化：`internal/i18n/i18n_test.go`

## 1. 测试分层

### L0 纯函数与组件（不启动终端）

| 文件 | 覆盖内容 |
| :--- | :--- |
| `format_extra_test.go` | 速度/百分比/紧凑数字/进度条、CJK 安全的 `truncate`、复选框 |
| `chrome_theme_extra_test.go` | `uiText` 中英标签、chrome 字典双语完整性、主题解析与颜色标签 |
| `status_mouse_extra_test.go` | 状态栏 hint 裁剪、队列胶囊宽度、可点击区域、wheelList/Table/Form、`clipScreen`、actionBar 布局 |
| `shell_extra_test.go` | 断点/侧栏宽度、`mainWidth` 夹取、队列胶囊、busy 状态、toast、Esc 回退顺序、视图切换、侧栏 footer |
| `internal/uistate/*_test.go` | 搜索分页与竞态、队列铃声/展示、Manage 分组、收藏 scope、设置 clamp、API key 掩码 |
| `internal/i18n/i18n_test.go` | 语言归一化、key 对等、插值与回退 |

### L1 渲染不变量

[`render_test.go`](../../../internal/tviewui/render_test.go)：

- `TestRenderMatrix`：6 个页面 × 5 种尺寸 `50x20 / 80x24 / 100x30 / 120x40 / 160x50`
  × 中英双语，断言行数等于终端高度、每行宽度不超过终端宽度。
- `TestDialogRenderMatrix`：confirm / prompt / select / path / target / help /
  version / favorites / refs / disable-impact 在全部尺寸与语言下不溢出。
- `assertFrameBounds`：所有帧的共同边界断言。

### L2 Golden 快照

- `TestGoldenPages` 生成 6 页 × 5 尺寸 × 2 语言，共 60 个文件。
- 更新：`go test ./internal/tviewui -run TestGoldenPages -update`。
- 每个 golden diff 必须人工 review，不能直接提交 `-update` 的结果。
- golden 只保存屏幕文本，不断言具体颜色值。

### L3 键盘、焦点与弹窗

- [`shell_test.go`](../../../internal/tviewui/shell_test.go)：数字键、输入框吞键、
  Tab 焦点循环、多选、兼容性折叠、语言切换、弹窗栈、设置顺序。
- [`shell_extra_test.go`](../../../internal/tviewui/shell_extra_test.go)：关闭的下拉框不吞
  全局键、打开的下拉框接管输入、Esc 逐层回退、视图切换关闭队列并恢复焦点。
- [`dialogs_extra_test.go`](../../../internal/tviewui/dialogs_extra_test.go)：
  - confirm：长文本滚动、PgUp/PgDn/↑↓、左右/Backtab 切换、Esc 等价取消；
  - prompt：校验失败保持打开、去空格、取消不提交；
  - select：禁用项不可选、Esc 关闭；
  - path：目录/保存模式、`.zip` 补全、非法路径、无补全、候选截断、`~`/`$HOME`；
  - target：目录/实例/刷新/MC 版本/loader 行；
  - version：加载/空/已安装/固定标记、pin 与 replace 两条路径；
  - favorites：列表选择、嵌套新建、缺 scope 提示、引用渲染；
  - disable-impact：缓存缺失提示、多候选来源选择、恢复完成标记；
  - 弹窗栈：同名去重、仅关最上层、中间层移除、焦点恢复；
  - centered：小终端下的尺寸夹取与遮罩点击。

### L4 鼠标

[`mouse_test.go`](../../../internal/tviewui/mouse_test.go)、
[`list_click_test.go`](../../../internal/tviewui/list_click_test.go) 与
[`status_mouse_extra_test.go`](../../../internal/tviewui/status_mouse_extra_test.go)：

- 滚轮滚动结果列表、设置表单、pin 表格；滚动后的行坐标仍命中正确行。
- 多选列表点击语义：左侧复选框切换该项多选；单击标题文字只选中该行；灰色描述行、
  状态图标、标题后的空白一律视为未命中。标题命中区按实际渲染宽度计算（含 CJK、
  横向滚动 offset）。
- 点击行切换复选、点击 action bar、点击侧栏、双击降级为单击。
- 状态栏 hint 区域可点击；右键队列胶囊清空提醒。
- 弹窗打开时背景点击被拦截，点边框不关闭也不穿透。
- 主题切换后列表/文字/侧栏背景使用新调色板。

### L5 事件与异步

- [`events_extra_test.go`](../../../internal/tviewui/events_extra_test.go)：`handleEvent`
  的 8 类事件、错误 payload、stale 搜索请求、扩展结果、状态栏刷新、事件压缩。
- [`lifecycle_test.go`](../../../internal/tviewui/lifecycle_test.go)：真实 tview 事件循环
  启动/停止、事件泵批处理、busy spinner 上屏、`goleak` 无残留 goroutine。
- `internal/uistate`：request-id 竞态丢弃、分页 append、队列 membership diff、
  铃声每周期只响一次。

### L5.5 真实事件循环与键鼠注入

[`input_loop_test.go`](../../../internal/tviewui/input_loop_test.go) 通过
`SimulationScreen.InjectKey` / `InjectMouse` 驱动**真实的** tview 事件循环与输入捕获，
不走离线 harness 的快捷路径：

- 1–6 导航；`?` 帮助、`t` 目标弹窗、Esc 分层关闭；
- `/` 聚焦搜索、输入数字/空格不会触发导航、Tab/Backtab 在搜索与列表间切换；
- 侧栏鼠标点击（按下+释放，走 tview 的 click 判定）；
- 运行中缩放 `60x20 → 40x14 → 120x40`，含弹窗打开时的窄屏重排；
- 设置页主题/语言下拉框的键盘打开、选择、表单重建；
- Tab/Backtab 跳过只读分区标题与只读行，始终落在可交互控件上；
- 下拉框打开时 Esc 只关闭下拉框（回归：曾导致 UI goroutine 死锁）；
- 队列 dock 打开/聚焦/Esc 关闭；
- 500 条队列事件风暴后仍能响应导航。

`controller_offline_test.go` 锁定离线 harness 的 DAAU 契约：所有控制器入口在
`Service()==nil` 时必须安全返回，不能 nil panic。

### L6 页面业务流

[`pages_extra_test.go`](../../../internal/tviewui/pages_extra_test.go)：

- Download：搜索/追加、安装确认、metadata 重试、批量安装、队列行状态、版本弹窗、
  收藏入口、目录选择、光标触底加载。
- Manage：分组渲染、全部/启用/禁用过滤、搜索、Space/Ctrl+A、兼容性折叠、
  删除确认、未使用依赖扫描、收藏与版本入口保护。
- Favorites：双栏布局、窄屏切换、列表排序、item 多选、所有列表级快捷键、
  跨版本输入校验、快照转搜索结果、草稿映射。
- Unpin：表格渲染、选中行、清空态、单条/全部取消。
- Settings：快照回填、密钥掩码、解析工具、ClearFocus、语言切换重建。
- Home：不同宽度下的指标卡布局与快捷键。
- Download 动作栏：安装当前行、批量下载空选择、全选、收藏入口（光标回退与多选）、
  目标选择弹窗（`TestDownloadActionBarCallbacks`）。

[`controller_live_test.go`](../../../internal/tviewui/controller_live_test.go) 与
[`controller_live_more_test.go`](../../../internal/tviewui/controller_live_more_test.go)
使用真实 `tuihost.Host` + `appcore.Service`，在 SimulationScreen 上驱动控制器：

- 设置保存：主题/语言/精简/清理/MCIM/release/网络/缓存目录/API key；
- 收藏：创建、重命名、固定、复制、图标、引用增删、页面级移除、导出失败提示；
- 收藏页面全部弹窗：prompt 新建/重命名（空名拒绝）、列表复制、引用添加、
  图标 mdi/project/clear、条目复制、转搜索结果、删除取消/确认、
  add-favorite 内联新建列表；
- 跨版本复制：用本地缓存的 project + 版本快照离线解析成功、目标名称冲突、
  未知平台冲突、忽略冲突但无匹配；
- 固定版本：pin/refresh/batchUnpin/unpin/unpinAll；
- 队列：非法下载请求、cancel/retry/remove/reminder/optional 动作、下载状态回填；
- 实例：目录切换、选择实例、刷新 mods、Connector 错误、无效版本；
- Manage：页面级批量操作失败提示、未使用依赖扫描、`enterRow` 折叠/批量/越界；
- 版本弹窗：未知平台返回空列表时的 loading 收尾与占位行；
- 禁用影响弹窗的单候选恢复路径（缓存拒绝也不卡在 restoring）；
- `Quit` 后事件循环退出。

约束：

- 一个进程只能运行一个 `appcore.Service`，所以 live 测试不能 `t.Parallel`。
- 每个 live 测试必须通过 `t.Cleanup` 调用 `Quit` 并等待退出。
- 控制器方法通过 `doUI` 在 UI goroutine 上调用，页面状态通过 `waitFor` / `waitUI`
  在 UI goroutine 上读取，避免竞态。

### L7 进程级、真实 API 与真实终端

- [`run_test.go`](../../../internal/tviewui/run_test.go)：完整 `runWith` 启动、
  `startupDone`、Ctrl+C 退出、stderr 为空。
- `TestApplicationLifecycleNoLeaks` / `TestEventPumpAppliesBatchedEvents`：`goleak`。
- [`live_api_test.go`](../../../internal/tviewui/live_api_test.go)：默认跳过，设置
  `MOD_DOWNLOADER_LIVE_API=1` 后执行：
  - Mojang release manifest 冷启动（`LoadMinecraftReleases=true`）；
  - 真实 Modrinth 搜索（键盘路径）并打开版本弹窗；
  - 真实下载第一个结果，断言 jar 落到临时实例的 `mods/`，队列最终收敛。
- `scripts/tui-smoke.sh`：构建真实二进制，在 tmux PTY 中启动、按键导航、打开帮助、
  缩放 100x30→60x20→120x40、Ctrl+C 退出并检查无超宽帧。
- 公开入口 `Run` 需要真实 TTY；由上面的 tmux 冒烟覆盖。

```sh
make test-live-api   # 真实网络
make test-tui-smoke  # 真实终端
```

## 1.5 DAAU（Developer As A User）契约

把内部开发者视为 TUI/Host API 的用户，以下行为由测试锁定：

- `tuihost.Host.Start` 串行且幂等：重复调用不会替换或泄漏 core/HTTP server；失败后
  `Service()`/`Core()` 仍为 nil，可重试。`Shutdown` 可重复调用。
- `App` 离线模式：未启动 core 时，所有控制器方法必须安全 no-op；新增控制器方法必须
  加入 `TestOfflineControllersAreNoOps`。
- `App.service()` 是唯一的核心入口；不要在控制器里直接 `host.Service()` 绕过离线守卫。
- 页面可选接口（`pageActivator`、`pageResizer`、`pageRestyler`、`pageClearer`、
  `pageDropdowns`）有各自的事件/焦点契约；新增实现必须补对应测试。
- core 事件契约变更必须同步更新 `tuihost.emitCoreEvent`、映射测试与
  `host-adapter.md`；未知事件/错误 payload 记录 Warn，不静默丢弃。
- 真实输入用例走 `Application` 事件循环；离线 `testApp` 只用于纯渲染/焦点/命中测试。
- 屏幕断言统一通过 `screenText`（跳过宽字符续格）。

## 2. 测试工具

### 2.1 离线 `testApp`（`harness_test.go`）

```go
h := newTestApp(t, 100, 30) // 不启动 core，控制器方法为 no-op
h.draw()
h.rune('3'); h.key(tcell.KeyEsc)
h.mustContain("Local Mod Management")
h.switchPage(ViewSettings)
```

- `sendKey` 同时经过 `handleGlobalKey` 与焦点控件，和真实事件路径一致。
- `text()` 从 SimulationScreen 取字符，跳过宽字符的续格。
- 控制器调用依赖 `a.offline` 守卫；新增控制器方法时必须保留该守卫。

### 2.2 在线 `liveApp`（`controller_live_test.go`）

```go
l := newLiveApp(t)                 // 临时 HOME/XDG + SimulationScreen + 真实 core
l.doUI(func() { l.app.saveTheme(ThemeLight) })
l.waitFor("theme", func(a *App) bool { return a.theme.Name == ThemeLight })
l.pressRune('3')                   // 真实 InjectKey，走 Application 输入捕获
l.waitFor("manage", func(a *App) bool { return a.active == ViewManage })
l.resize(60, 20)                   // SetSize + QueueUpdateDraw
l.click(x, y)                      // 按下 + 释放，触发 tview 的 MouseLeftClick
```

- `doUI`：在 UI goroutine 上执行控制器/页面方法。
- `waitUI`：把断言排到所有待处理事件之后。
- `waitFor`：在 UI goroutine 上轮询条件，超时失败；不使用裸 `sleep`。
- `waitForTimeout`：用于真实网络流程的显式长超时。
- `press*` / `typeText` / `click` / `resize`：真实输入注入；点击必须发送按下+释放，
  `SetSize` 后必须触发一次 Draw。
- `newLiveAppWith(t, Options{...})`：需要 `LoadMinecraftReleases=true` 等启动选项时使用。
- 屏幕文本统一用 `screenText` 读取，宽字符的续格会被跳过；否则 CJK 文案会带出
  上一帧残留字符，产生伪失败。

### 2.3 Golden helper

```go
go test ./internal/tviewui -run TestGoldenPages -update
```

更新后检查 `git diff internal/tviewui/testdata/frames`，确认只是预期的布局变化。

## 3. 命令与覆盖率基线

```sh
make test
make test-race
make test-live-api     # 需要网络
make test-tui-smoke    # 需要 tmux
go test ./internal/tviewui -run TestGoldenPages
go test ./internal/tviewui ./internal/uistate ./internal/tuihost ./internal/i18n -cover
```

当前基线（2026-10，`go test ./... -cover`）：

| 包 | 行覆盖率 |
| :--- | ---: |
| `internal/tviewui` | 93.4% |
| `internal/uistate` | 97.7% |
| `internal/tuihost` | 98.1% |
| `internal/i18n` | 100.0% |
| `cliapp/prompt` | 79.6% |

门槛：

- `internal/tviewui` 不低于 80%，`internal/uistate` 不低于 90%。
- 所有 golden diff 人工确认。
- `-race` 失败阻断合并。
- live 测试禁止并行，避免多个 core 同时启动。
- 真实 API / tmux 测试默认不跑，发布前手动执行。

## 4. 已知未覆盖项

这些不是遗漏，而是当前自动化测试不适合默认覆盖的路径，需用真实环境或显式命令验证：

| 路径 | 原因 |
| :--- | :--- |
| `tviewui.Run` / `run` | 需要真实 TTY；由 `make test-tui-smoke` 覆盖 |
| `managePage.copySelection` | 依赖系统剪贴板 |
| 真实 Modrinth/Mojang 网络 | 默认 hermetic；由 `make test-live-api` 覆盖 |
| `tuihost.Start` 的真实网络失败分支 | 用 `startCore` seam 覆盖状态清理与重试；不依赖外网故障 |

## 5. 历史缺陷回归清单

修复缺陷时必须保留对应测试：

| 缺陷 | 回归测试 |
| :--- | :--- |
| 版本事件再次排 refresh，形成反馈环 | `TestVersionChangeDoesNotScheduleRefresh` |
| 自动打开的队列 dock 抢焦点 | `TestQueueFocusRestoration` |
| 下拉框打开后焦点移到内部列表，全局键仍生效 | `TestTextInputFocusAndDropdownState` |
| 安装确认按 Esc 后下载 key 未释放 | `TestDownloadInstallConfirmationAndMetadataRetry` |
| Connector 文案使用了不存在的 i18n key | `TestConnectorLabelAndAvailability` |
| 收藏选择框 `[X]` 被当作样式标签吞掉 | `TestAddFavoriteDialogSelectionAndCancel` |
| `batchUnpin` 传入空 version 导致实际未取消 | `TestLivePinControllers` |
| 失败队列项在结果行不可见 | `TestDownloadQueueRowsAndItemText` |
| 空初始路径落到当前目录而不是 home | `TestInitialPathResolution` |
| 滚轮只移动内部 offset，光标不动 | `TestWheelScrollsResultList` |
| 主题切换后文字背景残留旧调色板 | `TestThemeSwitchRepaintsTextBackgrounds` |
| 设置表单焦点移出视口 | `TestSettingsKeyboardKeepsFocusVisible` |
| 下载速率输入框无法输入小数点 | `TestSettingsFormCallbacksAndFocus` |
| 零选项确认框越界 panic | `TestDialogEdgeCases` |
| 重复 `Host.Start` 替换正在运行的 core（泄漏 HTTP/core） | `TestSecondStartIsIdempotent` |
| `Startup` 失败后缓存半初始化 service | `TestStartFailureCanBeRetried` |
| 离线 harness 调用控制器 nil panic | `TestOfflineControllersAreNoOps` |
| 设置下拉框打开时 Esc 使 UI goroutine 死锁 | `TestLiveOpenDropdownConsumesEscape` |
| Tab/Backtab 把焦点移到只读分区标题（焦点不可见） | `TestLiveSettingsTabSkipsReadOnlyRows` |
| 点击描述行误触发多选/光标移动 | `TestDownloadClickZones`、`TestManageClickZones`、`TestFavoritesClickZones` |
| 标题含 CJK 时点击命中偏位 | `TestDownloadTitleClickWideCharacters` |
| 列表滚动后点击命中错行 | `TestDownloadClickAfterScrollingUsesOffset` |
| 收藏列表刷新后光标跳回第 0 行 | `TestFavoritesClickZones` |
| CJK 文案因宽字符续格被误判不存在 | `screenText` + `TestLiveLanguageDropdownRebuildsTheForm` |
| 事件契约漂移被静默丢弃 | `TestUnhandledEventsAreLogged` |

## 6. 不做的事

- 不在单元测试里执行真实网络下载；live 测试只使用本地 sqlite、配置和临时实例目录。
- 不断言具体颜色值，只断言布局、文本、调色板可区分性和 ANSI 完整性。
- 不追求 100% 覆盖率；优先覆盖事件循环、弹窗、设置、收藏、队列和鼠标命中。
