# 架构梳理与改进计划

> 分析日期：2026-10-25（修订版）
> 基线提交：`6b15602`（主仓库 `main`；`core` 子模块以该提交记录的版本为准）
> 范围：`mod-downloader-cli` 主仓库 + `core` 子模块（`mod-downloader-core`，被 CLI/TUI 与 Wails GUI 共用）
> 基线校验：`go build ./...`、`go vet ./...`、CLI 与 core 两个 module 的 `go test ./...` 全部通过；当前没有 CI。本文行数、方法数与日志统计均在该基线上实测，统计口径见第 7 节。
>
> 修订说明：本版修正了旧稿中与当前代码不符的数字和引用（`cliapp` 行数、Service 方法数、`Controller` 类型、core 包数等），并补齐了日志写入 mods 目录、`info` 命令同样依赖 `jarmeta.go`、事件契约包含适配器内部事件等遗漏。所有数字以基线上实测为准。

> 实施进展（2026-10-27）：core 侧 P0-1（日志轮转/路径）、P0-2（生命周期幂等与失败清理）已落地；本仓库完成 P2-3 的按域文件拆分（`controller.go` 916→151 行，未新增 Controller 类型）、P1-5 的未知事件/错误 payload 告警与映射测试、P2-5 的 Makefile。新增 DAAU 测试层：真实事件循环键鼠注入、真实 Modrinth/Mojang 集成（env 门控）、tmux 真实终端冒烟、离线控制器 no-op 契约。

## 0. 结论

分层方向是对的：`cmd → cliapp / tviewui → tuihost / uistate → appcore → core 领域包`，`appcore` 不依赖 Wails；TUI 的 UI 写入统一经 `uiUpdate`/`QueueUpdate` 回到 UI 线程，golden + goleak 测试也已成型。**不需要重写。**

真正的问题不是“层数不够”，而是三类横向侵蚀：

1. **适配层复制了 core 的业务逻辑**：CLI 自带一套 jar 解析器（`cliapp/jarmeta.go`，711 行），`verify` 同时对同一个 jar 解析两遍；`check-update`/`upgrade` 的升级编排也留在 CLI，并绕过了 core 已有的旧 jar 归档逻辑。
2. **core 是“进程级单例”但约束没有收口**：`downloader`、`storage`、`global`、`logging`、`providers`、`modbridge` 都持有包级可变状态，`Service.Startup` 非幂等，`Shutdown` 与 `Close` 语义不同；`Host.Start` 在 `Startup` 前就写入 core，失败后仍会留下半初始化实例；若 `Start` 从未调用，`Service()` 会创建未启动实例。
3. **日志与工作区治理缺失**：日志追加写到 `Runtime.WorkDir`（默认 CWD，带 `--dir` 时是目标 mods 目录），无轮转、高频 INFO 记录；当前工作区已有 392 MB 日志、2.76 M 行，尾 20 万行中三类刷新日志各约 6.5 万次。

改进策略：先做低风险的 P0（日志、生命周期），再消除重复实现（P1），最后才做结构性拆分（P2）。所有 `core` 改动必须保持对 GUI 的向后兼容，或与 GUI 仓库协同发布。

---

## 1. 现状架构

### 1.1 分层与模块

```text
cmd/mod-downloader-cli
  ├─ (无子命令 | tui) ──► internal/tviewui   tview/tcell 全屏 TUI
  │                         ├─ 6 个页面 + 9 类弹窗 + 右侧队列 dock
  │                         ├─ internal/uistate   纯状态（minecraft/search/queue/manage/favorites）
  │                         ├─ internal/i18n      zh/en 字典
  │                         └─ internal/tuihost   core service + HTTP bridge + event sink
  │                                │
  └─ 其他子命令 ─────────► cliapp             urfave/cli，15 个命令 + huh 交互向导
                                   │
                                   ▼
                         core/appcore.Service   adapter-neutral facade
                         （search / favorites / settings / downloads / instances / events）
                                   │
        ┌──────────────────────────┼─────────────────────────────┐
        ▼                          ▼                             ▼
   providers ───────────────► modbridge ───────────────► downloader
        │                          │                             │
        └──────────────► storage / minecraft / packwiz ◄─────────┘
                                   │
                    global / configs / logging / networking / httpserver
```

### 1.2 各层职责与规模

| 层 | 主要职责 | 关键位置 | 生产代码行数（不含测试） |
| :--- | :--- | :--- | ---: |
| CLI 适配 | 参数、配置优先级、表格/JSON 输出、交互向导 | `cliapp/` | 4,766 |
| TUI 外壳 | 布局、焦点、弹窗、队列 dock、事件泵 | `internal/tviewui/` | 8,420 |
| TUI 状态 | 与 Vue/Pinia 对齐的纯状态机 | `internal/uistate/` | 1,214 |
| TUI Host | core 生命周期 + 事件翻译 | `internal/tuihost/host.go` | 269 |
| 国际化 | key-based 中英文文案 | `internal/i18n/i18n.go` | 796 |
| 应用编排 | 对外 facade、事件、工作流 | `core/appcore/` | 含在 core 16,760 |
| 领域能力 | provider、下载、解析、存储、实例发现 | `core/{providers,downloader,minecraft,storage,modbridge,...}` | 含在 core 16,760 |

`appcore/service.go` 共 97 个方法（其中 73 个导出）、2,004 行；`core/appcore` 全包共 104 个 `*Service` 方法（80 个导出）。`core` 测试约 9,367 行。

### 1.3 关键运行路径

**TUI 事件路径**（`internal/tuihost/host.go` + `internal/tviewui/app.go`）：

```text
core worker goroutine
  → appcore.Service.emit(appcore.Event{Kind, Payload any})
  → tuihost.emitCoreEvent（翻译成 tuihost.Event{Kind, Payload any}）
  → App.enqueueEvent（非阻塞缓冲）
  → 事件泵 goroutine：compactEvents → QueueUpdate → App.handleEvent
  → App.handleEvent 更新 uistate / 重建控件 → ForceDraw
```

事件泵、批量合并、`uiUpdate`、goleak 测试是这套架构里做得最扎实的部分（`app.go#L412-L541`）。

**下载路径**：

```text
cliapp 或 TUI App 方法
  → appcore.Service.QueueModDownload / InstallModToDirAndWait
  → downloader 包级队列（pending/current/retryable/progress）
  → modbridge.ResolveInstallTarget / ResolveVersion
  → filetransfer + 事件回调
  → appcore.EventDownloadQueueUpdated / DownloadFailed / DownloadCompleted
```

### 1.4 与 GUI 的关系

`core` 是独立 git submodule，主仓库通过 `replace github.com/link-fgfgui/mod-downloader-core => ./core` 引入。`appcore` 同时服务 Wails GUI、CLI 和 TUI，因此：

- **可以**新增 core 方法、内部拆分实现；
- **不要**直接修改既有导出方法签名或语义，除非同步 GUI 仓库；
- 涉及 `storage` 返回类型（`PinnedMod`、`FavoriteList` 等）的调整会影响多个适配器。

---

## 2. 架构评估

### 2.1 应当保留的设计

- **adapter-neutral facade**：`core/appcore` 没有 Wails 依赖，CLI/TUI 共享同一套业务能力，这是正确的方向。
- **TUI 的线程边界**：UI 写入统一经 `uiUpdate`/`QueueUpdate` 回到 UI 线程（`internal/tviewui/app.go#L412-L541`）。注意 service 调用并不集中在单个 goroutine：`afterStart` 会启动 4 个刷新 goroutine（另有 1 个等待 goroutine，`app.go#L550-L554`），`refreshMinecraft`/`refreshSelectedMods` 等工作 goroutine 直接调用 service（`controller.go#L95-L102`、`#L129-L135`）。因此 service 必须并发安全；当前没有独立的 `Controller` 类型，相关方法都挂在 `*App` 上。
- **事件合并**：队列进度按快照合并，避免每个 tick 重绘（`app.go#L518-L541`）。
- **纯状态层**：`internal/uistate` 不依赖 tview，便于单测与 golden。
- **测试基建**：60 个 golden 覆盖 6 页面 × 5 尺寸 × 2 语言；有 `-update` 生成开关；`goleak` 检查协程泄漏。
- **配置/网络/存储分包**：`configs`、`networking`、`providers`、`storage` 边界清楚，网络调用有 `*Fn` 测试 seam。

### 2.2 问题清单

| ID | 级别 | 问题 | 直接证据 | 影响 |
| :--- | :--- | :--- | :--- | :--- |
| P0-1 | 高 | 日志无轮转、写入 WorkDir、高频 INFO | 392 MB / 2.76 M 行日志；尾 20 万行中 3 类消息各约 6.5 万次 | 磁盘膨胀、I/O 浪费、日志失去可用性；`--dir` 时会污染 mods 目录 |
| P0-2 | 高 | core 生命周期与包级单例未收口 | `Service.Startup` 非幂等；`Shutdown`/`Close` 语义不同；`Storage/Global/Downloader` 包级变量 | 无法多实例、测试隔离差、异常路径资源泄漏 |
| P1-1 | 高 | jar 解析双实现 | `cliapp/jarmeta.go`（711 行）与 `core/minecraft/modparser.go`（891 行） | 解析语义漂移；`verify` 每个 jar 解析两遍 |
| P1-2 | 高 | 更新/校验业务逻辑留在 CLI | `cliapp/update.go` 手动删旧 jar；`cliapp/verify.go` 自建依赖分析 | CLI/TUI/GUI 行为不一致，逻辑无法复用 |
| P1-3 | 中 | 目标目录/版本探测在 CLI | `cliapp/app.go#L529-L802` 与 `core/minecraft` 的 `launcher.go`/`prism.go`/`tool.go` 重叠，并临时 `appcore.New` | 三套探测规则，易漂移 |
| P1-4 | 中 | 配置双轨、优先级分散 | CLI `config.json`（6 个 key）vs core `mod-downloader.toml` | 同一设置两处、TUI/CLI 不一致 |
| P1-5 | 中 | 事件契约重复且可静默丢事件 | `appcore.EventKind` 与 `tuihost.EventKind` 两套；`Payload any` + 类型断言 | 事件丢失无告警，契约难维护 |
| P2-1 | 中 | `appcore.Service` 是 god facade | `service.go` 2,004 行 / 97 个方法（73 导出），7 组职责 | 修改成本高，难并行开发 |
| P2-2 | 中 | core 领域包都是全局单例 | `downloader` 队列、`storage` DB、`global` 状态、`providers` 后台任务 | 阻断实例化、并发测试与未来多窗口 |
| P2-3 | 中 | TUI `App` 是 god object | `app.go` 1,150 行；`controller.go` 925 行；`App` 共 121 个方法（其中 controller.go 62 个），没有独立 `Controller` 类型 | 认知负担、合并冲突 |
| P2-4 | 低 | i18n 是 796 行 Go map | `internal/i18n/i18n.go` | 文案改动都改 Go 代码，缺少完整性校验 |
| P2-5 | 中 | 工程化缺失 | 无 `.github`、无 Makefile/Taskfile；README 写 Go 1.24，CLI `go.mod` 1.25、core `go.mod` 1.24.0 | 双 module 测试/发布靠手工，文档漂移 |
| P2-6 | 低 | 适配器依赖持久层类型 | `tviewui` 直接使用 `storage.PinnedMod/FavoriteList` | 契约与持久层耦合 |

### 2.3 重点问题说明

#### P0-1 日志治理

`core/logging/logging.go#L37-L77` 的 `Configure` 只用 `os.OpenFile(..., O_APPEND)` 打开文件，没有大小上限、轮转、保留份数。日志路径由 `core/appcore/service.go#L1648-L1651` 决定，为 `<Runtime.WorkDir>/mod-downloader.log`：TUI 默认 WorkDir 为空，落在进程 CWD；CLI 带 `--dir` 时 `Runtime.WorkDir` 会被设为目标 mods 目录（`cliapp/app.go#L561`），日志会写进用户的 mods 文件夹。`LoggingConfig` 目前只有 `Enabled/Disabled/ForceFile`，没有 file/level 字段（`core/configs/structs.go#L298-L302`）。TUI 以 `NoStderr` 模式运行（`internal/tviewui/app.go#L87-L94`），所有日志都写文件。

高频刷新路径把状态变更记成 INFO：

- `core/global/global.go#L100`（`global versions cache set`）
- `core/global/global.go#L165`（`global selected version set`）
- `core/global/localmods.go#L224`（`local mods cleared by instance`）
- `core/appcore/service.go#L1474-L1505`（每次 `LoadVersionsFromDisk` / `refreshVersionMods` 都会触发上述日志）

实测：`mod-downloader.log` 392,932,349 字节、2,760,806 行；尾 20 万行中三类消息分别出现 65,892 / 65,872 / 65,806 次。根因不是单次 bug，而是“每次目录监听/实例刷新都全量扫描并打 INFO”叠加“永不轮转”。

#### P0-2 生命周期与全局状态

`core/appcore/service.go#L65-L110` 的 `Startup` 依次修改 `providers`、`downloader`、`modbridge`、`global`、`storage`、`logging` 的包级状态；`Shutdown`（#L138-L150）保存配置并关闭这些全局对象，`Close`（#L152-L157）关闭但不保存。`Startup` 没有状态保护，重复调用会重复启动后台任务并重新加载全局状态。`Shutdown`/`Close` 也没有显式保护；虽然 `storage.Close`、`logging.Close`、`stopModsWatcher` 本身可重复调用，但两个入口语义不同，调用方无法从名字判断该用哪个。

CLI 的业务命令通过 `withService` 创建 Service（`cliapp/app.go#L495-L527`，共 9 处调用，另有 1 处定义）；目标探测里还有 2 处临时 `appcore.New`（见 P1-3）。`Startup` 失败时直接 return（不会执行 `defer svc.Close()`，因为 defer 在其后才注册），会留下已初始化的全局资源。`internal/tuihost/host.go#L132-L136` 的 `Start` 在调用 `Startup` 前就把 core 写入 `h.core`；失败后 `Service()`（#L179-L195）返回的是这个半初始化实例。只有 `Start` 从未调用时，`Service()` 才懒创建一个未 `Startup` 的实例。

当前因为是“一个进程一个 Service”，问题被掩盖；一旦要做多窗口、并行测试或嵌入 GUI，就会暴露。

#### P1-1 jar 解析双实现

`cliapp/jarmeta.go` 自带 `parseFabricMetadata`、`parseQuiltMetadata`、`parseModsTomlMetadata`、`parseMcmodInfoMetadata` 以及一套 Maven/tilde/caret 版本范围实现；`core/minecraft/modparser.go` 已提供 `ScanModsDir`、`ParseLocalModZipReader`、依赖解析和 Jar-in-Jar 处理。

`cliapp/verify.go#L68` 先用 `minecraft.ScanModsDir` 扫描一遍，`#L125` 又对同一批 jar 调 `parseJarMetadata`。结果是：同一个 jar 打开两次、两套规则可能给出不同结论（例如版本范围、loader 兼容性）。

此外，`parseJarMetadata` 还被 `info` 命令使用（`cliapp/verify.go#L325` 的 `buildInfoView`），输出 `License`、`Authors`、`Recommends`、`Breaks` 和 Minecraft 版本范围；而 `structs.ModInfo` 目前只包含 `Dependencies`、`Loaders`、`JijMods` 等字段（`core/structs/minecraft/modinfo.go#L13-L36`）。因此删除 `jarmeta.go` 之前，必须先扩展 core 的元数据模型或新增详情解析 API，不能只迁移 verify。

#### P1-2 更新/校验逻辑在 CLI

- `cliapp/update.go` 的 `applyUpgradePlans`（L311-L353）自己调用 `InstallModToDirAndWait`、自己 `os.Remove` 旧 jar、自己处理 `.disabled` 和 `.old*` 清理（`removeSupersededFiles` L365-L377）。
- core 的 `downloader` 已经有 `archiveSupersededModJars`（`core/downloader/download.go#L854-L878`），两条清理路径并存。
- `cliapp/verify.go` 自己实现了缺失依赖、重复 ID、loader 和版本范围检查。core 现有的 `AnalyzeLocalModDisableImpact`（`dependency_impact.go#L19`，禁用影响分析）与 `ScanUnusedDependencies`（`unused_dependencies.go#L33`，未使用模组扫描）解决的是不同问题，不能直接复用；应新增共享的 verify API，而不是把这两个方法硬套进来。

这导致同样的问题在 CLI、TUI、GUI 各有一套判断，且 CLI 的升级行为无法被 TUI 复用。

#### P1-3 目标探测重复

`cliapp/app.go` 的 `runtimeInput` / `locateRuntimeDir` / `inferRuntimeFromModsParent` / `inferVersionInfoFromDir` / `candidateMinecraftRootsForModsDir` 等约 270 行，自行实现了：

- `mods` 目录识别；
- `.minecraft/versions/<id>` 与 Prism 两种布局；
- 从 `../<version>.json` 回退读取 manifest；
- `--dir` 的三种输入形态。

core 已有 `minecraft.LoadLauncherVersions`（`launcher.go`）、`prism.go` 布局、`CheckManifest`（`tool.go#L145`）等能力。CLI 在 `locateRuntimeDir` 内部甚至创建了临时 `appcore.New(appcore.Options{})` 只为调用 `LoadVersionsFromDisk`（`cliapp/app.go#L619`），同样的临时实例在 `inferVersionInfoForModsDir` 中还有一处（`#L703`），这是明显的适配层越界。

#### P1-4 配置双轨

- CLI：`cliapp/config.go` 的 `userConfig`，落在 `os.UserConfigDir()/mod-downloader-cli/config.json`，只支持 6 个 key。
- TUI/GUI：`core/configs` 的 `mod-downloader.toml`，包含主题、语言、下载并发、日志、代理等完整设置。
- CLI 用 `NoConfigFile: true` 跳过 TOML（`core/appcore/service.go#L1524`），但仍会按 `Runtime.WorkDir` 写日志（默认 CWD，`--dir` 时是 mods 目录）。
- 优先级在 CLI 里手写一遍（`cliapp/app.go#L529-L592`），又在 core 的 `ConfigOverrides` 里实现一遍。注意 flags/env 由 `urfave/cli` 在进入命令前解析，CLI 的 `userConfig` 实际只是兜底。

`minecraft-dir` 在 CLI 语义中更像“默认目标目录”，在 TOML 中是 GUI 的 MC 根目录；API key 也存在两处。用户切换 CLI/TUI 时会看到不同的“有效配置”。

#### P1-5 事件契约

`core/appcore/contracts.go#L15-L25` 定义了 8 个 core 事件；`internal/tuihost/host.go#L23-L35` 另外定义了 9 个事件，并有一个内部事件 `eventUITask`（`internal/tviewui/app.go#L407`）。两套类型不同、字符串值也不同，而且并非完全一一对应：extension 事件在 appcore 中没有对应项，`eventUITask` 则是适配器内部事件。`tuihost.emitCoreEvent` 用 `switch` + 类型断言翻译，未知 kind 或 payload 类型不匹配时静默丢弃（`host.go#L231-L260`）。`tviewui.handleEvent` 同样对每个 payload 做 `.(T)` 断言（`controller.go#L20-L72`）。

结果：事件契约的修改无法被编译器发现，新增事件忘记映射也不会报错。

---

## 3. 改进计划

### P0：先止血（低风险、可独立合入）

#### P0-1 日志治理与工作区产物

**目标**：日志可运维，单日日志量可控，工作区不再堆积运行产物。

**改动**：

1. `core/logging`：增加按大小轮转（默认 6 MB，保留当前文件 + 2 份历史，约 18 MB 上限，可配置），或引入成熟库。
2. 默认路径改为 `os.UserCacheDir()/mod-downloader/logs/`；新增 `[logging].file` 字段可覆盖（当前 `LoggingConfig` 没有该字段）；保留 `NoStderr` 语义。不再从 `Runtime.WorkDir` 推导日志路径。
3. 将 `global versions cache set`、`global selected version set`、`local mods cleared by instance`、`config saved` 降为 `Debug`，或在 1 秒窗口内聚合。
4. `prepareLogging` 与 `logging.Configure` 打印一次最终 path/level，便于排障。
5. 增加 `make clean-artifacts`（或脚本）清理 `*.log`、`mod-metadata.tmp`、`mod-favs.sqlite`、编译产物；`.gitignore` 已覆盖，无需改。

**验收**：

- TUI 连续运行 24 小时，日志目录 ≤ 20 MB，份数不超过配置上限。
- CWD 与目标 mods 目录都不再出现 `mod-downloader.log`（除非显式配置）。
- 现有 `TestRunKeepsStderrClean` 保持通过。

**风险**：日志路径变化需要更新 README 与 TUI 设置页文案；GUI 可能依赖旧路径，需要与 GUI 仓库确认后提供迁移说明。

#### P0-2 生命周期收口

**目标**：消除“半初始化 Service”和异常路径资源泄漏，为后续实例化铺路。

**改动**：

1. `Service.Startup` 幂等。建议用状态机而不是 `sync.Once`：`sync.Once` 会把一次失败永久固化，若需要重试语义则状态机更合适。重复调用返回同一结果。
2. 统一 `Close`/`Shutdown` 语义：保留一个“保存配置并停止全部资源”的入口，另一个作为兼容别名或显式传入是否保存；不要保留两个行为不同的公开入口。
3. `providers.StartBackgroundTasks/StopBackgroundTasks` 改为引用计数，或由 Service 实例持有。
4. `tuihost.Host.Service()` 不再返回未 `Startup` 的 Service。优先改为返回 `(*appcore.Service, error)` 或在 Host 上暴露 `Started()` 检查；直接返回 `nil` 会影响 `tviewui` 中 64 处 `host.Service()` 调用，需要统一加保护。
5. CLI `withService` 在 `Startup` 失败时也执行清理（先 `defer` 再 `Startup`，或显式调用 Close）。
6. 在 `core/appcore` 文档中写明“当前版本一个进程只支持一个 Service”。

**验收**：

- 新增 `TestServiceStartupShutdownIdempotent`、`TestServiceCloseAfterStartupFailure`。
- 重复调用 `Close`/`Shutdown` 不报错、不泄漏资源；`Startup` 失败后不残留后台任务与已打开文件。
- `go test -race ./...` 与 goleak 全绿。
- TUI 启动失败路径不再留下后台任务。

**风险**：`Close`/`Shutdown` 语义调整可能影响 GUI 调用点；先加注释并保留兼容行为，再在 GUI 仓库同步。

### P1：消除重复实现（下一迭代）

#### P1-1 统一 jar 解析

**目标**：仓库只保留一套 jar 元数据解析规则。

**改动**：

1. 将 `cliapp/jarmeta.go` 的版本范围判断（`versionRangeAllows` 等）移植到 `core/minecraft`，补单测。
2. `cliapp/verify.go` 改为只使用 `ScanModsDir` 产出的 `structs.ModInfo`（已含 `Dependencies`、`Loaders`、`JijMods`）；需要兼容性范围时，由 core 在解析阶段填充。
3. 扩展 core 的元数据模型或新增详情解析 API，覆盖 `info` 命令需要的 `License`、`Authors`、`Recommends`、`Breaks` 和 Minecraft 版本范围。
4. 删除 `cliapp/jarmeta.go` 与仅测试它的用例（先移植断言），并把 `info` 命令改为调用 core。

**验收**：

- `verify` 与 `info` 的 JSON 字段、表格输出和退出码不变。
- 每个 jar 只打开一次；用临时 jar 的单元测试覆盖 Fabric/Quilt/Forge/NeoForge/mcmod.info。
- `grep -r "parseJarMetadata" .` 为空。

**风险**：两套解析器对 `mods.toml` 版本范围的边界行为可能不同；先以现有 `jarmeta_test.go` 为基准做对照测试。`info` 的详情字段是新增契约，需确认 GUI 是否也需要同一份数据。

#### P1-2 下移 update / upgrade / verify 工作流

**目标**：CLI 只做参数与输出，业务编排回到 core，供 TUI/GUI 复用。

**改动**：

1. 在 `core/appcore` 新增（不修改旧签名）：
   - `CheckUpdates(req) []UpdateStatus`
   - `BuildUpgradePlan(req) []UpgradePlan`
   - `ApplyUpgradePlan(ctx, plan) []UpgradeResult`
   - `VerifyLocalMods(dir, mcVersion, loader, strict) VerifyReport`
2. 升级流程统一走 core 的旧 jar 归档与 `.disabled` 保持逻辑，删除 CLI 的 `removeSupersededFiles`。
3. `VerifyLocalMods` 基于 core 的 `structs.ModInfo` 实现；不要直接调用 `AnalyzeLocalModDisableImpact` 或 `ScanUnusedDependencies`，它们分别服务于禁用影响和未使用扫描，语义不同。
4. CLI 的 `update.go`/`verify.go` 只负责组装请求、渲染表格/JSON、映射退出码。

**验收**：

- CLI `check-update`/`upgrade --dry-run`/`verify` 输出与现有测试 golden 一致。
- 升级后旧 jar 只被处理一次，`.disabled` 状态保持。
- 新增 core 单测覆盖“下载失败保留旧文件”“旧文件已禁用”两个分支。

**风险**：core 方法增多会让 `Service` 更大；与 P2-1 的内部拆分一起做，先加方法后搬迁实现。

#### P1-3 目标探测下沉 core

**目标**：CLI/TUI/GUI 共用同一套实例/目录探测规则。

**改动**：

1. 在 `core/minecraft` 新增纯函数，例如 `ResolveModsTarget(input string) (modsDir string, info VersionInfo, confidence, error)`，覆盖：
   - `mods` 目录 / 版本目录 / `.minecraft` 根 / Prism 实例根；
   - `--dir` 与 CWD 两种入口；
   - manifest 回退。
2. `cliapp/app.go` 的 `locateRuntimeDir` 等函数改为薄封装，删除 `#L619` 与 `#L703` 两处临时 `appcore.New`。
3. 把现有 CLI `dir_test.go`/`target_test.go` 的路径矩阵迁到 core。

**验收**：

- 路径矩阵测试全部在 core；CLI 侧只保留参数解析测试。
- `cliapp/app.go` 不再包含自己实现的目标目录/版本探测逻辑；`cliapp` 对 `minecraft` 的其它使用（如 `ScanModsDir`、`FileSHA1`）保留。

**风险**：探测顺序变化会改变“自动识别到的版本”；用现有测试矩阵锁定行为。

#### P1-4 配置单一解析层

**目标**：同一设置只有一个“有效值”和一个来源，CLI/TUI 不再各算一遍优先级。

**改动**（推荐保守方案，不做文件格式迁移）：

1. 在 core 新增 `EffectiveSettings`（或 `SettingsResolver`）：按 `defaults → TOML → CLI JSON → 已解析的 flag/env` 计算，并记录每个值的来源。注意 flags/env 已由 `urfave/cli` 在进入命令前解析，resolver 不应重新实现 env 优先级。
2. CLI 的 6 个 key 映射到 resolver；`minecraft-dir` 明确为“默认目标目录”，与 TOML 的 `prefers.minecraft_dir` 区分命名。
3. API key 只保留一个有效来源：CLI key 作为进程级 override，不回写 TOML；文档写清楚。
4. `config list` 增加来源列（可选，`--json` 增加 `source` 字段）。

**验收**：

- 表驱动测试覆盖每一层优先级与空值回退。
- 同一设置在 CLI `config list` 与 TUI Settings 页显示一致的值。
- README 的“优先级”章节只描述 resolver 一处。

**风险**：`minecraft-dir` 语义调整可能影响现有用户；保留旧 key 并加迁移提示。

#### P1-5 事件契约统一

**目标**：core 事件只有一个定义、编译期可查、不再静默丢事件；适配器内部事件与 core 事件明确分离。

**改动**：

1. core 事件只保留一处定义（`appcore.Event` 或独立 `contracts` 包），`tuihost` 直接引用，不再维护第二套同名常量。
2. 适配器内部事件单独命名（例如 `localEventKind`），保留 `eventUITask`；如果希望 extension 结果也成为 core 事件，在 `appcore` 新增 `EventExtensionModsAccepted`，否则继续留在适配器层。不要把两类事件强行合并成一个枚举。
3. 在 `tuihost.emitCoreEvent` 与 `tviewui.handleEvent` 增加 `default` 分支：记录 `Warn`（含 kind 和 payload 类型），不再静默 return。
4. 为每个事件 kind 增加一条映射测试，防止新增事件漏接。

**验收**：

- core 事件只有一个 `EventKind` 定义；适配器内部事件不再伪装成 core 事件。
- 未知事件在测试中触发 warn；事件映射测试覆盖全部 kind。
- 事件表与 `docs/specs/tui/host-adapter.md` 一致。

**风险**：`appcore.Event` 的 payload 仍是 `any`；本次不强行泛型化，只保证默认分支可见。extension 事件归属需要在 core/GUI/TUI 之间达成一致。

### P2：结构性改造（中期，需与 GUI 协同）

#### P2-1 `appcore.Service` 内部分解

**目标**：保持 `Service` 的公开方法不变，降低单文件复杂度。

**改动**：

1. 在 `core/appcore` 内部按现有方法组拆出未导出子系统：
   - `searchService`（search/pinning）
   - `favoritesService`
   - `settingsService`
   - `downloadsService`
   - `instancesService`
2. `Service` 只保留 facade、依赖装配和事件转发；每个子系统接收显式依赖（config、store、providers、downloader）。
3. 先搬实现、后删旧方法体，确保 GUI 编译不受影响。

**验收**：

- `service.go` < 600 行，各子系统文件 < 500 行。
- 导出方法签名零变化；GUI/CLI 编译通过；core 测试全绿。
- `core/appcore` 的共享状态与初始化顺序有明确归属，不再依赖隐蔽的包级变量。

**风险**：搬迁期间容易出现循环依赖；按方法组逐个提交，禁止一次性大改。

#### P2-2 core 全局状态实例化

**目标**：`downloader` / `storage` / `global` / `logging` 从包级单例变成可注入实例。

**改动**：

1. `downloader.Manager`：持有 `downloadQueue`、并发配置与事件回调；包级函数暂时保留为默认实例的 wrapper。
2. `storage.Store`：持有 metadata cache 与 user DB；`OpenAt/Close` 变为方法；包级 wrapper 标记 deprecated。注意当前 `storage.OpenAt` 在路径变化时会关闭前一个全局 DB，进一步说明多实例必须一起改造。
3. `global.State`：minecraftDir/versions/clients/selectedVersion 收进结构体，由 Service 持有。
4. `logging.Logger` 实例化，注入到 core 包（或至少保留全局但允许 override）。

**验收**：

- 同一进程创建两个 Service，互不干扰（新测试）。
- 现有 CLI/TUI 行为不变；GUI 迁移完成后删除 wrapper。

**风险**：改动面最大，必须放在 P2-1 之后；与 GUI 仓库协调一个大版本。

#### P2-3 TUI App 方法拆分

**目标**：`App` 只做 shell，业务方法按域拆分。

**改动**：

1. 保留 `App` 的布局、焦点、状态栏、队列 dock、事件泵、busy/spinner。
2. 把 `controller.go` 中的 62 个 `*App` 方法按域拆到 `downloadController`、`manageController`、`favoritesController`、`settingsController`、`instanceController`，各自持有 `*App` 与 `*appcore.Service`，实现本域方法。当前并不存在 `Controller` 类型，拆分后也不建议为拆分而新增一层同名抽象。
3. 页面只保留纯 UI 局部状态（焦点、选中、草稿），业务状态继续来自 `uistate`。
4. `controller.go` 只保留事件分发与跨域协调。

**验收**：

- `controller.go` < 300 行；各 controller < 300 行。
- 所有现有 golden 与交互测试不改期望值。
- `go test -race` 通过。

**风险**：跨域调用（如下载完成刷新管理页）需要明确接口；先画调用表再拆。

#### P2-4 i18n 外置与校验

**目标**：文案改动不再修改 Go 代码，并有完整性测试。

**改动**：

1. `zh`/`en` map 移到 `internal/i18n/locales/{zh,en}.json`，`go:embed` 加载。
2. 增加 `TestLocaleKeysMatch`（两个语言 key 集合一致）与 `TestUsedKeysExist`（扫描 `i18n.T("...")` 字面量）。
3. 保留 `i18n.T` API 不变。

**验收**：所有 golden 不变；缺 key 的测试会失败；新增文案只改 JSON。

**风险**：动态拼接的 key 无法静态扫描；先用白名单，逐步收敛。

#### P2-5 工程化、CI 与文档

**目标**：双 module 的构建/测试/发布可重复，文档与代码一致。

**改动**：

1. 增加 `Makefile` 或 `Taskfile`：
   - `build`、`test-cli`、`test-core`、`test-all`、`test-race`、`vet`、`golden-update`、`clean-artifacts`。
2. GitHub Actions：checkout submodules、Go 1.25（core 兼容 1.24）、`go vet ./...` + `go test ./...`（两个 module）、`go test -race ./...`、goleak。
3. README：明确“CLI 需要 Go 1.25，core 需要 Go 1.24”；补充 TUI 日志路径、配置来源；链接本计划。
4. 在 README 和本计划中记录基线 commit 与统计命令，避免后续数字漂移。
5. `TODO.md` 已完成，可作为历史归档；后续用本计划的 P0/P1/P2 跟踪。

**验收**：CI 从零 clone（含 submodule）可跑通；README 命令可复制执行；文档中的行数/包数可用第 7 节的命令复现。

**风险**：CI 首次接入会有环境差异；先只跑 build/vet/test，不同时上 lint。

#### P2-6 契约类型归位

**目标**：适配器不依赖 `storage` 持久层类型。

**改动**：

1. 把 `PinnedMod`、`FavoriteList`、`FavoriteMod`、`FavoriteModEntry` 等对外 DTO 移到 `core/contracts` 或 `core/structs`。
2. `storage` 只保留内部行结构；`appcore` 返回对外 DTO。
3. TUI/CLI 更新 import；GUI 同步。

**验收**：`internal/tviewui` 不再 import `core/storage`（或仅限内部测试）。

**风险**：影响 GUI 绑定类型；与 core 版本发布绑定。

---

## 4. 落地顺序

```text
第 1 步（独立、低风险）
  P0-1 日志治理
  P0-2 生命周期收口

第 2 步（消除重复，收益最高）
  P1-1 统一 jar 解析（含 info 元数据）
  P1-3 目标探测下沉
  P1-5 事件契约统一

第 3 步（工作流归位）
  P1-2 update/upgrade/verify 下移（依赖 P1-1）
  P1-4 配置解析层

第 4 步（结构性改造，需 GUI 协同）
  P2-1 appcore 内部分解
  → P2-2 core 全局状态实例化
  → P2-3 TUI App 方法拆分

随时可做
  P2-4 i18n 外置
  P2-5 CI / Makefile / 文档
  P2-6 契约类型归位（与 GUI 版本绑定）
```

原则：**每一步都保持 CLI 输出、TUI golden、core 测试不变**；先加新入口，再迁调用方，最后删旧实现。避免“大爆炸式重构”。

---

## 5. 验收指标

| 指标 | 当前 | 目标 |
| :--- | :--- | :--- |
| 日志体积（当前文件） | 392 MB / 2.76 M 行 | ≤ 20 MB，自动轮转 |
| CWD / mods 目录运行产物 | log/binary/sqlite/tmp | 仅显式配置时产生 |
| jar 解析实现数量 | 2 套 | 1 套 |
| `cliapp` 生产代码 | 4,766 行 | < 4,000 行（P1 完成后） |
| `core/appcore/service.go` | 2,004 行 | < 600 行（P2-1 后） |
| `internal/tviewui/controller.go` | 925 行；`App` 共 121 个方法 | < 300 行（P2-3 后） |
| 事件静默丢弃 | 可能发生 | 0，未知事件必须告警 |
| 配置解析入口 | 2 套（CLI/core） | 1 个 resolver |
| CI | 无 | 双 module build/vet/test/race 全绿 |
| 测试基线 | `go test ./...` 全绿 | 保持不变，新增 `-race` 与 goleak |

---

## 6. 明确不做

- 不重写 TUI，不迁回 Bubble Tea；`tview` 的布局、golden 和事件泵保留。
- 不合并 `huh` 向导与 `tview`：前者是行内向导，后者是全屏 TUI，职责不同。
- 不把 `appcore` 拆成微服务或多进程；它只是进程内 facade。
- 不为了“行数好看”拆文件；拆分必须按职责边界。
- 不强制 CLI 迁移到 `mod-downloader.toml`；先统一 resolver 与语义，文件格式迁移另立议题。
- 不在本次改动 GUI 仓库；所有 core 变更以向后兼容为先，破坏性变更单独协同。

---

## 7. 分析依据

- 基线提交：`6b15602`（`git rev-parse --short HEAD`）。
- `go build ./...`：CLI 与 core 均通过。
- `go vet ./...`：CLI 与 core 均通过。
- `go test ./...`：CLI 8 个包、core 16 个包全部通过（`go list ./...` 统计；core 含 `structs/minecraft`）。
- 日志统计：`mod-downloader.log` 392,932,349 字节、2,760,806 行；`tail -n 200000` 后 `global selected version set` 65,892 次、`global versions cache set` 65,872 次、`local mods cleared by instance` 65,806 次。
- 代码规模：CLI 生产代码约 15.8k 行（根目录合计 15,785 行：`cliapp` 4,766 行 + `tviewui` 8,420 行 + `uistate` 1,214 行 + `i18n` 796 行 + `tuihost` 269 行 + `cmd` 50 行 + `cliapp` 子包 270 行），core 生产代码 16,760 行，`core` 测试约 9,367 行。
- 方法数：`appcore/service.go` 97 个 `*Service` 方法（73 个导出），`core/appcore` 全包 104 个（80 个导出）；`*App` 方法共 121 个，其中 `controller.go` 62 个。
- 统计命令：行数用 `wc -l`（排除 `*_test.go`），包数用 `go list ./...`，方法数用 `grep -rhoE '^func \((s|a) \*(Service|App)\)' <files> | wc -l` 或按文件汇总。
