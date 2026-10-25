# TUI 迁移到 tview 计划

> 状态：已完成（2026-10-25），本文保留为迁移历史记录。

> 结论：先做小规模 spike，确认 tview 的布局、焦点、弹窗和测试方式后，再以“新包 + 特性开关”的方式并行迁移。不要在当前 Bubble Tea 代码上原地替换。

## 1. 目标与非目标

### 目标

- 用 tview 的现成组件替代手写 overlay、焦点栈、命中矩形、表格滚动和表单。
- 保留现有 appcore、internal/uistate、i18n、Host 的核心能力。
- 6 个页面 + 弹窗 + 路径输入式文件选择器 + 右侧队列 dock 达到与 GUI 等价的功能。
- 不保留旧 Bubble Tea TUI：每个阶段完成即删除对应的旧 BT 代码；回滚依赖 git revert/分支。
- 可用 tcell SimulationScreen 做确定性 UI 测试。

### 非目标

- 不重写 core 下载、搜索、收藏逻辑。
- 不追求像素级复刻 Lip Gloss 的卡片和渐变；采用 tview 原生风格。
- 不以旧 Bubble Tea TUI 为设计来源；布局和交互以 GUI 项目的 Vue 实现为准。
- 不在迁移期间同时改业务行为。

## 2. 目标架构

~~~
cmd/mod-downloader-cli
  └─ internal/tuihost          // 与 UI 无关：core service + HTTP + 事件 sink
       └─ internal/tviewui     // 唯一 TUI（旧 tui 包在 P2 删除）
            ├─ app.go          // Application + Pages + 侧栏 + 状态栏
            ├─ theme.go        // tcell 颜色 + 边框
            ├─ views/          // home/download/manage/favorites/unpin/settings
            ├─ dialogs/        // confirm/prompt/select/target/path input
            ├─ queue.go        // 右侧 dock
            └─ testutil/       // SimulationScreen harness
~~~

## 2.1 设计来源

每个页面以 GUI 项目的 Vue 组件为准，不再对照旧 BT TUI：

| tview 视图 | GUI 参考 |
| :--- | :--- |
| Home | ../mod-downloader 的 Home 页：指标卡、运行状态 |
| Download | 搜索栏、结果行、安装/更新状态、批量栏 |
| Manage | 本地 Mod 行、启用状态、筛选、清理 |
| Favorites | 左列表栏 + 右条目栏、引用/导出 |
| Unpin | 版本表格、批量取消固定 |
| Settings | 分组表单、缓存目录、API Key |
| Queue | GUI 的右下下载队列（tview 改为右侧 dock） |

关键原则：

- 单一数据源：数据仍在 uistate/core；tview 组件只做展示和输入，不另存一份业务状态。
- 控制器集中：所有 service 调用放在 tviewui.Controller，视图通过回调触发；异步结果统一走 Application.QueueUpdateDraw。
- UI 线程约束：Host 事件、下载进度、搜索回调都从 goroutine 进入，必须经 QueueUpdateDraw 更新界面。
- 不混用两套事件循环：Bubble Tea 和 tview 不能同时驱动同一个终端。

## 3. 组件映射

| 现有 Bubble Tea | tview 方案 | 说明 |
| :--- | :--- | :--- |
| AppModel.Update 路由 | Application.SetInputCapture + Controller | 全局键优先，Modal/Form 内部键交给焦点组件 |
| Frame.Hits / 手写命中 | tview 焦点 + 鼠标事件 | List/Table/Button 原生可点；状态栏胶囊用 SetMouseCapture |
| 侧栏 renderSidebar | List + Flex | 宽度 30/7/0，resize 时调整 Flex 固定尺寸 |
| 主内容 | Pages | 每个 View 一个 page，保留滚动和焦点状态 |
| 状态栏 | TextView + 右侧 Button | 队列胶囊用 Button，原生可点 |
| Home 卡片 | TextView 边框 + Flex/Grid | 窄屏重建列数；不做渐变 |
| Download 列表 | InputField + List | List 支持 secondary text、multi-select |
| Manage 列表 | List + 自绘复选框文本 | 过滤/排序后重建 List |
| Favorites 双栏 | Flex + 两个 List | Tab 切焦点 |
| Unpin 表格 | Table | 原生列宽、选择、滚动 |
| Settings | Form | DropDown/Checkbox/InputField/Button 分节 |
| 确认弹窗 | Modal | 原生按钮 + DoneFunc |
| 输入/选择弹窗 | 居中的 Form/List page | 用 spacer Flex 居中；Pages 页面不透明 |
| 文件选择器 | InputField 自动补全 + TreeView | tview 无内置 file picker，需要自建 |
| 队列面板 | 右侧 dock Flex（推荐）或 Pages 弹窗 | tview 没有透明浮层；dock 最稳 |
| Toast | 状态栏临时文本 / SetAfterDrawFunc | 不做右上角浮层 |
| Theme | tcell.Color + tview.Borders | 圆角可用 tview.Borders 定制字符 |
| 测试 | tcell.NewSimulationScreen + golden | 单元格断言，不依赖真实终端 |

## 4. 迁移阶段

### P0：Spike（先验证，不进入主线）

交付一个独立的 cmd/tview-spike 或 internal/tviewui/spike：

- 侧栏 List + Pages 切页。
- 居中 Modal（确认框）。
- Table + List 滚动。
- 50/80/120 列 resize。
- 鼠标点击 + 键盘焦点。
- SimulationScreen 注入按键并断言单元格。

验收：确认 tview 的焦点、键捕获、鼠标和测试 API 满足需求；确认队列面板采用 dock 还是 Pages。若 spike 发现关键阻塞，停止迁移，保留 Bubble Tea。

### P1：Host 解耦（不改变 UI 行为）

- 把 tui/host.go 中与 Bubble Tea 绑定的 ConfigureProgram(*tea.Program) 改为 UI 无关的 SetEventSink(func(Event))。
- 抽出 internal/tuihost：core service、HTTP server、事件分发、版本信息。
- Bubble Tea 适配层注册 sink：program.Send(coreEventMsg{...})。
- tview 适配层注册 sink：app.QueueUpdateDraw(func(){ controller.Handle(ev) })。
- 现有测试全部保持通过。

验收：旧 TUI 行为不变；go test ./... 绿。

### P2：新壳层 + 删除旧 TUI

新增 internal/tviewui：

- Application + Pages + Flex。
- 侧栏 List：6 项、30/7/0 列、resize 响应。
- 底部状态栏 TextView + 队列 Button。
- 全局键：1–6、q、t、?、Esc、Ctrl+C；注意 Modal 打开时不要被全局键截走。
- 主题：tview 原生颜色/边框。
- 语言切换：重建/刷新所有 primitive 的文本。
- 切换 main.go 到 tviewui。

**删除旧 Bubble Tea TUI（同一提交）：**

- 删除 tui/app.go、view_*.go、modal.go、components.go、overlay.go、drawer_queue.go、target_modal.go、pathpicker.go、help_modal.go、layout.go、frame.go、theme.go、format.go、commands.go、messages.go 等 BT 展示层。
- 保留并迁出 host 相关代码到 internal/tuihost；保留 internal/uistate、i18n、kit 中仍被新 UI 使用的部分。
- 删除仅测试旧 TUI 的测试；新 UI 先建立 SimulationScreen 基础测试。

验收：仓库只存在一套 TUI；main 可编译并显示 6 个占位页；50/80/120 列布局正确；模拟屏幕测试通过。

> 注意：按“完成即删”的要求，P2 结束时旧 TUI 已删除，P3 之前只有占位页。若不能接受这个中间态，把 P3 的只读页面并入 P2，完成只读界面后再删旧 TUI。

### P3：只读页面

按 GUI 页面实现，按依赖从少到多：

1. Unpin：Table + 数据加载/刷新。
2. Home：指标卡 + 运行状态。
3. Favorites：双 List + 作用域。
4. Download：搜索框 + 结果 List（先只读）。
5. Manage：本地 Mod List（先只读）。
6. Settings：Form（先只读展示）。

每页提供：

- Refresh(state)：从 uistate 重建内容。
- Focus()：进入页面时设置焦点。
- HandleKey：仅页面特有键。

验收：每页在 50x20/80x24/120x40、中英文下不越界；SimulationScreen golden 通过。P2 已删除旧 BT 展示层，P3 只新增 tview 页面。

### P4：写操作

- Download：搜索、分页、安装、批量、pin、收藏。
- Manage：启用/禁用/删除/扫描/筛选。
- Favorites：创建/重命名/删除/排序/引用/导出。
- Settings：所有保存动作、缓存目录、API Key。
- 所有 service 调用放 Controller；结果通过 QueueUpdateDraw 回到 UI。
- 统一错误提示：状态栏 + Modal（替代 toast 的复杂浮层）。

验收：现有 cliapp/core 测试不受影响；新 UI 的关键路径有模拟测试。

### P5：弹窗与路径输入

- dialogPage(primitive, width, height)：用 Flex spacer 居中，Pages 叠加。
- Confirm：Modal。
- Prompt：Form + InputField，Enter 确认、Esc 取消。
- Select/VersionList：List/Table + 按钮。
- Target/Instance：Form 或 List；左右键切换 MC/loader。
- 文件选择器（已确认：只做路径输入 + Tab 补全）：
  - 单行 InputField + 确认/取消按钮，不再内置 TreeView 浏览器。
  - Tab 实现 bash 风格：第一次补公共前缀，第二次列出候选；支持 $HOME、$PWD、~。
  - 目录不存在或不是目录时在表单内显示错误，不关闭弹窗。
- 焦点栈：Pages 顶层页面；Esc 只关最上层；点遮罩关闭不做（Pages 无透明遮罩）。

验收：所有弹窗在 50x20 不换行、不越界；路径输入能选中 /home/link/.minecraft 并在设置页回显；Tab 补全测试通过。

### P6：队列、Toast、响应式

- 队列：右侧 dock（已确认）：
  - Flex 主区 + 右侧固定宽度队列列；打开时主区收缩。
  - 宽度 clamp(28, 40)；q 切换显隐；Tabs、逐项进度、取消/重试/移除。
- Toast：状态栏短暂消息；不做浮动层。
- 响应式：resize 时重建 Home 列数、侧栏宽度、队列宽度。
- 空闲时停止 ticker；队列面板关闭时不刷新。

验收：队列打开/关闭不破坏焦点；无 goroutine 泄漏；模拟测试覆盖。

### P7：测试对齐

- 移植/重写测试（不再依赖旧 BT 模型）：
  - 渲染不变量（尺寸/宽度/ANSI/弹窗不换行）。
  - 设置顺序、隐藏动画/简易模式。
  - 路径输入 onAccept/Tab 补全/错误提示。
  - 版本切换 + refresh loop 回归。
  - 日志：TUI stderr 为空。
- Golden：SimulationScreen 单元格序列化。
- 不保留 --tui 开关，不保留旧 tui 包；回滚只能通过 git revert。
- 文档：README、TUI 键位、日志说明。

## 5. 关键设计决策

### 5.1 队列面板

已确认：右侧 dock。tview 没有透明浮层。

- 根布局：横向 Flex = 主区（flex）+ 队列列（固定宽度，打开时）。
- 队列宽度 clamp(28, 40)，q 切换显隐；关闭时主区占满。
- 队列列内部：标题、Tabs、List/Table、底部按键提示。
- 不采用 Pages 全屏弹窗，也不自定义 overlay。

这样牺牲“浮在内容上”的 GUI 观感，换取稳定布局和可测试性。

### 5.2 弹窗

- 已确认采用 tview 原生风格：Pages 全屏 page + 居中带边框的 Form/Modal。
- tview Pages 不透明，不做“背景变暗但可见”的 scrim，也不自绘遮罩。
- 弹窗宽度按内容设置，高度按内容设置；超过屏幕时由 Flex 裁剪/滚动。

### 5.3 设置页

- 用单个 Form 分节（TextView 分隔 + 表单控件），不再手写卡片。
- 每个控件保存一个稳定 key，Refresh 时按 key 回填，避免顺序错位。
- DropDown/Checkbox/InputField 的变更回调直接走 Controller。
- 数字项用 InputField + 校验，或 Button +/-。

### 5.4 焦点与快捷键

- Application.SetInputCapture 只处理全局键；如果当前焦点是 InputField，字符键不拦截。
- Modal/Form 打开时，全局 1–6 不生效。
- 页面内 Tab/Shift+Tab 由 tview 焦点链处理；Favorites 双栏显式 SetFocus。

### 5.5 状态同步

- List/Table 的选中项可能被重建打乱。重建前保存 key，重建后按 key 恢复；没有 key 时按索引 clamp。
- 设置 Form 用 key→item 映射，不依赖行号。
- 所有列表的行数据来自同一个有序切片，避免“显示顺序 ≠ 光标顺序”。

## 6. 测试方案（适配 tview）

### 6.1 SimulationScreen harness

~~~go
sim := tcell.NewSimulationScreen("UTF-8")
sim.SetSize(120, 40)
app := tview.NewApplication().SetScreen(sim)
app.SetRoot(root, true)
app.Draw()
cells, w, h := sim.GetContents()
~~~

- 注入键：sim.InjectKey(tcell.KeyRune, '6', tcell.ModNone)、tcell.KeyEnter、tcell.KeyEsc。
- 注入鼠标：sim.InjectMouse(x, y, tcell.Button1, tcell.ModNone)。
- 异步：用 channel 等待 Controller 完成，再 app.Draw()。
- 把 cells 序列化为纯文本/ANSI，做 golden。

### 6.2 必测矩阵

- 尺寸：50x20、80x24、100x30、120x40、160x50。
- 语言：en、zh。
- 页面：6 个。
- 弹窗：confirm/prompt/select/version/target/file picker。
- 状态：空、加载、错误、长列表、滚动、多选。
- 回归：版本切换不回弹、弹窗不换行、文件选择器回调、Tab 补全、设置顺序、日志隔离。

### 6.3 测试层级

- 单元：uistate、i18n、格式化。
- 组件：每个 View 的 Refresh + 模拟键。
- Golden：整屏单元格。
- E2E：真实 Application + SimulationScreen；tmux 仅做真实终端冒烟。

## 7. 风险与缓解

| 风险 | 影响 | 缓解 |
| :--- | :--- | :--- |
| 大重写回归 | 高 | 新包 + flag；旧 TUI 保留；逐页切换 |
| tview 无透明 overlay | 中 | 队列 dock；弹窗用全屏 page |
| 文件选择器工作量大 | 高 | 先做 InputField 路径输入 + 简单 TreeView；Tab 补全单独测试 |
| 样式损失 | 中 | 用 tview.Borders/颜色；接受终端原生风格 |
| 焦点/键冲突 | 中 | InputCapture 只做全局键；每页显式 Focus |
| 状态双份 | 高 | 单一 uistate；List 重建按 key 恢复 |
| QueueUpdateDraw 死锁 | 高 | 只在 goroutine 中调用；UI 回调内不阻塞 |
| 测试基建重建 | 中 | P0 先验证 SimulationScreen；golden 先行 |
| tview Table 大列表性能 | 中 | 限制可见行/分页；避免每次全量重建 |

## 8. 验收标准

- 仓库只保留一套 TUI；P2 完成后旧 Bubble Tea 展示层已删除。
- 6 个页面、9 类弹窗、路径输入式文件选择器、右侧队列 dock 功能等价。
- 50x20/80x24/120x40/160x50、中英文无越界/换行/残缺 ANSI。
- 版本切换不回弹；路径输入可选中 /home/link/.minecraft；设置顺序正确。
- go test ./...、go test -race ./...、go vet ./... 全绿。
- TUI 运行时 stderr 为空；日志仍只写文件。
- 退出后无 goroutine 泄漏（goleak）。

## 9. 已确认的决策

1. 队列面板：右侧 dock。
2. 视觉：tview 原生风格。
3. 文件选择器：路径输入 + Tab 补全，不做 TreeView 浏览器。
4. 切换策略：完成任一阶段即删除对应的旧 Bubble Tea TUI；不保留并行实现和 --tui 开关。
5. 设计来源：GUI 项目的 Vue 页面，不是旧 BT TUI。

回滚方式：git revert / 分支；不做运行时回退。