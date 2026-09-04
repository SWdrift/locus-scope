# Changelog

本文件记录 `locus-scope` 各发布版本的用户可见变更。

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
