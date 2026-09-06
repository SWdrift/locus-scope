---
name: locus-use
description: "指导用户从安装 Locus Scope 到创建、验证、查询和安装 npm Package，并解释 Entity、Relation、Scope、Import、Export、ownership、Pure 与 npm/pnpm 环境及离线复现。用于入门、安装后配置、基本使用、Package 消费或理念说明。"
metadata:
  domain: locus-scope
---

# Locus Scope · 从安装到使用

## 目标

用最短路径让用户安装 CLI、创建第一个 Scope、查询 Entity 图，并按需在 Pure Locus 或 npm/pnpm 环境安装 Package。先给可执行步骤，再解释模型和环境边界。

## 选择安装环境

### Windows 独立 CLI

运行 `locus-setup-windows-amd64.exe`，至少选择 `locus-scope`；需要 Pure Locus Package lifecycle 时同时选择 `locus-pkg`。安装目录固定为 `%USERPROFILE%\.locus`，安装器可把 `bin` 加入当前用户 `PATH`。

安装包只含独立 CLI，不含 Registry，也不要求 Go、Node.js、pnpm 或联网。安装后新开终端，创建只含以下内容的 `smoke/locus.yaml`：

```yaml
id: smoke
```

执行真实加载：

```text
locus-scope --scope ./smoke validate
```

### npm / pnpm

Node.js 20.6+ 项目安装 adapter：

```text
pnpm add @sundw/locus-scope
# 或
npm install @sundw/locus-scope
```

入口是 `locus-scope-node`。它从 importer 的 package manager 环境发现 Package，将 descriptor 交给同一 Go Scope 核心；JavaScript 不解析 Locus definitions。

### 仓库开发环境

```powershell
pnpm run deploy
```

独立 CLI 位于 `temp/local/bin/`。只有明确需要写用户目录时才运行 `pnpm run deploy:user`。本地 Registry 由根 package scripts 管理，不属于 CLI 部署或安装包。

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
```

进入 Scope 目录后可省略 `--scope`。CLI 沿祖先链寻找最近的 Scope manifest。Agent 或脚本追加 `--json` 获取字段与顺序稳定的输出。

Relation 的 definition input 和 CLI JSON output 不同：

- YAML 使用三元组 `[from, relation, to]`。
- `relation list --json` 输出解析后的对象；`from` / `to` 含 owner 的 `scope_id`、Source identity `scope` 与 Entity `id`。
- `--json` 只切换 CLI 输出，不改变 definition document 语法。

## 本地 Scope 组合

root Scope 通过相对路径导入另一个本地 Scope：

```yaml
id: app
imports:
    infra: ../infra
```

Import 创建带 alias 的 Projection。访问 `infra:database` 时，目标必须显式 export `database`。本地 Source identity 使用规范 `file://` URL。

## 准备可消费的 Locus Package

一个可分发 npm Package 对应一个 Scope。其 `package.json` 至少声明：

```json
{
  "name": "@example/infra",
  "version": "1.0.0",
  "files": ["locus.yaml", "resources.locus.yaml"],
  "locus": {"entry": "locus.yaml"},
  "exports": {"./package.json": "./package.json"}
}
```

packed tree 中 `locus.entry` 必须是唯一 Scope manifest。分发 Package 不允许本地/绝对 Import；bare Import 必须对应 `dependencies`。存在 `exports` 时必须导出 `./package.json`，否则 Node adapter 会拒绝该 Locus Package。

consumer Scope 只写 bare npm Package 名，不带版本或 subpath：

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

解析后的 Package Source identity 是 `npm:@example/infra@1.2.3`。Import alias 只是 root Scope 中的 Projection 名，不决定 identity。

## Pure Locus 消费

独立工具直接解析 npm Registry SemVer graph：

```text
locus-pkg install
locus-scope validate
locus-scope resolve infra:database
```

也可由命令添加依赖：

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg update @example/infra
locus-pkg uninstall @example/infra
locus-pkg list
```

Pure Locus 保持三类状态：

- `package.json`：直接依赖范围。
- `locus.lock`：完整、确定的 Package graph 与 tarball integrity。
- `.locus/cache` / `.locus/packages`：项目内按 integrity 保存的 tgz 和展开内容。

普通 JavaScript Package 仍锁定和安装，但不进入 Scope graph。只有带有效 `locus.entry` 的 Package 及其直接 Locus edges 会形成 Package environment。

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

`--frozen-lockfile` 不改写 `package.json` 或 lock；`--offline` 禁止 metadata 和 tarball 请求。缺少完整 lock/cache/store 时应重新联网安装，而不是手工拼装目录。

## npm / pnpm 消费

package manager 同时安装 Locus Package 和 adapter：

```text
pnpm add @example/infra @sundw/locus-scope
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node resolve infra:database
```

或：

```text
npm install @example/infra @sundw/locus-scope
npx locus-scope-node validate
```

adapter 对 root importer 及每个已发现 Locus Package 分别做 importer-relative resolution，因此 npm hoisting、重复物理副本与 pnpm symlink layout 不改变 identity/edge 语义。多份相同 `name@version` 在 entry 与已解析 Locus edges 相同时合并；语义不同则明确失败。

不要让 Node workflow 读取 Pure `.locus/packages`，也不要让 Pure workflow 扫描 `node_modules`。两种环境共享 Go Scope/Graph 语义，各自由自己的 dependency manager 提供 Package graph。

## Registry 与本地开发

Pure Locus 的 Registry 选择顺序为 `--registry`、`NPM_CONFIG_REGISTRY`、项目 `.npmrc`、用户 `.npmrc`；匹配 scope 的 registry 配置优先。认证使用 `_authToken` 或 `NPM_TOKEN`。凭据不得进入 Package、lock、`.locus` 或输出。

仓库开发 Registry：

```powershell
pnpm install --frozen-lockfile
pnpm run registry:start
pnpm run registry:status
```

它仅监听 `http://127.0.0.1:4873/`；匿名读取允许，发布需要认证。若需登录，用隔离 userconfig：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
```

脚本不写 `.npmrc`，不重定向 package-manager cache/store。结束时运行 `stop`；只有明确删除全部开发 Registry state 时才运行 `reset -Force`。

## 理念

- **Entity graph 优先**：Entity 表示需独立身份、属性或关系的对象；Relation 是 Entity 间有向语义边。
- **Scope 是边界**：Entity 在所属 Scope 内唯一，默认私有，只有显式 Export 才公开。
- **组合而非复制**：Import 创建 Projection，不展平、不复制，也不改变原始 ownership。
- **来源决定身份**：本地 Source 使用 `file://`；Package 使用不可变的 `npm:<name>@<version>`。
- **声明与快照分离**：Scope 声明 Package 名，`package.json` 声明范围，环境 lock 固定解析结果。
- **安装联网，查询离线**：Package manager 负责获取与物化；Scope 查询不隐式访问 Registry。
- **描述而非执行**：Locus 不负责 provisioning、运行状态或 deployment drift reconciliation。

## 常见边界

- `locus-scope install` 不是命令；Pure Package 安装使用 `locus-pkg install`。
- bare Import 只能是合法 npm Package 名；不能携带 `@version` 或 `/subpath`。
- `locus-scope validate` 验证完整 reachable Workspace，不只检查 root manifest。
- `:` 分隔 imported Projection，`/` 表示 Scope 内 Group path。
- 同名 manifest ID 不代表同一 Scope；ownership 与 Source identity 由真实来源决定。
- 本地 Scope 可用相对 Import；分发 Package 内任何本地/绝对 Import 都无效。
