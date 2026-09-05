# Changelog

本文件记录 `locus-scope` 各发布版本的用户可见变更。

## Unreleased

## [1.0.0] - 2026-09-05


### Added

- npm-compatible Package 分发：Pure `locus-pkg` 的 install/uninstall/update/list/pack/publish，以及 npm/pnpm 的 `@locus/scope` 与五个平台 host package。
- importer-relative 多版本解析、严格 `locus.lock`、项目内 integrity store、offline/frozen transaction 和 Verdaccio 开发/E2E 工具。
- project-local Inno Setup 6 管理脚本，固定下载校验、portable 安装位置和 release 默认编译器。

### Changed

- Package identity 固定为 `npm:<name>@<version>`；bare Scope Import 由声明它的 package dependency context 解析。
- Windows installer 只分发两个 standalone CLI；Registry server 改为独立的项目基础设施。
- 交付定义集中到 `packaging/`；仓库脚本统一采用 `<domain>-<action>.ps1` 命名。

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
