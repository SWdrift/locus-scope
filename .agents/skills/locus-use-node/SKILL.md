---
name: locus-use-node
description: "指导用户在 Node.js 项目中通过 npm 或 pnpm 安装和使用 Locus Scope，创建、验证、查询 Scope，并消费 npm Registry 中的 Locus Package。用于 npm 模式、pnpm、node_modules、package-lock 或 pnpm-lock 工作流。"
metadata:
  domain: locus-scope
---

# Locus Scope · npm 模式使用

## 目标

让现有 Node.js 项目通过 npm 或 pnpm 管理 Locus Package 的版本、lockfile、缓存和 `node_modules`，并使用 `locus-scope-node` 创建、验证和查询 Workspace。非 Node.js 或 Pure Locus 环境改用 `$locus-use`。

## 安装

要求 Node.js 20.6+。pnpm 项目：

```text
pnpm add @sundw/locus-scope
```

npm 项目：

```text
npm install @sundw/locus-scope
```

入口是 `locus-scope-node`。JavaScript adapter 只从 importer 的 package manager 环境解析 Package，并把 descriptor 交给平台 Go host；它不解析 Locus definitions。

创建 `locus.yaml`：

```yaml
id: app
```

执行真实加载：

```text
pnpm exec locus-scope-node validate
# npm 项目：npx locus-scope-node validate
```

## 创建和查询 Scope

添加 `entities.locus.yaml`：

```yaml
entities:
    - id: database
      type: postgres
      host: db.internal
      port: 5432

    - id: backend
      type: service

relations:
    - from: backend
      type: uses
      to: database
```

pnpm 项目统一执行：

```text
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node entity
pnpm exec locus-scope-node entity database
pnpm exec locus-scope-node relation
pnpm exec locus-scope-node graph backend --depth 1
```

npm 项目把命令前缀替换为 `npx locus-scope-node`。在 Scope 目录之外执行时使用 `--scope <dir>`；否则 CLI 沿祖先链寻找最近的 Scope manifest。Agent、脚本或 CI 追加 `--json` 获取稳定输出。

Relation definition 使用 `from`、`type`、`to` 三元组；`relation --json` 输出解析后的对象，其中端点包含 owner 的 `scope_id`、Source identity `scope` 和 Entity `id`。

## 本地 Scope 组合

root Scope 可通过相对路径导入另一个本地 Scope：

```yaml
id: app
imports:
    infra: ../infra
```

Import 创建带 alias 的 Projection。访问 `infra:database` 时，目标 Scope 必须显式 export `database`。本地 Source identity 使用规范 `file://` URL。

## npm Package 消费

安装 adapter 和 Locus Package：

```text
pnpm add @sundw/locus-scope @example/infra
```

npm 对应为：

```text
npm install @sundw/locus-scope @example/infra
```

root `locus.yaml` 只写 bare npm Package 名，不携带版本或 subpath：

```yaml
id: app
imports:
    infra: "@example/infra"
```

版本范围属于同目录的 `package.json.dependencies`，确定版本由 `pnpm-lock.yaml` 或 `package-lock.json` 固定：

```json
{
  "name": "example-app",
  "private": true,
  "dependencies": {
    "@example/infra": "^1.0.0",
    "@sundw/locus-scope": "^1.0.0"
  }
}
```

验证和查询：

```text
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node entity infra:database
```

解析后的 Package Source identity 是 `npm:@example/infra@1.2.3`。Import alias 只是 root Scope 中的 Projection 名，不决定 identity。

adapter 从 root importer 开始，并在每个已发现 Locus Package 自己的依赖上下文中解析直接依赖。npm hoisting、pnpm symlink 和同一版本的重复物理副本不改变 identity 或 edge 语义；相同 `name@version` 的多份副本只有在 entry 与 Locus edges 一致时才合并。

## 依赖管理与 CI

始终使用当前项目的 package manager：

```text
pnpm add @example/infra
pnpm update @example/infra
pnpm remove @example/infra
```

或对应的 `npm install`、`npm update`、`npm uninstall`。提交 `package.json` 和 package manager lockfile。

CI 使用项目既有的 frozen 安装方式，然后验证 Workspace：

```text
pnpm install --frozen-lockfile
pnpm exec locus-scope-node --json validate
```

npm 项目使用：

```text
npm ci
npx locus-scope-node --json validate
```

安装可访问 Registry；安装完成后的 Workspace 加载和查询不隐式联网。离线复现能力由 npm/pnpm lockfile 与机器级 package-manager cache 决定。

## 发布与 Registry

消费 Package 不需要 `locus-pkg`。Agent 在 npm 模式发布 Locus Package 时使用 `$locus-publish-node`；它通过轻量校验、`npm pack` 和裸 `npm publish` 发布检查过的确切 tarball。

Registry、scope mapping 和认证遵循 npm/pnpm 自身配置。token 只通过 npm userconfig 或环境提供，不得写入 Package、lockfile、日志或回复。不得把 package-manager cache/store 重定向到仓库。

## 核心边界

- Entity 在所属 Scope 内唯一，默认私有，只有显式 Export 才公开。
- Import 创建 Projection，不复制 Entity，也不改变 ownership。
- 本地 Source identity 使用 `file://`；Package identity 使用不可变的 `npm:<name>@<version>`。
- Scope 声明 Package 名，`package.json` 声明版本范围，package-manager lockfile 固定解析结果。
- `locus-scope-node` 读取 `node_modules`，不读取 `locus.lock` 或 `.locus/packages`。
- npm 模式不运行 `locus-pkg install`、`update` 或 `uninstall`。
- bare Import 不能携带 `@version` 或 `/subpath`。
- `:` 分隔 imported Projection，`/` 表示 Scope 内 Group path。
- Locus 描述系统，不负责 provisioning、运行状态或 deployment drift reconciliation。

## 项目

- [SWdrift/locus-scope](https://github.com/SWdrift/locus-scope)
