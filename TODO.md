# TODO: mod-downloader-cli 功能完善规划

本文档记录 `mod-downloader-cli` 的全量功能完善开发任务清单。所有功能均采用单清单模式推进，涵盖语法规范、标志位定义、运行逻辑及验收标准。

---

## 待办清单

### 1. 全局参数与目录定位增强 (`--dir` / `-C`)
- [x] **支持全局指定目标目录**
  - **命令参数**：全局标志 `--dir <path>`，别名 `-C <path>`。
  - **设计规范**：
    - 允许指向目标 `mods` 文件夹，或其父级 Minecraft 实例目录（例如 `.minecraft/versions/<version>` 或 Prism Launcher 实例目录）。
    - 若传入实例目录，自动向下探测是否存在 `mods/` 子文件夹；若不存在可根据参数创建或提示。
    - 自动从该目录所在层级继承或向上探测 Minecraft 版本与 Loader，无需手动 `cd` 进 `mods` 目录。
    - 优先级规则：显式命令行参数 (`--mc-version`/`--loader`) > 探测值 > 默认配置。
  - **验收标准**：
    - `mod-downloader-cli --dir /path/to/instance list` 可正确列出该实例下的模组。
    - `mod-downloader-cli -C /path/to/instance/mods install modrinth:sodium` 可正常下载安装至该目录。
    - 单元测试覆盖各类路径传入形态（绝对路径、相对路径、实例根路径、`mods` 子路径）。

---

### 2. 本地模组启用与禁用 (`enable` / `disable`)
- [x] **实现 `enable` 与 `disable` 命令**
  - **命令语法**：
    - `mod-downloader-cli enable [--dry-run] [--force] <mod...>`
    - `mod-downloader-cli disable [--dry-run] [--force] <mod...>`
  - **设计规范**：
    - 基于社区标准规范，通过重命名文件实现：
      - `disable`：将 `<file>.jar` 重命名为 `<file>.jar.disabled`。
      - `enable`：将 `<file>.jar.disabled` 重命名为 `<file>.jar`。
    - 目标匹配复用现有模糊匹配逻辑（大小写不敏感匹配 Mod ID、名称、文件名或去除后缀的文件名）。
    - 支持 `--dry-run` 预览操作，不实际修改文件名。
    - 支持 `--json` 输出结构化变更结果（包含操作类型、原路径、目标路径、是否执行）。
  - **验收标准**：
    - `mod-downloader-cli disable sodium` 将 `sodium-fabric-*.jar` 重命名为 `*.jar.disabled`，再次执行 `list` 显示 `ENABLED=false`。
    - `mod-downloader-cli enable sodium` 恢复为 `.jar`，再次执行 `list` 显示 `ENABLED=true`。
    - 重复操作（对已启用的模组执行 enable 或已禁用的模组执行 disable）友好跳过并提示。

---

### 3. 模组更新与批量升级 (`check-update` / `upgrade`)
- [x] **实现 `check-update` (别名 `outdated`) 命令**
  - **命令语法**：`mod-downloader-cli check-update [--platform modrinth|curseforge]`
  - **设计规范**：
    - 扫描本地已安装模组，获取其当前版本号与哈希。
    - 向 Modrinth / CurseForge 查询适配当前 Minecraft 版本和 Loader 的最新可用稳定版本。
    - 终端输出格式化对比表格：`ID`、`NAME`、`CURRENT_VERSION`、`LATEST_VERSION`、`PLATFORM`。
    - 支持 `--json` 输出机器可读格式。
  - **验收标准**：
    - 本地模组存在较新兼容版本时，`check-update` 正确高亮或列出待更新项。
    - 本地模组已是最新时，提示所有模组均为最新版本。

- [x] **实现 `upgrade` 命令**
  - **命令语法**：`mod-downloader-cli upgrade [--all] [--dry-run] [--timeout <dur>] [<mod...>]`
  - **设计规范**：
    - 默认模式：必须传入至少一个待升级模组名；若带 `--all` 则批量升级所有检测到有新版本的模组。
    - 升级策略：
      1. 下载新版本 JAR。
      2. 下载并验证成功后，清理移除旧版本 JAR 文件，避免新旧版本共存引发游戏崩溃。
      3. 若旧文件处于 `.disabled` 状态，升级后维持其禁用状态（命名为新文件名的 `.jar.disabled`）。
    - 支持 `--dry-run` 预览将要执行的下载与旧文件删除。
    - 失败回滚：若新版本下载失败，保留原模组文件不变更。
  - **验收标准**：
    - `mod-downloader-cli upgrade sodium` 成功下载最新版并将旧版 jar 移出目录。
    - `mod-downloader-cli upgrade --all` 完成全量自动升级与清理。
    - `upgrade --dry-run` 仅输出预期变更，不修改任何文件。

---

### 4. 依赖校验与本地体检 (`verify` / `info`)
- [x] **实现 `verify` (别名 `check`) 命令**
  - **命令语法**：`mod-downloader-cli verify [--strict]`
  - **设计规范**：
    - 读取本地所有模组 JAR（解析 `fabric.mod.json`、`quilt.mod.json`、`META-INF/mods.toml`、`mcmod.info` 等元数据）。
    - 执行三项健康检查：
      1. **缺失依赖检测**：扫描声明的 `depends` / `dependencies`，比对本地已安装清单；发现缺失时输出警告，并推荐可执行的安装命令（如 `mod-downloader-cli install modrinth:<dep>`）。
      2. **版本冲突与重复检测**：同一 Mod ID 存在多个不同 JAR 文件时标记告警。
      3. **加载器/游戏版本兼容性**：检查本地模组元数据声明的 Minecraft 版本或环境与当前实例是否匹配。
    - 退出码设计：所有检查通过退出码为 0；存在依赖缺失或冲突时退出码为 1（方便 CI / 自动化脚本调用）。
    - 支持 `--json` 输出结构化诊断报告。
  - **验收标准**：
    - 故意移除 `fabric-api` 时运行 `verify`，明确提示缺少 `fabric-api` 依赖并给出安装建议。
    - 在目录中放置同名模组的不同版本文件时，`verify` 告警提示版本冲突。

- [x] **实现 `info` (别名 `inspect`) 命令**
  - **命令语法**：`mod-downloader-cli info <mod-or-jar-file>`
  - **设计规范**：
    - 既支持按模组名检索本地已安装模组，也支持直接传入本地 `.jar` 文件路径进行离线分析。
    - 显示详细元数据：
      - 基础信息：Mod ID、名称、版本号、作者、描述、许可证。
      - 兼容环境：支持的 Minecraft 版本范围、Mod Loader。
      - 依赖关系：必需依赖 (Depends)、推荐依赖 (Recommends)、冲突列表 (Breaks/Conflicts)。
      - 文件信息：文件路径、文件大小、SHA1、SHA512 哈希。
      - 云端关联：若能匹配到 Modrinth/CurseForge，附带线上项目链接与 Slug。
    - 支持 `--json` 输出。
  - **验收标准**：
    - `mod-downloader-cli info sodium` 准确输出当前已安装 sodium 的内部元数据与依赖关系。
    - `mod-downloader-cli info ./some-mod.jar` 能正确解析任意独立 jar 文件的元数据。

---

### 5. 模组清单导入与导出 (`export` / `import`)
- [x] **实现 `export` 命令**
  - **命令语法**：`mod-downloader-cli export [--output <file>]`
  - **设计规范**：
    - 采用轻量文本清单格式（类似 Python `requirements.txt`）：
      - 每行格式：`<platform>:<slug-or-id>[==<version>]`（例如 `modrinth:sodium==0.5.8` 或 `curseforge:iris==1.7.0`）。
      - 未匹配到云端平台的模组以注释方式列出本地文件名，例如 `# local: custom-addon.jar`。
      - 文件首部附带注释元数据（导出时间、Minecraft 版本、Loader）。
    - 默认输出到标准输出 (stdout)，支持 `-o` / `--output <file>` 保存到文件（例如 `mods.txt`）。
  - **验收标准**：
    - `mod-downloader-cli export -o mods.txt` 成功生成符合规范的清单文件。
    - 输出内容排版清晰，可直接被文本编辑器阅读与修改。

- [x] **实现 `import` 命令**
  - **命令语法**：`mod-downloader-cli import [--skip-existing] <file>`
  - **设计规范**：
    - 读取指定的清单文本文件，逐行解析：
      - 自动忽略空行及 `#` 开头的注释行。
      - 解析平台前缀、模组标识以及指定版本号。
    - 安装处理：
      - 若声明了确切版本号（如 `==0.5.8`），按指定版本安装。
      - 若未指定版本号，安装适配当前游戏版本的最新版本。
      - 遇到已存在的文件默认跳过或覆盖（提供 `--skip-existing` 选项）。
    - 安装进度展示与汇总结果（成功数、跳过数、失败数）。
  - **验收标准**：
    - `mod-downloader-cli import mods.txt` 在全新实例中一键还原下载所有列出模组。
    - 清单中存在错误条目时不中断后续模组下载，最后统一汇总报错。

---

### 6. 用户配置持久化管理 (`config`)
- [x] **实现全局配置持久化管理与 `config` 命令族**
  - **存储路径规则**：
    - **严禁保存在当前工作目录 (CWD)**。
    - 严格遵循操作系统标准用户配置目录（通过 Go 标准库 `os.UserConfigDir()`）：
      - Linux: `~/.config/mod-downloader-cli/config.json`
      - macOS: `~/Library/Application Support/mod-downloader-cli/config.json`
      - Windows: `%APPDATA%\mod-downloader-cli\config.json`
  - **命令语法**：
    - `mod-downloader-cli config get <key>`
    - `mod-downloader-cli config set <key> <value>`
    - `mod-downloader-cli config unset <key>`
    - `mod-downloader-cli config list`
    - `mod-downloader-cli config path`
  - **支持的配置键 (Keys)**：
    - `mc-version`：默认 Minecraft 版本。
    - `loader`：默认模组加载器（`fabric` / `forge` / `neoforge`）。
    - `curseforge-api-key`：CurseForge API 密钥。
    - `modrinth-api-key`：Modrinth API 密钥。
    - `cache-dir`：自定义缓存目录。
    - `minecraft-dir`：默认 Minecraft 游戏根目录。
  - **优先级层级**：
    - 命令行 Flags（最高） > 环境变量 > 用户配置文件 (`config.json`) > 运行时自动探测（最低）。
  - **验收标准**：
    - `config path` 输出符合操作系统规范的标准路径。
    - `config set loader fabric` 后，在任意目录执行命令均默认采用 `fabric`，无需每次指定 `--loader`。
    - 敏感信息（API Key）在 `config list` 中自动脱敏（如 `****abcd`），带 `--show-secrets` 标志时方可明文显示。
    - 配置文件读写具备容错与目录自动创建能力。

---

### 7. 文档与测试套件完善
- [x] **单元测试与集成测试补充**
  - 为所有新命令 (`enable`, `disable`, `check-update`, `upgrade`, `verify`, `info`, `export`, `import`, `config`) 编写完备的 `_test.go`。
  - 覆盖参数互斥、非法输入、离线/无网络情况下的错误处理。
  - 确保 `go test ./...` 保持 100% 通过。
- [x] **更新 `README.md`**
  - 补充新命令的使用说明、示例、参数表格与典型工作流。
