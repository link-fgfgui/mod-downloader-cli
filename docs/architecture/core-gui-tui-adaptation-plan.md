# Core 统一后的 GUI/TUI 适配架构计划

> 日期：2026-11-01
> 范围：`mod-downloader-cli`（CLI + TUI）、`mod-downloader-gui-v3`（Wails GUI + 共享 core）
> 基线：`core` 已从 git submodule 切换为 sibling symlink；CLI 导入路径已与 GUI 统一为 `github.com/link-fgfgui/mod-downloader/core`；CLI `go build/vet/test`、core `go test` 全部通过。

## 0. 结论

core 现在只有一份：本仓库的 `core` 指向 `../mod-downloader-gui-v3/core`。**不需要重写领域层。**

真正需要收口的是 GUI 与 TUI 之间两套并行适配代码：

1. **生命周期 / HTTP / 事件桥接**：GUI `app.go` 与 TUI `internal/tuihost` 各有一套，逻辑约 90% 相同。
2. **事件契约**：`appcore.EventKind`、GUI 事件名常量、`tuihost.EventKind` 三套定义；适配器还要再做一次 `any` 断言。
3. **会写回 core 的纯决策**：设置边界、收藏排序/作用域、Manage 分组与兼容性判定，在 core、TUI `uistate`、GUI 前端各有一份。

原则：**共享“决策”，不共享“渲染投影”**。凡是会影响 core 状态的规则必须只有一份 Go 实现；纯展示的分页、焦点、抽屉、动画留在各 UI。

非目标：不统一 React 与 tview 的渲染层；不把每帧都要用的格式化/高亮改成 RPC；不在本阶段做 `appcore.Service` 多实例化。

## 1. 已完成：core 单一来源（Phase 0）

| 项目 | 之前 | 现在 |
| :--- | :--- | :--- |
| 依赖方式 | git submodule `mod-downloader-core`（b97c96c） | symlink `core -> ../mod-downloader-gui-v3/core` |
| `.gitmodules` | 存在 | 已删除 |
| CLI 模块路径 | `github.com/link-fgfgui/mod-downloader-core` | `github.com/link-fgfgui/mod-downloader/core`（与 GUI 一致） |
| `go.mod` | `replace ...mod-downloader-core => ./core` | `require github.com/link-fgfgui/mod-downloader/core v0.0.0` + `replace ... => ./core` |
| 导入点 | 67 个 Go 文件 | 全部改为 `.../mod-downloader/core/...` |

验证：

```sh
go build ./...        # CLI
go vet ./...          # CLI
go test ./...         # CLI: cliapp / tviewui / uistate / tuihost 全绿
cd core && go test ./...   # core 全绿
```

残留事项：

- symlink 是**阶段性方案**，只在两个仓库保持 sibling 布局时有效；Windows 检出需要 `core.symlinks=true`。
- CI 需要同时 checkout GUI 仓库，或提供恢复 symlink 的 bootstrap 脚本。
- core 稳定后应改为 `go.work`（开发期）或带版本的正式模块（发布期），见 Phase 6。

## 1.1 已完成：Phase 5（Manage 分组/兼容性下沉）

| 项目 | 之前 | 现在 |
| :--- | :--- | :--- |
| 共享投影类型 | GUI `ModGroup` / TUI `uistate.ModGroup` | `core/structs.LocalModGroup`（含 `DisplayName` / `Categories` / `Loaders` / `SearchText` / `Incompatible`） |
| 分组与判定 | GUI `lib/modGroups.ts` `groupMods` + TUI `uistate.BuildGroups` | `appcore.BuildLocalModGroups(mods, selectedLoader)` |
| 读取入口 | — | `appcore.Service.GetLocalModGroups(versionKey, modLoader)`；版本已选中时复用缓存扫描，不落盘 |
| GUI 绑定 | — | `App.GetLocalModGroups(versionKey, modLoader)`（Wails 绑定已重新生成） |
| TUI | `uistate` 内 Jij 去重/兼容性/搜索文本 | `uistate.ModGroup = appstructs.LocalModGroup`；`uistate` 只保留行 key、过滤、分区、批量辅助 |
| GUI 前端 | `lib/modGroups.ts`（99 行） | 删除；新增 `lib/modGroupDisplay.ts`（行 key、tooltip、pinyin 搜索文本） |

搜索文本统一为“人类可读字段”（id/name/onlineName/version/fileName/description/categories/Jij），不再索引 `path` 与依赖声明；这修复了实例路径导致所有 mod 命中版本号查询的问题。GUI 额外叠加拼音首字母，属于展示层增强。

验证：

```sh
rg "groupMods|BuildGroups" internal ../mod-downloader-gui-v3/frontend/src   # 0 命中
go build ./... && go vet ./... && go test ./...                              # CLI
cd core && go test ./...                                                     # core（新增 localmodgroups_test.go）
cd ../mod-downloader-gui-v3 && go build . && go build -tags server . && go test .
cd frontend && npx tsc --noEmit && npx vitest run && npm run build
```

## 1.2 已完成：Phase 1–4（2026-11-01）

| Phase | 结果 |
| :--- | :--- |
| 1 — `core/host` | 新增 `core/host/host.go`：`Options`（含 `Runtime` / `DisableHTTPServer` / `HTTPAddr`）、`Start`/`Shutdown`/`Service`/`Config`/`Version`/`Started`/`StartupStatus`/`ServerNotice`；`internal/tuihost` 删除，TUI 与 GUI 共用同一生命周期；启动失败、端口占用、重复 Start/Shutdown 行为与之前一致（测试已迁移）。 |
| 2 — 事件契约 | `appcore.EventKind.Name()` + `EventName*` 常量；`host.On[T]` / `host.OnEvent` 做唯一一次类型断言；删除 `tuihost.EventKind`/`Event`、GUI 事件名常量与 `emitCoreEvent` switch；`tviewui` 直接注册 typed handler。Wails 绑定生成器可求值跨包常量，`frontend/bindings/events` 仍生成 9 个事件。 |
| 3 — 设置规范化 | `configs.NormalizeNetworkSettings` / `NormalizeSettingsView` / `NormalizeAnimationDurationMultiplier`；`Service.GetSettings` 返回规范化快照，`SaveNetworkSettings` 复用同一函数；TUI 删除 clamp 常量与函数；GUI `lib/animations.ts` 不再重复边界（滑块范围仅作输入体验）。TUI release 下拉补齐 `latest`，与 core 默认值及 GUI 一致。 |
| 4 — 收藏投影 | `structs.FavoriteOverview`；`appcore.BuildFavoriteOverview` / `FilterFavoriteContents` / `FavoriteListMatchesScope` + `Service.ListFavoriteOverview` / `ListFavoriteContentsForScope`；排序统一为 `pinned DESC, sort_order ASC, created_at ASC, name ASC`；TUI 删除 `uistate` 镜像函数；GUI 删除 `listOrder`/`listMatchesScope`/`favoriteContentsForScope`，Favorites 页与 AddToFavoriteDialog 消费 core 投影。 |

验证：

```sh
rg "EventKind" internal ../mod-downloader-gui-v3/*.go        # 0 命中
rg "event\.Payload\.\(" internal ../mod-downloader-gui-v3/*.go   # 0 命中
rg "MinFileConcurrency|MinConcurrentDownloads|MaxTargetDownloadRateMiB" internal ../mod-downloader-gui-v3/*.go  # 0 命中
rg "listOrder|listMatchesScope|PartitionLists" internal ../mod-downloader-gui-v3/frontend/src  # 0 命中
go build ./... && go vet ./... && go test ./...                       # CLI
(cd core && go build ./... && go vet ./... && go test ./...)          # core
cd ../mod-downloader-gui-v3 && go build . && go build -tags server . && go test .
cd frontend && npx tsc --noEmit && npx vitest run && npm run build
```

## 1.3 已完成：Phase 6（CLI 瘦身与模块策略，2026-11-01）

| 项目 | 之前 | 现在 |
| :--- | :--- | :--- |
| jar 元数据 | `cliapp/jarmeta.go`（711 行）独立实现 Fabric/Quilt/Forge/NeoForge/mcmod.info | 删除；`core/minecraft/modparser.go` 统一解析，新增 Quilt 与 mcmod.info 解析器 |
| `ModInfo` 字段 | 仅 ID/Name/Version/Description/Dependencies | 补齐 `Authors` / `License` / `Environment` / `MinecraftVersion` / `Provides` / `Recommends` / `Breaks` |
| 版本范围 | `cliapp.versionRangeAllows` | `minecraft.VersionRangeAllows`（Maven/`~`/`^`/`.x`/复合范围，含单测） |
| `verify` | 先 `ScanModsDir` 再逐 jar 重新解析 | `appcore.VerifyLocalMods(modsDir, mods, mc, loader, strict)`，只解析一次 |
| `info` | 依赖 `jarmeta.go` 的私有结构 | `minecraft.ParseLocalModJarWithSHA1` + `structs.ModInfo` |
| `upgrade` 清理 | CLI 自行删除旧 jar 与 `.old*` 备份 | 复用 downloader 安装时的归档逻辑，保留备份 |
| 目标探测 | 临时 `appcore.New(appcore.Options{}).LoadVersionsFromDisk` | `minecraft.LoadLauncherVersions(root, minecraft.LoadGameDirVersions)`；`minecraft.ValidInstance` 下沉 |
| 模块策略 | 仅 symlink + `replace` | 新增 `go.work`（CLI + GUI + core）、`scripts/bootstrap-core.sh`、`make bootstrap` / `make workspace`；README 记录 Windows `core.symlinks=true`、CI 双仓 checkout 与发布期 tag 策略 |

验证：

```sh
rg "parseJarMetadata|jarMetadata|jarDependency|versionRangeAllows" cliapp/   # 0 命中
rg "appcore\.New\(appcore\.Options\{\}\)" cliapp/ --glob '!*_test.go'   # 0 命中
go build ./... && go vet ./... && go test ./...                              # CLI
go test github.com/link-fgfgui/mod-downloader/core/...                       # core（workspace）
make bootstrap && make workspace                                             # symlink 兜底 + go.work
```

## 2. 冗余清单（实测）

| 区域 | GUI 侧 | TUI 侧 | core 现状 | 重复内容 |
| :--- | :--- | :--- | :--- | :--- |
| 启动/关闭/HTTP | `app.go` `ServiceStartup`/`ServiceShutdown`、`startupErr`/`serverErr` | `internal/tuihost/host.go`（306 行） | `appcore.Service.Startup/Shutdown` + `httpserver` | 两套几乎相同的装配、错误缓存、HTTP 启停与 notice |
| 事件契约 | `app.go` 9 个事件名常量 + `emitCoreEvent` switch；`app_events.go` 9 个 `RegisterEvent` | `tuihost.EventKind` 9 个常量 + `emitCoreEvent` switch；`tviewui/controller.go` 再断言一次 | `appcore.EventKind` 9 个常量 | 3 套枚举、2 层 `any` 断言、事件名靠手写字符串保持一致 |
| 设置校验 | `stores/settings.ts`（426 行）+ `lib/animations.ts` 再 clamp | `uistate/settings.go`（113 行）再 clamp | `configs` 边界 + `Save*` 已 clamp | 三份边界常量与 NaN/Inf/精度规则 |
| 收藏顺序/作用域 | `stores/favorites.ts`（410 行）`listOrder` / `listMatchesScope` / 内容过滤 | `uistate/favorites.go`（103 行）同名镜像函数 | SQL 已 `ORDER BY pinned DESC, sort_order ASC, created_at ASC, name ASC` | 两份排序/过滤；GUI/TUI 都漏了 `created_at` 平局规则 |
| Manage 分组/兼容性 | `lib/modGroups.ts`（99 行）+ `pages/Manage.tsx`（1,185 行）内联判定 | `uistate/manage.go`（404 行）`BuildGroups` 等 | `service_localmods.go` 只返回扁平 `[]ModInfo` | Go 与 TS 各一份分组、Jij 去重、兼容性、搜索文本 |
| CLI 领域逻辑 | — | — | core 已有 jar 解析/实例探测/更新归档 | `cliapp/jarmeta.go`（711 行）等与 core 重叠（Phase 6） |

规模参考：`internal/tuihost` 生产代码 306 行、测试 422 行；`internal/uistate` 生产代码 1,168 行；GUI `app.go` 595 行；GUI 前端 `stores/` 约 1,934 行。

## 3. 目标架构

```text
core/
  appcore/          领域 facade（既有公开方法签名保持不变）
  host/             新增：UI 无关的 Service + HTTP + 类型化事件分发
  structs/          新增共享投影类型（ModGroup / FavoriteOverview / SettingsView 扩展）
  ...               现有领域包

mod-downloader-gui-v3/
  app.go            Wails 绑定 + 原生对话框 + host 装配（不再持有生命周期）
  app_events.go     只做 core 事件 → Wails 事件名的映射与注册
  frontend/         React 渲染与本地 UI 缓存（不再实现领域排序/分组/边界）

mod-downloader-cli/
  internal/tuihost  删除，改用 core/host
  internal/tviewui  渲染 / 输入 / 事件泵；注册类型化 handler
  internal/uistate  只保留 UI 状态（分页、抽屉、焦点、表单草稿、多选）
  cliapp            参数、输出、退出码（Phase 6 再瘦身）
```

依赖规则：

- `core` 不得导入任何适配器包；适配器可以导入 `core/*`。
- 事件 kind 与事件名只在 `appcore` 定义一次；适配器不得再定义同名枚举。
- 适配器不得重新实现会写回 core 的规则；只允许展示层投影。
- `appcore.Service` 仍是进程级单例；`core/host` 只封装一个 Service，不改变该约束。

## 4. 分阶段计划

### Phase 1 — 提取 `core/host`（P0）

**目标**：GUI 与 TUI 共用一套启动、关闭、HTTP、事件分发；删除 `internal/tuihost`。

新增 `core/host/host.go`：

```go
package host

type Options struct {
    Version               string
    LoadMinecraftReleases bool
    Config                *configs.Config
    Runtime               appcore.RuntimeOptions
    LogLevel              slog.Level
    LogToFileOnly         bool
    DisableHTTPServer     bool   // CLI 一次性命令设为 true
    HTTPAddr              string // 空值使用 httpserver.DefaultAddr
}

type Host struct{ /* core, server, config cache, notice, mutex */ }

func New(opts Options) *Host
func (h *Host) Start(ctx context.Context) error
func (h *Host) Shutdown()
func (h *Host) Service() *appcore.Service
func (h *Host) Config() *configs.Config
func (h *Host) Version() string
func (h *Host) Started() bool
func (h *Host) StartupStatus() appstructs.StartupStatus
func (h *Host) ServerNotice() string
```

迁移动作：

- `internal/tuihost/host.go` 的启动顺序、失败清理、`startMu` 幂等、`serverNotice` 逻辑整体移入 `core/host`；测试一并迁移。
- `tviewui.Run` 改为 `host.New(...)`，`App` 持有 `*host.Host`，`service()` 返回 `host.Service()`。
- GUI `ServiceStartup` 只保留 `host.New` + `host.Start`；`ServiceShutdown` 只调用 `host.Shutdown`；`GetStartupStatus` 改为 `host.StartupStatus()`。
- GUI 的 `appRuntimeOptions()`（`os.Getwd()`）与 TUI 的同名函数合并为 `host` 默认值。

验收：`internal/tuihost` 目录删除；GUI/TUI 的启动失败、HTTP 端口占用、重复 Start/Shutdown 行为与现在一致。

### Phase 2 — 事件契约单一来源（P0，与 Phase 1 同批）

**目标**：core 只保留一套事件定义；适配器不再定义枚举，也不做 `any` 断言。

`appcore` 增加：

```go
// Name 返回稳定的线缆名（保持现有 kebab-case，前端事件名不变）。
func (k EventKind) Name() string

// 类型化订阅；断言与告警只在 host 内做一次。
func On[T any](h *host.Host, kind appcore.EventKind, fn func(T))
```

迁移动作：

- 删除 `tuihost.EventKind`、`tuihost.Event` 与 `emitPayload`；`tviewui` 直接注册 `host.On(appcore.EventSearchModsUpdated, func(u appstructs.SearchModsUpdate) {...})`。
- GUI 删除 `app.go` 的 9 个事件名常量；`app_events.go` 用 `appcore.EventX.Name()` 注册；`emitCoreEvent` 改为 `host.On` 注册的薄映射。
- 保留未知事件告警，但只保留 `core/host` 一处实现。
- `tviewui/controller.go` 删除第二层类型断言。

验收：`rg "EventKind"` 只命中 `core/appcore` 与 `core/host`；GUI/TUI 不再出现 `event.Payload.(`。

### Phase 3 — 设置规范化下沉（P1）

**目标**：边界与规范化只存在于 `core/configs`。

`configs` 导出：

```go
func NormalizeNetworkSettings(req appstructs.SaveNetworkSettingsRequest) appstructs.SaveNetworkSettingsRequest
func NormalizeAnimationDurationMultiplier(v float64) float64
func NormalizeSettingsView(view appstructs.SettingsView) appstructs.SettingsView
```

迁移动作：

- `Service.GetSettings` 返回已规范化的 `SettingsView`；`SaveNetworkSettings` 复用同一个函数。
- TUI `uistate/settings.go` 删除 `Min/Max/Default` 常量、`clampInt`、`clampRate`，只保留草稿结构与请求映射。
- GUI 删除 `settings.ts` 的数值 clamp 与 `lib/animations.ts` 的重复边界；`GetSettings` 已是规范值，前端只负责输入体验。
- 保持 `configs.Min*/Max*` 常量作为唯一来源。

验收：`rg "MinFileConcurrency|MinConcurrentDownloads|MaxTargetDownloadRateMiB"` 在适配器为 0 命中。

### Phase 4 — 收藏投影下沉（P1）

**目标**：排序、作用域过滤、pinned/ungrouped 分区只由 core 决定。

`appcore` 增加（保留旧方法，不破坏现有绑定）：

```go
func (s *Service) ListFavoriteOverview(mcVersion, modLoader string) appstructs.FavoriteOverview

type FavoriteOverview struct {
    Pinned    []storage.FavoriteList
    Ungrouped []storage.FavoriteList
}
func (s *Service) ListFavoriteContentsForScope(listID, mcVersion, modLoader string) storage.FavoriteListContents
```

迁移动作：

- core 统一排序规则为 `pinned DESC, sort_order ASC, created_at ASC, name ASC`，与 SQL 对齐。
- TUI 删除 `uistate/favorites.go` 的 `ListOrder`/`SortLists`/`PartitionLists`/`FavoriteContentsForScope`，页面直接使用 overview。
- GUI 删除 `stores/favorites.ts` 的 `listOrder`/`listMatchesScope`/`favoriteContentsForScope`，改为消费 overview。
- 更新 TUI 单元测试为 core 侧纯函数测试。

验收：`rg "listOrder|listMatchesScope|PartitionLists|FavoriteContentsForScope"` 只命中 core。

### Phase 5 — Manage 分组/兼容性下沉（P1）

**目标**：同一份 mod 分组与兼容性判定同时服务 GUI 与 TUI。

`core/structs` 增加：

```go
type LocalModGroup struct {
    Primary       minecraft.ModInfo
    Strong        []minecraft.ModInfo
    Jij           []minecraft.JijModInfo
    DisplayName   string
    Categories    []string
    Loaders       []string
    SearchText    string
    Incompatible  bool
}
```

`appcore` 增加：

```go
func BuildLocalModGroups(mods []minecraft.ModInfo, selectedLoader string) []LocalModGroup
```

迁移动作：

- TUI `uistate/manage.go` 保留多选、行 key、toggle 行等 UI 状态；删除 `BuildGroups`/Jij 去重/兼容性/搜索文本逻辑，改调 core。
- GUI 删除 `lib/modGroups.ts`；`Manage.tsx` 通过新增绑定 `GetLocalModGroups(versionKey, modLoader)` 获取分组；只保留纯展示 helper（tooltip 文案等）。
- 若担心 RPC 体积，可先让前端在 `RefreshSelectedVersionMods` 的返回值上附带 groups；不新增每帧 RPC。

验收：`rg "groupMods|BuildGroups"` 在适配器为 0 命中；GUI/TUI 的 incompatible 分区与 Jij 展示一致。

### Phase 6 — CLI 与模块策略（P2）

**CLI 瘦身**（沿用 `architecture-improvement-plan.md` 的 P1-1..P1-4）：

- `cliapp/jarmeta.go`（711 行）删除，改用 `core/minecraft/modparser.go`；先补齐 `ModInfo` 的 License/Authors/Recommends/Breaks 字段。
- `cliapp/verify.go` 的依赖/版本范围检查下沉为 `appcore.VerifyLocalMods`。
- `cliapp/update.go` 的旧 jar 归档改为复用 `downloader` 的既有逻辑。
- `cliapp` 的目标探测改为调用 `minecraft` 的 launcher/prism API，删除临时 `appcore.New`。

**模块策略**：

1. 开发期：`go.work` 同时包含 CLI 与 GUI 模块，symlink 只作为兜底。
2. 发布期：core 打 tag，CLI/GUI 引用同一版本，移除 `replace`。
3. CI：checkout 两个仓库 + 恢复 symlink 的脚本；Windows 明确 `core.symlinks=true`。

## 5. 验收标准

- `core` 不导入任何适配器包；事件 kind 只在 `appcore` 定义。
- GUI/TUI 适配器不再包含设置边界、收藏排序、mod 分组、兼容性判定。
- `go build ./...`、`go vet ./...`、`go test ./...`（CLI）、`go test ./...`（core）、GUI `go build .` 与 `go build -tags server .` 全部通过。
- TUI golden、事件泵/goleak、GUI 前端相关测试保持通过；新增 core 纯函数测试覆盖迁移出的规则。
- 行为一致性抽查：设置非法值、收藏跨作用域、Jij 去重、incompatible 分区在 GUI/TUI 输出一致。

## 6. 风险与取舍

| 风险 | 影响 | 缓解 |
| :--- | :--- | :--- |
| core 改动同时影响 GUI | GUI 编译/行为回归 | 每个 Phase 先在 core 加 API，再改 GUI/TUI；GUI 全量构建 |
| Wails 绑定面变化 | 前端需同步 | 新方法只增不改；旧方法保留到前端迁移完成 |
| 事件线缆名变化 | 前端收不到事件 | `EventKind.Name()` 固定为现有 kebab-case，并加契约测试 |
| 分组结构体变大 | RPC 体积略增 | 只随版本刷新返回一次，不做每帧调用 |
| symlink 不可移植 | Windows/CI 检出失败 | bootstrap 脚本 + `core.symlinks=true`；最终改 `go.work`/版本化模块 |

## 7. 建议执行顺序

1. Phase 1 + Phase 2（同一批，先删 `internal/tuihost`，收益最大）。
2. Phase 3（低风险，纯函数下沉）。
3. Phase 4（收藏，需同步 GUI store）。
4. Phase 5（Manage，改动面最大，最后做）。
5. Phase 6（CLI 瘦身与模块策略，独立推进）。

预计净删除约 450–550 行适配代码（不含测试），并把 3 套事件枚举、3 套设置边界、2 套收藏排序、2 套分组算法收敛为各 1 套。
