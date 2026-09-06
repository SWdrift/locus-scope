---
name: locus-use
description: "指导用户安装和使用独立版 Locus Scope CLI，在 Pure Locus 环境创建、验证、查询 Scope，并通过 locus-pkg 管理 Package、lock、离线缓存和 Registry。用于非 Node.js 项目、独立 CLI、Pure Locus 或 locus.lock 工作流。"
metadata:
  domain: locus-scope
---

# Locus Scope · 独立 CLI 使用

## 目标

用最短路径让用户在不依赖 Node.js、npm 或 pnpm 的环境中安装 `locus-scope`，创建并查询 Scope；需要 Package 时使用 `locus-pkg` 管理 Registry、`locus.lock` 和项目内离线状态。npm/pnpm 项目改用 `$locus-use-node`。

## 安装

Windows 运行 `locus-setup-windows-amd64.exe`，至少选择 `locus-scope`；需要 Package lifecycle 时同时选择 `locus-pkg`。默认安装目录是 `%USERPROFILE%\.locus`，安装器可把 `bin` 加入当前用户 `PATH`。

安装包不含 Registry，也不要求 Go、Node.js、pnpm 或联网。安装后新开终端，创建 `smoke/locus.yaml`：

```yaml
id: smoke
```

执行真实加载：

```text
locus-scope --scope ./smoke validate
```

仓库开发环境使用：

```powershell
pnpm run deploy
```

独立 CLI 位于 `temp/local/bin/`。只有明确验证用户级安装时才运行 `pnpm run deploy:user`。

## 创建和查询 Scope

建立 `app/locus.yaml`：

```yaml
id: app
```

添加 `app/entities.locus.yaml`：

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

验证并查询：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity
locus-scope --scope ./app entity database
locus-scope --scope ./app relation
locus-scope --scope ./app graph backend --depth 1
```

进入 Scope 目录后可省略 `--scope`。CLI 沿祖先链寻找最近的 Scope manifest。Agent、脚本或 CI 追加 `--json` 获取稳定输出。

Relation definition 使用 `from`、`type`、`to` 三元组；`relation --json` 输出解析后的对象，其中端点包含 owner 的 `scope_id`、Source identity `scope` 和 Entity `id`。

## 本地 Scope 组合

root Scope 可通过相对路径导入另一个本地 Scope：

```yaml
id: app
imports:
    infra: ../infra
```

Import 创建带 alias 的 Projection。访问 `infra:database` 时，目标 Scope 必须显式 export `database`。本地 Source identity 使用规范 `file://` URL。

## Pure Locus Package

一个分发 Package 对应一个 Scope。consumer 的 `locus.yaml` 只写 bare npm Package 名：

```yaml
id: app
imports:
    infra: "@example/infra"
```

版本范围属于同目录的 `package.json.dependencies`：

```json
{
  "name": "example-app",
  "private": true,
  "dependencies": {
    "@example/infra": "^1.0.0"
  }
}
```

安装和查询：

```text
locus-pkg install
locus-scope validate
locus-scope entity infra:database
```

也可直接变更依赖：

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg update @example/infra
locus-pkg uninstall @example/infra
locus-pkg list
```

Pure Locus 保持三类状态：

- `package.json`：直接依赖范围；
- `locus.lock`：完整、确定的 Package graph 与 tarball integrity；
- `.locus/cache` 和 `.locus/packages`：项目内按 integrity 保存的 tgz 和展开内容。

普通 JavaScript Package 可以被锁定和安装，但不进入 Scope graph。只有带有效 `locus.entry` 的 Package 及其直接 Locus edges 会形成 Package environment。

CI 严格复现：

```text
locus-pkg install --frozen-lockfile
locus-scope --json validate
```

完全离线：

```text
locus-pkg install --offline --frozen-lockfile
locus-scope --json validate
```

`--frozen-lockfile` 不改写 `package.json` 或 lock；`--offline` 禁止 metadata 和 tarball 请求。缺少完整 lock、cache 或 store 时应重新联网安装，不得手工拼装状态。

## Registry

Registry 选择顺序为 `--registry`、`NPM_CONFIG_REGISTRY`、项目 `.npmrc`、用户 `.npmrc`；匹配 Package scope 的 Registry 配置优先。认证使用目标 Registry 的 `_authToken` 或 `NPM_TOKEN`。凭据不得进入 Package、lock、`.locus`、日志或回复。

仓库开发 Registry：

```powershell
pnpm install --frozen-lockfile
pnpm run registry:start
pnpm run registry:status
```

它仅监听 `http://127.0.0.1:4873/`。需要登录时，把 userconfig 隔离到仓库测试状态，不修改真实用户配置：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
```

不得把 package-manager cache/store 重定向到仓库。结束时运行 `pnpm run registry:stop`；只有明确删除全部开发 Registry state 时才运行 `pnpm run registry:reset`。

## 核心边界

- Entity 在所属 Scope 内唯一，默认私有，只有显式 Export 才公开。
- Import 创建 Projection，不复制 Entity，也不改变 ownership。
- 本地 Source identity 使用 `file://`；Package identity 使用不可变的 `npm:<name>@<version>`。
- Scope 声明 Package 名，`package.json` 声明版本范围，`locus.lock` 固定解析结果。
- Package 安装可以联网；Workspace 加载和查询不隐式访问 Registry。
- `locus-scope` 不读取 `node_modules`；不要把 npm 环境与 `.locus/packages` 混用。
- `locus-scope install` 不是命令；Package lifecycle 使用 `locus-pkg`。
- bare Import 不能携带 `@version` 或 `/subpath`。
- `:` 分隔 imported Projection，`/` 表示 Scope 内 Group path。
- Locus 描述系统，不负责 provisioning、运行状态或 deployment drift reconciliation。

## 项目

- [SWdrift/locus-scope](https://github.com/SWdrift/locus-scope)
