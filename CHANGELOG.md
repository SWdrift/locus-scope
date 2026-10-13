# Changelog

本文件记录 `locus-scope` 各发布版本的用户可见变更。

## [4.0.2] - 2026-10-13

### Fixed

- 非法 diff selector 返回参数错误（CLI 退出码 2），不再误报内部错误并隐藏输入诊断。
- graph library API 对负 depth 返回 `invalid_argument`，不再抛裸 `Error`。
- 统一 application/domain 错误 Reason 注册、Code 映射和 CLI JSON 错误输出；公开诊断不再泄露绝对路径。
- 将 Scope、package environment 和 mutation 的已知失败迁移到结构化错误分类，并保留底层 cause。

## [4.0.1] - 2026-10-13

### Fixed

- Scope mutation rollback 不再静默丢弃恢复原文件、删除新文件或二次 reload 失败；原始 reload 错误与 rollback 错误通过 `AggregateError` 保留并暴露。

## [4.0.0] - 2026-10-12

### Changed

- 将 Scope domain、application、CLI 和 npm package runtime 全部迁移到 TypeScript。
- 运行时基线提升为 Node.js 24 LTS，TypeScript 编译器使用 5.9。
- `@sundw/locus-scope` 同时提供 `locus-scope` CLI 和进程内 TypeScript API。
- 删除 Go host、平台 host package 和内部 JSON host protocol。

### Fixed

- `diff path:` 在不同 Scope 含有相同 ID 时保留带 Scope 限定的 Entity 和 Relation identity。
- Scope mutation 拒绝解析后越出可写 root Scope 的符号链接目标。
- 修复向无 Details 的已有错误合并 Details 时的 panic；运行时加载错误保持退出状态 `1`，CLI 输出隐藏文件系统路径。

## [3.1.0] - 2026-10-12

### Fixed

- `diff path:` 在不同 Scope 含有相同 ID 时保留带 Scope 限定的 Entity 和 Relation identity。
- Scope mutation 拒绝解析后越出可写 root Scope 的符号链接目标。
- 修复向无 Details 的已有错误合并 Details 时的 panic；运行时加载错误保持退出状态 `1`，CLI 输出隐藏文件系统路径。

## [3.0.1] - 2026-09-20

### Fixed

- `--json` 现在也覆盖未知命令等 Cobra 参数解析错误，统一输出 JSON 错误对象并返回退出状态 `2`。

## [3.0.0] - 2026-09-18

### Removed

- 删除 Pure Locus Package Manager：`locus-pkg`、`locus.lock`、`.locus/packages`、cache/store/materialization、offline 和 frozen-lockfile 流程。
- 删除 Go 中重复实现的 npm Registry、SemVer resolution、tarball、integrity、pack/publish、authentication 与 dependency installation。
- 删除 standalone Scope CLI 分发、Windows standalone installer 及其脚本、测试和文档。
- 删除 graph 的 Gonum adapter 与 Gonum 依赖。

### Changed

- 项目收缩为 npm/pnpm-hosted Scope 图引擎；npm/pnpm 负责 Package 管理，Node adapter 负责解析实际安装环境。
- Node adapter 将 exact resolved Environment 交给 Go；Go `packageenv` 只验证 Scope Resolver 所需不变量。
- npm Package 统一暴露 `locus-scope` executable；CLI 参数解析迁移到 Cobra，业务逻辑继续位于 `scopeapp`。
- `graph`、`path`、`impact` 改用 outgoing/incoming adjacency 与 BFS。
- npm release 保留主 Package 与五个平台 Go host Package，并使用 npm/pnpm 原生命令完成 pack/publish。

## [2.0.2] - 2026-09-06

### Fixed

- 修正 npm README 中 GitHub 文档链接使用错误分支而导致的 404。

## [2.0.1] - 2026-09-06

### Changed

- README 与入门、npm、独立 CLI 和完整 CLI 参考文档提供对应的中英文版本，并统一用户文档导航。
- npm release 支持跳过 Windows 独立制品和安装包，仅生成并验证六个 npm Package tarball。

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
