# TUI 测试方案

> 状态：旧 Bubble Tea TUI 已删除，本文中 `tui/`、`tea.Cmd`、`AppModel.Update` 等描述仅作历史参考。
> 当前 tview 测试位于 `internal/tviewui/*_test.go`：SimulationScreen 用例见
> `harness_test.go`、`shell_test.go`、`render_test.go`，golden 在
> `internal/tviewui/testdata/frames/`，生命周期/泄漏检查见 `lifecycle_test.go`。

> 目标：用可重复的自动化测试覆盖终端渲染、事件循环、键鼠交互、异步命令和进程级行为，避免“看起来能用、实际一用就坏”的回归。

## 1. 风险模型

按已发生过的真实缺陷分类：

| 类别 | 典型缺陷 | 测试层 |
| :--- | :--- | :--- |
| 事件反馈环 | refresh → event → refresh 无限循环，手动目标版本被重置 | L1 / L5 |
| 宽度与换行 | 弹窗内容按 width 截断，但 lipgloss 实际内容宽度是 width-2，导致内部换行、左右边框被挤掉 | L0 / L2 / L3 |
| 异步路由 | filepicker 的 readDirMsg 被送到当前页面而不是弹窗，永远 “No Files Found” | L1 / L4 |
| 回调未接线 | path picker 关闭但 onAccept 为空，确认后无任何效果 | L1 / L4 |
| 输入焦点 | 光标 blink 消息把输入框重置为当前目录，打字被吞 | L1 / L4 |
| 顺序错位 | 设置页显示顺序与上下键顺序不一致 | L1 / L2 |
| 副作用 | 日志写 stderr 与全屏 TUI 抢终端 | L6 |
| 命中区域 | 列表行、弹窗按钮、滚动后的行坐标错位 | L2 / L4 |
| 国际化 | 中文宽字符导致截断/边框错位 | L0 / L2 / L3 |
| 小终端 | 侧栏消失、弹窗溢出、宽图表换行 | L2 / L3 |

## 2. 测试分层

### L0 纯函数与组件

- `tui/kit`：Rect/Split/Grid、ANSI 安全 Width/Truncate/Pad/Wrap、CJK/emoji/组合字符。
- `tui/list`：cursor/offset/window/scrollbar/多选区间。
- `tui/table`：列宽分配、Drop 优先级、左右对齐、ANSI cell。
- 不变量：`Width(Truncate(s,w)) <= w`、`Width(Fit(s,w)) == w`、任何换行后每行 `<= w`。

### L1 模型与事件循环

纯 `AppModel.Update`，不启动真实 Host：

- 键路由顺序：WindowSize → 全局键 → 弹窗 → 队列 → 当前页。
- Modal 栈：push/pop、同 ID 去重、Esc 顺序、遮罩点击。
- 异步消息：`SelectedVersionChangedMsg` 不得再次排 refresh（回归测试）。
- 文件选择器：readDirMsg 必须到达顶层弹窗；onAccept 必须被调用。
- 设置页：`rows()` 顺序 = cursor 顺序；隐藏行（target_rate）不参与导航。
- 队列/下载状态：每个 core 事件都能被消费，且不会重复触发命令。

### L2 渲染不变量（每个页面、每个弹窗、每个尺寸）

对每一帧断言：

1. 行数 == 终端高度。
2. 每行 `kit.Width(line) <= 终端宽度`。
3. 无残缺 ANSI 转义（已有 `hasPartialEscape`）。
4. 弹窗：上下边框各出现一次，内容行数 == `modal.View()` 行数（不能因宽度不足而换行）。
5. 弹窗内容区宽度 == `Width - 2`（lipgloss padding 后的真实内容宽度）。
6. 点击矩形：`dialogBox` 包含 `dialogRect`，且都在屏幕内。
7. 侧栏：>=50 列必须可见；50–119 为 7 列 rail；>=120 为 30 列。
8. 设置页：如果内容高于视口，必须出现滚动条；滚动到尾部能看到最后一个设置项。

### L3 Golden 快照

目录：

```
tui/testdata/frames/
  home_120x40_en.golden
  home_80x24_zh.golden
  settings_80x24_zh.golden
  dialog_target_50x20_zh.golden
  ...
```

规则：

- 尺寸：50x20、80x24、100x30、120x40、160x50。
- 语言：en、zh。
- 页面：Home/Download/Manage/Favorites/Unpin/Settings。
- 弹窗：choice/select/prompt/version list/target/path picker/help/disable impact。
- 状态：空列表、加载中、错误、长列表（滚动）、多选、窄屏。
- 更新方式：`go test ./tui -run TestGolden -update`，golden diff 必须人工 review。
- 快照只存去掉 ANSI 的文本 + 单独存一份带 ANSI 的哈希，避免颜色变化导致噪音。

### L4 交互矩阵

| 操作 | 覆盖点 |
| :--- | :--- |
| 键 | 1–6、j/k、↑↓、PgUp/PgDn、Home/End、Space、Ctrl+A、Shift+↑↓、Esc、Enter、Tab/Shift+Tab |
| 鼠标 | 点击侧栏、行、按钮、遮罩、边框；Ctrl/Shift 多选；滚轮上/下 |
| 弹窗 | 点击按钮、点击行、滚动后点击、点边框不关闭、点遮罩关闭 |
| 文件选择器 | 树加载、Enter 选目录、Shift+Tab 切焦点、Ctrl+S 确认、输入路径、`~`/`$HOME` 展开 |
| Tab 补全 | 公共前缀、二次 Tab 列候选、单候选、目录斜杠、无候选 |
| 队列 | q 打开、Tab 切页、c/r/x、右键清空提醒、滚动不穿透 |

断言方式：直接调用 `m.Update` 注入 `tea.KeyMsg` / `tea.MouseMsg`，然后检查模型状态和 `m.frame.Hits`；不要依赖 sleep。

### L5 异步与命令

- `Host.emitCoreEvent`：8 种事件 → 强类型 Msg。
- 命令返回值：`cmd()` 产生的消息再次喂给 `Update`，断言状态收敛。
- 反馈环检测：连续执行 N 轮命令，若命令数量持续增长或状态重复变化则失败。
- 异步竞态：`SelectedVersionChangedMsg`、`localModsMsg`、`DownloadQueueUpdatedMsg` 乱序到达时状态仍一致。
- 定时器：toast 过期、spinner、queue 刷新在空闲时停止（命令数归零）。

### L6 进程级 E2E

优先用 `github.com/charmbracelet/x/exp/teatest`（或轻量 pty harness）：

- 启动真实 `tui.Run`，等待首帧，断言关键文本。
- 打开设置 → 文件选择器 → 输入 `/home/link/.minecraft` → Enter → 回到设置断言路径。
- 打开实例弹窗 → 切到 MC version → Right 两次 → 断言值变化并保持（回归 refresh loop）。
- 选择器：Shift+Tab → Down → Enter → Ctrl+S → 断言保存。
- 退出：Ctrl+C，断言进程在 2s 内结束、日志文件关闭。
- 日志断言：TUI 运行期间 stderr 为空；日志文件无 DEBUG。

补充 tmux 冒烟脚本（真实终端、真实 TTY 尺寸）：

```sh
tmux new-session -d -s tui -x 80 -y 24 "./mod-downloader-cli"
sleep 3
tmux send-keys -t tui t; ...
tmux capture-pane -p -t tui | grep -q "Minecraft Version"
```

### L7 稳定性

- `go test -race ./tui/...`
- 随机键序列 fuzz：从一组合法键中随机 10k 步，断言不 panic、帧尺寸不越界。
- 尺寸 fuzz：宽 40..200、高 10..60 的所有组合，6 个页面 + 弹窗不 panic。
- 性能基准：`BenchmarkView`（空/1000 行）、`BenchmarkManageRows`、`BenchmarkSettingsView`。
- 内存：渲染 1000 帧不增长（`testing.AllocsPerRun` 基线）。
- 泄漏：`goleak` 检查 TUI 退出后无残留 goroutine。

## 3. 测试工具设计

### 3.1 testutil（tui 包内）

```go
func newTestApp() *AppModel
func render(t *testing.T, m *AppModel, w, h int) string
func sendKey(t *testing.T, m *AppModel, key tea.KeyMsg)
func sendRunes(t *testing.T, m *AppModel, s string)
func click(t *testing.T, m *AppModel, x, y int)
func wheel(t *testing.T, m *AppModel, delta int)
func runCmd(t *testing.T, m *AppModel, cmd tea.Cmd) // 递归执行 BatchMsg
func assertFrame(t *testing.T, out string, w, h int)
func findHit(t *testing.T, m *AppModel, kind HitKind, name string) Hit
```

### 3.2 Golden helper

```go
var update = flag.Bool("update", false, "update golden frames")

func golden(t *testing.T, name, got string) {
    path := filepath.Join("testdata", "frames", name+".golden")
    if *update { os.WriteFile(path, []byte(got), 0o644); return }
    want, _ := os.ReadFile(path)
    if diff := cmp.Diff(string(want), got); diff != "" { t.Fatal(diff) }
}
```

### 3.3 事件/命令收敛 helper

```go
func settle(t *testing.T, m *AppModel, msg tea.Msg, maxSteps int) *AppModel {
    for i := 0; i < maxSteps; i++ {
        model, cmd := m.Update(msg)
        m = model.(*AppModel)
        if cmd == nil { return m }
        msg = cmd()
    }
    t.Fatal("commands did not settle")
    return m
}
```

## 4. 实施优先级

### P0（已有，必须保持）

- kit/list/table 纯函数测试。
- 六页 × 尺寸/语言渲染不变量。
- 弹窗点击/滚轮/遮罩/边框测试。
- 设置显示顺序、隐藏动画/简易模式。
- 文件选择器：onAccept、异步加载、Tab 补全、树选择。
- 版本切换 + refresh loop 回归。
- 日志：TUI stderr 为空、CLI 默认 WARN、-v DEBUG。

### P1（本季度）

- Golden 快照全量（6 页 × 5 尺寸 × 2 语言）。
- 所有弹窗的渲染不变量（不换行、边框完整）。
- `settle` 命令收敛测试：版本切换、目录切换、下载队列。
- fuzz：随机键序列 + 随机尺寸。
- `go test -race` 纳入 CI。

### P2（后续）

- teatest 进程级 E2E：启动、设置、文件选择、退出。
- tmux 冒烟脚本（3 个尺寸）。
- 性能基准与 alloc 基线。
- goleak。

## 5. CI 流水线

```sh
go test ./...
go test -race ./tui/...
go vet ./...
go test ./tui -run TestGolden
go test ./tui -fuzz FuzzKeySequences -fuzztime=30s
./scripts/tui-smoke.sh
```

门槛：

- 行覆盖率：`tui` >= 70%，`internal/uistate` >= 80%，`tui/kit` >= 90%。
- 所有 golden diff 必须人工确认。
- 任何 fuzz/race 失败阻断合并。

## 6. 不做的事

- 不测试真实网络下载（用 fake service，E2E 只测交互）。
- 不断言具体颜色值（只断言 ANSI 完整性和文本）。
- 不追求 100% 覆盖率；优先覆盖事件循环、弹窗、文件和设置这些高风险路径。
