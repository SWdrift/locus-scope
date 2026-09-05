# locus-scope

可组合 Entity 图的 Scope 协议及轻量工具，定义身份、关系、作用域和组合方式。

Locus Scope 将具有身份的事物及其关系组织成有边界的图。Entity 可以表示环境、资源、能力、代码、知识、逻辑结构或其他领域对象；Scope 提供 ownership、命名、可见性与组合边界。一个可分发的 npm Package 对应一个 Scope，并通过标准 npm Registry 在项目之间复用。

## 安装方式

### Windows 独立 CLI

运行 `locus-setup-windows-amd64.exe`，选择 `locus-scope`、`locus-pkg` 或两者，并按需把 `%USERPROFILE%\.locus\bin` 加入当前用户 `PATH`。安装包只包含独立 CLI，不包含 Registry，也不要求 Node.js。

安装后新开终端，并验证真实 Scope：

```yaml
# smoke/locus.yaml
id: smoke
```

```text
locus-scope --scope ./smoke validate
```

### npm / pnpm 环境

要求 Node.js 20.6 或更新版本。在项目中安装 Node adapter：

```text
pnpm add @locus/scope
# 或
npm install @locus/scope
```

它提供 `locus-scope-node`，并通过当前平台的可选依赖运行与独立 CLI 相同的 Go Scope 核心。npm/pnpm 负责依赖布局，adapter 不要求固定的 `node_modules` 结构。

### 从源码构建

要求 Go 1.26+ 与 PowerShell 7：

```powershell
pwsh -File scripts/local-build.ps1
pwsh -File scripts/local-deploy.ps1
```

产物位于 `temp/local/bin/`。只有明确需要写入用户目录时才运行 `pwsh -File scripts/local-deploy.ps1 -User`。脚本不修改 `PATH`，详情见 [`scripts/README.md`](scripts/README.md)。

## 创建 Scope

创建 `app/locus.yaml`：

```yaml
id: app

exports:
    - backend
```

在普通子目录创建 `app/model/services.locus.yaml`：

```yaml
entities:
    - id: database
      type: postgres
      host: db.internal
      port: 5432

    - id: backend
      type: service

relations:
    - [backend, uses, database]
```

Definition document 必须使用 `.locus.yaml`、`.locus.yml` 或 `.locus.json` 后缀；普通 YAML/JSON 不会被读取。目录不会产生隐式 Group。Loader 会递归普通子目录，但遇到另一个 Scope manifest 时停止；该 Scope 只有经 `imports` 才会加入 Workspace。

可选的 `.locusignore` 使用 `/` 分隔的 Scope 相对模式，支持单路径段内的 `*`、`?` 和字符类，不支持 `!` 与 `**`。`.git` 和 `.locus` 始终跳过。

验证和查询：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity list
locus-scope --scope ./app entity show database
locus-scope --scope ./app relation list
locus-scope --scope ./app resolve database
```

在 Scope 内执行时可以省略 `--scope`，CLI 会沿祖先链查找最近的 Scope manifest。

## 本地组合与 Package 组合

本地 Scope 使用显式相对路径：

```yaml
id: app
imports:
    local_infra: ../infra
```

远程 Package Import 使用不带版本和子路径的 npm Package 名：

```yaml
id: app
imports:
    infra: "@example/infra"
```

Package 的版本约束只声明在 `package.json.dependencies`，不会重复写入 Scope manifest。解析后的稳定 Source identity 是 `npm:<name>@<version>`；同一 Workspace 可以通过不同 importer edge 同时使用一个 Package 的多个版本。

## 编写可发布的 Locus Package

一个 Package 必须有严格的 `package.json`，并用 `locus.entry` 指向 Package 内唯一的 Scope manifest：

```json
{
  "name": "@example/infra",
  "version": "1.0.0",
  "files": [
    "locus.yaml",
    "resources.locus.yaml"
  ],
  "locus": {
    "entry": "locus.yaml"
  },
  "exports": {
    "./package.json": "./package.json"
  },
  "dependencies": {
    "@example/base": "^1.0.0"
  }
}
```

`files` 必须是非空的相对 literal 文件/目录列表。首个版本不支持 glob、`.npmignore`、bundled dependencies、symlink 或依赖 lifecycle script 的打包。root `package.json` 以及 root README/LICENSE/LICENCE/NOTICE 文件会自动包含；`.git`、`.locus`、`node_modules`、`locus.lock` 和生成的归档始终排除。

发布 Package 内的 bare Import 必须对应其 `dependencies`；分发 Package 不允许本地或绝对 Import。如果声明 `exports`，必须显式保留 `"./package.json": "./package.json"`，供 Node adapter 从真实 importer 上下文发现 Package。

## Pure Locus 工作流

Pure Locus 使用独立 `locus-pkg` 管理 Registry SemVer 依赖，不调用 npm 或 pnpm。消费项目的 `package.json` 与 Scope 位于同一 root：

```json
{
  "name": "example-app",
  "private": true,
  "dependencies": {
    "@example/infra": "^1.0.0"
  }
}
```

```text
locus-pkg install
locus-scope validate
locus-scope resolve infra:database
```

`install` 生成 `locus.lock`，并按内容完整性将 tarball cache 与展开内容保存在项目 `.locus/`。普通 JavaScript Package 仍会锁定和安装，但只有带有效 `locus.entry` 的 Package 会进入 Scope graph。

常用依赖操作：

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg update @example/infra
locus-pkg uninstall @example/infra
locus-pkg list
```

严格复现与断网复用：

```text
locus-pkg install --frozen-lockfile
locus-pkg install --offline --frozen-lockfile
locus-scope --json validate
```

`--frozen-lockfile` 要求 `package.json` 与完整 lock graph 一致，不改写声明或 lock；`--offline` 禁止任何网络请求，只有 lock、cache 和 store 足够完整时成功。日常 `locus-scope` 查询也不会隐式访问 Registry。

## npm / pnpm 工作流

由 package manager 安装同一 Locus Package 和 Node adapter：

```text
pnpm add @example/infra @locus/scope
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node resolve infra:database
```

npm 等价用法：

```text
npm install @example/infra @locus/scope
npx locus-scope-node validate
```

adapter 从每个 importer 的 `package.json` 上下文解析直接依赖，因此 npm hoisting 与 pnpm symlink layout 得到相同语义。JavaScript 不解析 Scope definitions；它只构造已解析 Package descriptor，并把查询交给随平台 Package 分发的 Go host。

Pure Locus 与 npm/pnpm 两种环境共享 Scope/Graph 语义，但依赖状态分别由 `locus.lock`/`.locus` 和 package manager lock/安装布局管理，不应混用两者的物理目录。

## 打包与发布

在带 `locus.entry` 的 Package root 执行：

```text
locus-pkg pack
locus-pkg publish --registry https://registry.example.com/
```

`pack` 生成 npm 约定名称的 `.tgz`；`publish` 先构建相同格式的临时 tarball，再向 Registry 发布当前不可变 `name@version` 和 `latest` dist-tag。重复发布同一版本会失败；要发布变化内容必须先更新 `package.json.version`。

Registry 选择顺序是 `--registry`、`NPM_CONFIG_REGISTRY`、项目 `.npmrc`、用户 `.npmrc`，匹配 Package scope 的 registry 配置优先。Bearer token 可来自相应 registry 的 `_authToken` 或 `NPM_TOKEN`。不要把凭据提交到 `package.json`、`locus.lock`、`.locus` 或命令输出。

### 项目本地 Verdaccio

仓库开发使用固定版本的 project-local Registry：

```powershell
pwsh -File scripts/npm-registry.ps1 install
pwsh -File scripts/npm-registry.ps1 start
pwsh -File scripts/npm-registry.ps1 status
pwsh -File scripts/npm-registry.ps1 logs
pwsh -File scripts/npm-registry.ps1 stop
pwsh -File scripts/npm-registry.ps1 reset -Force
```

服务只监听 `http://127.0.0.1:4873/`，状态、认证文件、storage 和日志只写入 `temp/verdaccio-dev/`。匿名读取允许，发布必须认证。脚本不会创建仓库或用户 `.npmrc`，也不会重定向 npm/pnpm cache 或 store。

需要本地登录时，把 package manager 的 userconfig 隔离到 Registry state 下，而不是修改真实用户配置：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
$env:NPM_CONFIG_REGISTRY = 'http://127.0.0.1:4873/'
```

## CLI

### `locus-scope` / `locus-scope-node`

| 命令 | 作用 |
| --- | --- |
| `validate` | 验证完整 Workspace，并输出 root identity 及 Scope、Entity、Relation 数量。 |
| `scope show` | 显示 root Scope 的 manifest ID、Source identity、Imports 和 Exports。 |
| `scope list` | 列出所有可达 Scope，并标记 root Scope。 |
| `entity list` | 列出所有可达 Entity 及其原始 owner。 |
| `entity show <ref>` | 从 root Scope 解析 Entity reference，并显示 owner、ID 和属性。 |
| `relation list` | 列出验证后的 Relation 及两端 Entity 的原始 owner。 |
| `resolve <ref>` | 解析 Entity reference，返回最终 owner 和 ID。 |
| `version` | 输出版本，不发现或加载 Scope。 |

通用参数：`--scope <dir>` 指定 root Scope，`--json` 输出稳定 JSON，`--version` 等价于 `version`。

### `locus-pkg`

| 命令 | 作用 |
| --- | --- |
| `install [<package-spec>...]` | 安装或变更直接依赖并提交 `package.json`、lock 和 store transaction。 |
| `uninstall <package>...` | 删除直接依赖并剪除不可达 Package。 |
| `update [<package>...]` | 在现有约束内更新指定或全部直接依赖。 |
| `list` | 从 lock/store 输出已解析依赖树。 |
| `pack` | 生成确定性的 npm tarball。 |
| `publish` | 打包并发布当前不可变版本。 |
| `version` | 输出版本，不发现项目。 |

选项可放在子命令前后：`--registry <url>` 选择 Registry，`--offline` 禁止网络，`--frozen-lockfile` 要求精确复用 lock，`--json` 输出稳定 JSON。失败的 JSON 统一写到 stderr，形如 `{"error":"..."}`。

## 核心理念

- **Scope 是边界**：Entity 在所属 Scope 内唯一，默认私有，只有显式 Export 才公开。
- **组合而非复制**：Import 创建带别名的 Projection，不展平、不复制 Entity，也不改变 ownership。
- **来源决定身份**：本地 Source 使用规范 `file://` identity，Package 使用 `npm:<name>@<version>`。
- **声明与快照分离**：Scope manifest 声明 bare Package 名，`package.json` 声明范围，环境自己的 lock 固定解析结果。
- **安装联网，查询离线**：网络与物化属于 Package 环境；Scope 加载、验证和查询不隐式联网。
- **描述而非执行**：Locus 描述、组合、分发和查询 Entity 图，不负责 provisioning 或运行时 reconciliation。

## 技术栈

- Go 1.26+
- Node.js 20.6+（仅 npm/pnpm adapter）
- npm-compatible Registry

## License

[MIT](LICENSE)
