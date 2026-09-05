# npm Package Control Plane

## Control Plane Role

本控制平面管理 `locus-scope` 从 OCI Package 分发整体切换到 npm 生态的设计、项目结构调整、实现迁移和验收。目标是完成干净切换，而不是在现有 OCI 实现旁增加第二套长期并存的分发路径。

当前代码和权威设计仍以 OCI、ORAS、Zot、OCI digest/cache/materialization 为迁移起点；目标状态已由 `参考.md` 和后续用户决定冻结，尚未确定的实现接口保留在动态讨论区。

## Meta Reference

- 执行相关任务前依次读取 [`cp-meta.md`](cp-meta.md) 和本文件。
- 随后读取 [`cpm-npm-packages.md`](cpm-npm-packages.md) 中已确认的跨任务决策；若其与当前用户决定或代码事实冲突，以后两者为准。
- 设计事实依次核对 `documents/design/Scope设计.md`、`documents/design/Package设计.md`、`documents/design/测试设计.md` 与当前代码；代码与旧文档只描述迁移起点，不覆盖用户确认的新目标。
- 当前迁移指导输入为 [`参考.md`](参考.md)；其中已明确的目标约束进入后续设计，仍有歧义的部分保留在动态讨论区。
- 创建或扩展本平面时使用 `create-control-plane`；任务或阶段收尾时使用 `compact-control-plane`。

## Scope

- 控制对象：Scope Package 的发布、引用、解析、获取、安装、锁定、缓存/物化、离线加载及其 npm 分发载体。
- 当前入口与直接调用方：`cmd/locus-pkg`、`internal/packages`，以及调用 `packages.LoadWorkspace` 的 `cmd/locus-scope`。
- 配套范围：npm 所需项目结构与 manifests、`scripts/`、Windows 安装/发布中 OCI 或 Zot 的耦合、`test/e2e/` fixtures、权威设计、README 与 changelog。
- 已冻结 package 单元、双模式职责、package identity 和首版 dependency 边界；Node 子进程协议、lock/store 细节与具体 Go npm library 尚待设计。
- Scope/Core protocol 的 Entity、Relation、Projection、Group 与 ownership 语义默认不变；若新包模型要求改变协议，必须单独提出并获得明确决定。

## Core Rules

### 目标与切换

- 最终状态不得依赖 OCI artifact、OCI Distribution、ORAS、Zot、Docker credential 或 OCI identity。设计确认后迁移全部调用方，并删除被替代的代码、依赖、脚本、安装资产、测试与文档；不保留兼容 shim 或双栈路径，除非用户明确要求过渡期。
- 一个可分发 Locus package 是一个标准 npm package、一个分发单元和一个 Scope；`package.json` 是 package metadata 唯一真相，`locus.entry` 指向该 Scope，不支持跨 package 寻址子 Scope，细粒度复用通过拆分 package 实现。
- 不把旧 OCI digest、tag、cache 或 materialization 术语机械改名为 npm；每个概念按 npm 的真实可观察行为重新论证。
- Scope/Graph semantic identity 固定为 `npm:<name>@<version>`，不包含 registry、resolved URL 或 integrity；后面三者只参与解析、校验和可重复安装。
- `@sundw/locus-scope` 通过薄 Node adapter 和子进程复用 Go 核心，不在 JavaScript 中复制 Scope/Graph 语义。
- Locus 不分发或管理 Registry server；Verdaccio 只作为项目级开发与 E2E 基础设施。
- `locus-pkg pack` 优先直接复用选定 Go npm library 的 npm-compatible pack；库不提供时再单独决定 packlist 实现或首版范围，不定义未经评估的 Locus packlist。

### 设计门

- 实现前必须明确：包内容边界、包名与版本、manifest import 表达、版本选择、不可变 identity/integrity、lock 契约、安装与离线模型、CLI 契约、registry/auth、缓存边界、monorepo/package 布局和 Windows 分发影响。
- 已冻结方向下的工程细节由 agent 按项目品味自主完成，用户只验收最终可观察表现；只有选择会改变已确认的用户工作流、公共契约、兼容范围、安全边界或交付平台时才暂停询问。
- 复用 npm/pnpm 的成熟能力，避免自建 registry、tarball、SemVer resolver 或缓存协议。只有项目独有的 Scope 装配、ownership 和离线行为留在 Go 代码中。
- `pnpm` 是仓库唯一 Node 包管理器。不得将 pnpm/npm 的 store 或 cache 重定向到仓库；本地测试 registry 的配置、存储、日志、打包结果和安装现场必须位于 `temp/`。
- 可复用 E2E 声明和模拟状态放在 `test/e2e/case/`，运行时具现化到 `temp/e2e-run/` 并保留现场。

### 质量与验收

- 先更新已确定的权威设计，再实现对应契约；同一规则只在一份设计中完整定义，其他入口链接引用。
- 测试选择能完整观察契约的最低层级；registry、打包、发布和安装边界不得用 mock 成功结果代替真实状态转换。
- E2E 必须使用环回本地 npm registry 完成真实 publish → resolve/install → load 流程，并验证实际发布 tarball 的内容、版本/integrity、重复发布或冲突行为、lock/离线复现和失败事务边界。
- 所有读取、写入、子进程工作目录、registry 状态和测试状态留在仓库内；不得读取用户 npm 配置、凭据或外部网络状态。机器级包管理器 cache 是唯一允许的工作区外缓存。
- 每个阶段只处理本次迁移新增或加重的高置信坏味道；不顺手重构 Core semantics。

### 控制平面维护

- 用户的详细指导进入后，先把决定和未决项写入动态任务区，再冻结本轮入口、调用方和最小公共 API。
- 会跨任务复用且已验证的 npm 迁移结论沉淀到 `cpm-npm-packages.md`；一次性进度、候选方案和会随代码变化的现状不进入 memory。

## Task Board

当前无待执行任务。
