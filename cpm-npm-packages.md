# npm Package Memory

本文件保存 `cp-npm-packages.md` 已确认、可跨任务复用的迁移决策。当前任务队列和未决问题仍以控制平面为准。

## Package 模型

- 一个可分发 Locus package 是一个标准 npm-compatible package、一个分发单元和一个 Scope。
- `package.json` 是 package name、version、dependencies、SemVer、registry/distribution 等 package metadata 的唯一真相；`locus.entry` 指向该 package 的唯一 Scope。
- Package Scope import 按 npm dependency 和 importer package context 解析，只指向 dependency package 的唯一 Scope；不提供 package 内子 Scope 寻址。需要更细粒度时拆分 npm package。
- `package.json.dependencies` 定义 package universe；`locus.yaml imports` 选择实际进入 Scope graph 的 package。两者不合并，也不在 `locus.yaml` 重复版本范围。
- Scope/Graph semantic identity 固定为 `npm:<name>@<version>`，不包含 registry、resolved URL 或 integrity。后面三者只服务于解析、校验与可重复安装。
- 可分发 package 中 `locus.entry` 是唯一 Scope manifest；发现第二个 Scope manifest 时 package validation 失败。
- `locus.yaml imports` 使用 bare npm package name 表示 package Scope，例如 `base: "@example/base"`；不携带 version 或 subpath。`./`、`../` 继续表示本地 Scope。
- Package import 必须对应 importer package 的 `dependencies` edge；解析始终以 importer package 为上下文。
- Locus package 可以同时包含 JavaScript API；`locus` metadata 不占用或改写 npm 的 `main`、`exports` 等 JavaScript 入口。
- 普通 npm dependencies 可以存在并参与完整 dependency graph；只有包含合法 `locus.entry` 的 package 可以被 `locus.yaml` import。

## 双模式

- Pure Locus 使用 standalone `locus-pkg` 和 `locus-scope`，不依赖 Node/npm/pnpm executable；`.locus/packages`、`.locus/cache` 和 `locus.lock` 只属于该模式。
- npm ecosystem 模式由 npm/pnpm 管理 package environment、dependency resolution、cache 和原生 lockfile；`@locus/scope` 与 `locus-scope-node` 不创建、修改或依赖 `locus.lock`。
- `@locus/scope` 是薄 Node adapter，首选通过子进程复用 Go 核心。JavaScript 不复制 Scope、Entity、Relation、Import、ownership 或 Workspace 语义。
- 两种模式共享同一种 Locus package 和 Scope/Graph 语义；区别只在 package installation/resolution 来源。
- `@locus/scope` 首版只承诺 `locus-scope-node` CLI 与稳定 JSON 输出，不承诺返回完整 Workspace 的 Node 程序 API。
- npm package 通过平台专用 package 携带 Go engine；首版平台矩阵为 Windows x64、Linux x64/arm64、macOS x64/arm64。
- Node adapter 首版一次性构造 importer-relative package environment descriptor，通过有版本的单次 JSON stdin/stdout 协议调用 Go 子进程；不引入双向 RPC。

## Pure Locus 边界

- 首版 package resolution 支持 `dependencies` 中的 registry package、scoped package、精确版本、SemVer range 和 transitive dependencies。
- 首版不承担 node_modules hoisting、peer/optional dependency installer 语义、lifecycle scripts、native addon rebuild、npm scripts 或 workspace installer 语义。
- 首版不支持 `file:` 或 npm workspace dependency；本地开发依赖在 Registry/SemVer 主链完成后另行设计。
- Pure Locus 遇到首版不支持的 peer、optional、alias、`file:`、`workspace:`、git 或 URL dependency 时明确失败，不静默忽略。
- `.locus/packages` 必须允许同名 package 的多个 resolved version 共存。解析以 importer package 为上下文，根据 `locus.lock` 的 dependency edges 定位具体 package node；禁止全局 package-name → single-version 映射。
- 普通本地 Scope 只需要 `locus.yaml`。只有参与 `locus-pkg install/pack/publish` 或作为 npm dependency 分发的 Scope 才必须是合法 npm package，并从 `package.json` 获取 name 和 version。

## Pack 与 Registry

- `locus-pkg pack` 优先直接复用选定 Go npm library 的 npm-compatible pack 能力；Locus 不预先定义独立 packlist 规则。
- 若选定库没有合适的 pack 能力，再决定引入专用 packlist 实现或缩减首版 pack/publish 范围。
- Locus 不再分发或管理 Registry server。Standalone installer 只提供 Locus CLI。
- Verdaccio 是项目级开发和 E2E 使用的独立 npm-compatible infrastructure；项目需要提供便捷的项目级安装、启动、停止等生命周期脚本。
- Verdaccio 配置、storage、日志和测试现场位于仓库 `temp/`；不得修改用户全局 npm/pnpm registry 或配置。npm/pnpm store/cache 沿用机器级全局配置，不重定向到仓库。

## 开发 Registry

- 开发 Verdaccio 默认监听 `127.0.0.1:4873`，使用 `temp/verdaccio-dev/`；E2E 使用独立的 `temp/e2e-run/npm/registry/` 和动态环回端口。
- 开发 Registry 允许匿名读取，publish 需要认证；E2E 使用确定性测试账户，凭据只写入 `temp/e2e-run/`。
- `scripts/npm-registry.ps1` 提供项目级 `install`、`start`、`stop`、`status`、`logs` 和 `reset`；`start` 后台运行并等待健康检查，`reset` 要求显式 `-Force`。
- 停止操作必须验证目标是脚本启动的 Verdaccio，不得仅按端口杀死进程。

## Release

- 根 `VERSION` 是整个发行批次的版本真相；release 流程写入或校验各 npm package 的 `package.json.version`。
- `@locus/scope` 与所有平台 package 使用相同版本，平台 `optionalDependencies` 使用精确版本。

## 决策权限

- 用户关注最终可观察行为，不再主导已冻结方向下的工程细节设计。
- Agent 按项目既有品味自主决定 schema、目录细节、内部接口、事务实现、错误模型、库组合和测试落点：轻量、直接、复用成熟能力、单一约定、干净切换、可验证。
- 只有当选择会改变已确认的用户工作流、公共契约、兼容范围、安全边界或交付平台时，才暂停并请求用户决定。
