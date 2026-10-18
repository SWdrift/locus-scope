# E2E 测试设计

## 简述

E2E 测试只验证跨越真实 CLI、构建产物、npm/pnpm 安装或文件系统的用户可观察行为。领域内部规则由 Unit/Application 测试负责；E2E 不复制内部断言。

## 职责

本文定义 E2E 用例覆盖的领域问题、fixture 边界和跨层验收。公共命令、Core API、Scope 和 Package 规则分别以[CLI](protocol/CLI.md)、[核心 API](protocol/CORE_API.md)、[Scope 设计](Scope设计.md)和[Package设计](Package设计.md)为准。

## E2E 用例组织

### `scope-fixtures.test.mjs`

覆盖构建后 CLI 的 Scope 组合、Mutation、查询和 package 错误。

| 场景 | 验证的问题 |
| --- | --- |
| `scope-composition/app` | root Scope 与 dependency Scope 组合后，CLI `validate` 和 `graph` 是否得到正确 Workspace。 |
| `mutation-discovery` | Mutation add 是否遵守 `.locusignore`、`.locus` 隐藏目录和 Definition document selection policy；被排除的目标不得创建文件。 |
| `query-edge` | 合法 Entity ID 含 `=` 等 filter 操作符时，CLI 是否优先按精确 Entity ref 查询。 |
| 动态 `mutation-bom` | 带 UTF-8 BOM 的 JSON definition 能否被 CLI mutation 读取、修改并持久化。 |
| 动态 `mutation-lifecycle` | Entity 与 Relation 的 add、set、unset、remove 是否保持持久化内容、reload Workspace 和 CLI result 一致。 |
| 动态 `graph-commands` | `graph`、`adjacent`、`path` 和 `impact` 是否遵守默认深度、有限深度、方向和可达性语义。 |
| 动态 `cli-errors` | CLI JSON 错误字段、退出状态、敏感路径过滤和不透明字段 mutation 边界是否符合公共契约。 |
| 动态 `blocked-package-export` | Locus package 缺少 `./package.json` export 时，CLI 是否返回 `scope.package_invalid`，而不是伪装成依赖缺失。 |

### `local-package.test.mjs`

验证最终 release tarball 可被本地 consumer 安装和使用。

- Core 与 CLI package 可以安装并加载；
- `locus-scope validate` 和默认 `graph` 可以执行；
- library consumer 可以通过公开 API 加载 Workspace；
- tarball 包含正确入口和 README。

### `npm-lifecycle.test.mjs`

验证 npm/pnpm 发布、安装和 resolved package graph。

- npm Registry publish/install 生命周期；
- npm 与 pnpm consumer；
- transitive Locus Package Scope；
- importer-relative exact identity；
- 同名 package 多版本；
- private package 和 authenticated publish；
- 最终 tarball 的 CLI、library、Entity、Relation、Path、Validate 和 package graph。

## 跨层验收基线

E2E 用例必须优先验证跨越构建产物、真实 CLI、文件系统和 npm/pnpm 安装边界的用户可观察不变量，不重复断言 Domain/Application 内部实现。

### Mutation 生命周期

通过构建后的 CLI 验证 Entity 与 Relation 的完整写入生命周期：

- `add`、`set`、`unset`、`remove` 的成功结果、持久化内容和 reload 后 Workspace 一致；
- `--file`、provenance、root Scope 可写边界和 dependency Scope 只读边界一致；
- Definition document selection policy 对 Loader 与 Mutation 使用同一规则；
- `.locusignore`、`.locus` 和嵌套 Scope 目标在写入前拒绝，且不创建文件；
- reload 或验证失败时执行 rollback，原文件恢复或新文件删除；
- rollback cleanup 或二次 reload 失败时，CLI 输出稳定错误契约。

### 图查询命令

使用包含多级路径、平行边、自环和不同 Relation type 的 Scope fixture，通过构建后的 CLI 验证：

- `graph` 省略 depth 时返回全部正向可达节点；
- `graph --depth 0`、有限 depth 和 `--via` filter 遵守深度与边筛选语义；
- `adjacent` 的 incoming、outgoing、both 方向及自环去重正确；
- `path` 返回确定的最短有向路径；
- `impact` 沿 incoming edges 返回完整反向影响集合；
- CLI JSON envelope、稳定排序和退出状态符合公共协议。

### CLI 错误与安全输出

通过真实 CLI 输入非法参数、缺失对象、非法 manifest、缺失依赖和不可读 source，验证：

- `--json` 错误至少包含 `error`、`code` 和 `reason`；
- 参数错误退出 `2`，运行时和数据错误退出 `1`；
- `details` 只包含安全的相对路径、引用、字段和行号；
- 输出不包含绝对工作区路径、用户目录、凭据、cause 或 stack trace；
- Entity、Relation 的 JSON mutation 不能改变 JavaScript prototype 或跨请求共享状态。

### Package 安装与布局一致性

在隔离 consumer 中分别使用 npm 与 pnpm 安装同一组 package，验证：

- hoisted 与 symlink 安装布局产生相同的 resolved Environment；
- importer-relative 多版本依赖保持正确 exact identity 和 ownership；
- 缺失 direct dependency、blocked export、entry escape 和冲突 identity 在执行 Scope 命令前失败；
- tarball consumer 的 CLI 与 library API 产生一致的 Validate、Query、Graph、Path 和 Relation 结果。

### Discovery 与 Scope 边界

使用包含普通子目录、`.locusignore`、`.locus`、`.git`、嵌套 Scope 和符号链接边界的 fixture，验证：

- Definition document 仅按统一 selection policy 进入 Workspace；
- 普通目录不产生额外 Scope、Group 或 ownership 边界；
- 嵌套 Scope 不被父 Scope 递归加载；
- 只有显式 Import 才会加载嵌套 Scope；
- Loader、Mutation 和 CLI 对相同路径边界给出一致结果。

## Fixture 边界

可复用 fixture 位于 `test/e2e/case/`；动态 consumer、BOM 文件、临时 package 和结果位于 `temp/e2e-run/`。

| Fixture 组                                                        | 用途                                                     |
| ----------------------------------------------------------------- | -------------------------------------------------------- |
| `scope-composition/`                                              | Scope 组合和依赖 Scope                                   |
| `mutation-discovery/`                                             | Mutation 与 Definition document selection policy         |
| `mutation-lifecycle/`                                             | Entity/Relation 完整 mutation 生命周期和 codec           |
| `query-edge/`                                                     | 操作符形式 Entity ID                                     |
| `graph-commands/`                                                 | Graph、Adjacent、Path 和 Impact 命令                     |
| 动态 `mutation-bom`、`cli-errors`、`blocked-package-export`        | BOM codec、CLI 错误契约和 package export                 |
| `npm/`                                                             | Registry、npm/pnpm、多版本和 transitive package          |
| `discovery/`、`validation/`、`cycle/`、`reexport/` 等              | 供其他测试复用；只有被 E2E 测试显式执行时才计入覆盖       |

E2E 不修改 `test/e2e/case/` 中的声明 fixture；测试状态保留在 `temp/e2e-run/`。npm/pnpm store/cache 继续使用机器级配置。

## E2E 验收矩阵

| 验收主题 | E2E 场景 | 观察断言 | 覆盖情况 |
| --- | --- | --- | --- |
| Mutation 生命周期 | Entity/Relation `add`、`set`、`unset`、`remove` | CLI 结果、持久化内容和 reload 后 Workspace 一致 | 已覆盖 |
| Relation structural fields | Relation `from`、`type`、`to` 与 field assignment 冲突 | 返回 `scope.field_invalid` 且不创建或修改文件 | 已覆盖 |
| Mutation selection policy | `.locusignore`、`.locus`、嵌套 Scope 目标 | 写入前拒绝且不创建文件 | 已覆盖 |
| Mutation rollback | reload 或验证失败的 Mutation | 原文件恢复或新文件删除；后续 `validate` 成功 | 未覆盖 E2E；Unit 已覆盖 |
| Mutation codec | 带 BOM 的 JSON、YAML、YML | 读取、修改、持久化和再次加载一致 | 已覆盖 |
| Graph 默认深度 | 三节点正向链路，省略 `--depth` | 返回全部正向可达节点且不伪造 depth | 已覆盖 |
| Graph 有限深度 | `depth 0`、`depth 1` 和更深链路 | 节点、Relation 和 seed 边界符合深度限制 | 已覆盖 |
| Relation 筛选 | `graph --via` | 只沿匹配 Relation 遍历 | 已覆盖 |
| Adjacent 方向 | `incoming`、`outgoing`、`both` | 返回方向正确的节点和 Relation | 已覆盖 |
| Adjacent 边界 | 自环、平行边 | 自环不重复；不同 Relation type 不丢失 | 未覆盖 E2E；Unit 已覆盖 |
| Path | 多段有向链路和等长候选路径 | 返回确定的最短路径和稳定顺序 | 部分覆盖；已覆盖多段路径，未覆盖等长决胜 |
| Impact | 多层 incoming 链路 | 返回完整反向影响集合和 Relation | 已覆盖 |
| CLI 错误协议 | 缺失对象、非法参数、非法 manifest、缺失依赖 | `error`、`code`、`reason`、退出码符合契约 | 已覆盖 |
| CLI 安全输出 | source、load、package 错误 | 不泄露绝对路径、用户目录、凭据、cause 或 stack | 部分覆盖；已覆盖缺失对象路径和 package 错误 |
| 不透明字段安全 | `__proto__`、`constructor.prototype` JSON mutation | 字段按普通数据持久化，不改变共享原型 | 已覆盖 |
| Package layout | npm hoisted 与 pnpm symlink consumer | resolved Environment、Workspace 和结果 JSON 一致 | 部分覆盖；已覆盖 npm/pnpm 生命周期，未逐项对比同一结果 |
| Package diagnostics | 缺失 direct dependency、blocked export、entry escape、冲突 identity | 在 Scope 命令执行前返回结构化错误 | 部分覆盖；已覆盖 blocked export |
| Discovery 边界 | 普通目录、`.locusignore`、`.git`、`.locus`、嵌套 Scope | 只加载 selection policy 允许的 Definition document | 部分覆盖；已覆盖 `.locusignore`、`.locus` 和 Mutation 边界 |
| Scope Import 边界 | 嵌套 Scope 无 Import 与显式 Import | 无 Import 不加载；显式 Import 才进入 reachable graph | 未覆盖 E2E；Unit 已覆盖 |
