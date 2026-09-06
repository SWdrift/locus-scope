# Changelog

本文件记录 `locus-scope` 各发布版本的用户可见变更。

## [2.0.0] - 2026-09-06

### Added

- `scope`、`group`、`entity`、`relation` 统一 list/show/find，支持嵌套 JSON field、正则和 provenance 查询。
- directed multigraph 的 `graph`、`path`、`impact`，支持 depth、Relation `--via` 与 stdin Entity seeds。
- Entity/Relation add/set/unset/remove、root `scope.locus.yaml`、`--file`、dependency read-only 和失败回滚。
- `scope:`、`group:`、`path:` 归一化语义子图 diff，以及 Node host protocol v2 stdin 转发。

### Changed

- Entity 与 Relation 的公共 JSON 改为 root 开放对象 envelope；Relation 以 `(from,type,to)` 唯一定位并支持开放属性。
- `locus-scope` 与 `locus-scope-node` 共享 command runner、JSON、错误和退出语义；旧 `list/show/resolve` 命令由统一查询入口替代。
- application error 使用稳定 Code、Reason、Details 和 cause，并集中映射 CLI exit code。

### Fixed

- 全部正向 E2E Relation fixture 使用 canonical `{from,type,to,...}` 对象；旧 tuple 输入不再兼容并由独立失败 fixture 验证拒绝。
- Relation Core 字段统一为 `Type`，并验证开放嵌套属性、canonical 落盘及 standalone/Node 管理结果一致性。
- Group Relation 协议示例和 canonical object 缺字段诊断 fixture 与核心定义一致。

## [1.0.2] - 2026-09-05

### Fixed

- Darwin 与 Linux npm platform package 将 Go host 声明为 `bin` target，保证最终 tarball 中的 Unix mode 为 `0755`；release 从最终 `.tgz` 安装并执行真实 `validate`。
- `locus-scope-node --json` 在 Workspace 加载前失败时仍输出 JSON，并为 npm consumer 的缺失直接依赖提示 `pnpm add` 或 `npm install`。
- Markdown 链接统一使用 `/`，恢复跨平台文档检查。

## [1.0.1] - 2026-09-05

### Changed

- 六个 `@sundw/locus-scope*` npm Package 补充 GitHub repository、homepage 和 issues 元数据，项目首页增加 npm 主包入口。

## [1.0.0] - 2026-09-05

### Added

- npm-compatible Package 分发：Pure `locus-pkg` 的 install/uninstall/update/list/pack/publish，以及 npm/pnpm 的 `@sundw/locus-scope` 与五个平台 host package。
- importer-relative 多版本解析、严格 `locus.lock`、项目内 integrity store、offline/frozen transaction 和 Verdaccio 开发/E2E 工具。
- project-local Inno Setup 6 管理脚本，固定下载校验、portable 安装位置和 release 默认编译器。

### Changed

- Package identity 固定为 `npm:<name>@<version>`；bare Scope Import 由声明它的 package dependency context 解析。
- Windows installer 只分发两个 standalone CLI；Registry server 改为独立的项目基础设施。
- 交付定义集中到 `packaging/`；仓库脚本统一采用 `<domain>-<action>.ps1` 命名。
- 仓库任务统一通过根 package scripts 执行；跨平台编排迁移到 Node，Windows 专用实现集中到 `scripts/windows/`，工具配置不再使用 `.tools/`。

## [0.1.1] - 2026-09-04

### Added

- `locus-scope version`、`locus-pkg version` 及对应的 `--version` option。
- 文本版本输出供人阅读，`--json version` 输出稳定的名称与版本字段。

### Changed

- 构建脚本从根目录 `VERSION` 向两个 CLI 注入发布版本。
- 用户安装脚本检测已有安装并提示先卸载，避免重复安装被正在运行的 Zot 或已有文件阻塞。

## [0.1.0] - 2026-09-04

### Added

- 支持从 Scope 根目录递归发现普通子目录中的 Definition documents。
- 支持 Scope 根目录 `.locusignore` 排除文件或整棵目录。
- 将后代 Scope manifest 作为递归边界，只有显式 Import 才会加载对应 Scope。

### Changed

- Definition document 仅识别 `*.locus.yaml`、`*.locus.yml` 和 `*.locus.json`，普通 YAML/JSON 不再作为 Locus 输入。
- `.git` 与 `.locus` 目录默认不参与 Definition document 发现。
- 项目示例统一为单个 root `locus.yaml`、单个 `locus.lock`，依赖由 OCI Package 提供。
