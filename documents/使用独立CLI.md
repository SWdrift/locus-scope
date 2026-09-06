# 使用独立 CLI

独立 CLI 模式直接运行 `locus-scope` 和 `locus-pkg`，不依赖 Node.js、npm 或 pnpm。它适合只需要描述和查询 Entity 图的项目、非 JavaScript 项目，以及希望由 Locus 自己管理 Package lock 和离线缓存的环境。

## 安装

Windows 运行 `locus-setup-windows-amd64.exe`：

- 只使用本地 Scope 时安装 `locus-scope`。
- 需要从 Registry 安装、更新或发布 Package 时，同时安装 `locus-pkg`。
- 如安装器修改了当前用户 `PATH`，安装后需要新开终端。

创建最小 Scope：

```yaml
# app/locus.yaml
id: app
```

验证安装和文件读取：

```text
locus-scope --scope ./app validate
```

## 使用本地 Scope

`locus-scope` 负责加载、验证和操作 Workspace：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity
locus-scope --scope ./app relation
```

进入 `app` 或其子目录后可以省略 `--scope`。完整的建模和 CLI 操作步骤见[基本使用](基本使用.md)。

只使用本地 Scope 时不需要 `package.json`、`locus.lock` 或 `.locus/`。

## 安装 Package

独立 CLI 使用 npm-compatible Registry，但不调用 npm 或 pnpm。消费 Package 的项目需要把 `package.json` 与 `locus.yaml` 放在同一目录：

```json
{
  "name": "example-app",
  "private": true,
  "dependencies": {
    "@example/infra": "^1.0.0"
  }
}
```

在该目录执行：

```text
locus-pkg install
locus-scope validate
```

`locus-pkg install` 维护以下项目状态：

- `package.json`：直接依赖及其版本范围。
- `locus.lock`：完整且确定的依赖解析结果。
- `.locus/`：下载缓存和展开后的 Package。

也可以直接变更依赖：

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg update @example/infra
locus-pkg uninstall @example/infra
locus-pkg list
```

不要再用 npm 或 pnpm 修改同一项目的依赖布局。两套工具不会共享 lock、缓存或安装目录。

## CI 与离线运行

CI 中要求声明、lock 和已解析依赖完全一致：

```text
locus-pkg install --frozen-lockfile
locus-scope --json validate
```

已经具备完整 lock 和 `.locus/` 状态时，可以禁止所有 Registry 请求：

```text
locus-pkg install --offline --frozen-lockfile
locus-scope --json validate
```

离线状态不完整时命令会失败，不会隐式联网补齐。

## 打包与发布

可发布 Package 仍使用标准 `package.json`，并通过 `locus.entry` 指向 Package 内的 `locus.yaml`：

```json
{
  "name": "@example/infra",
  "version": "1.0.0",
  "files": ["locus.yaml", "resources.locus.yaml"],
  "locus": {
    "entry": "locus.yaml"
  },
  "exports": {
    "./package.json": "./package.json"
  }
}
```

在 Package 根目录执行：

```text
locus-pkg pack
locus-pkg publish --registry https://registry.example.com/
```

Registry 选择沿用 npm 的 `.npmrc` 规则，也可以使用 `--registry` 或 `NPM_CONFIG_REGISTRY`。Bearer token 使用对应 Registry 的 `_authToken` 或 `NPM_TOKEN`。不要把凭据写入 Package、lock、`.locus/` 或命令输出。

Package 格式、发布约束和 Registry 规则见[Package 设计](design/Package设计.md)。

## 与 npm 模式的边界

独立 CLI 和 npm 模式使用相同的 Scope、Entity、Relation、Import 和 Export 语义。差异只在 Package 环境：

- 独立 CLI 由 `locus-pkg` 生成 `locus.lock` 并维护 `.locus/`。
- npm 模式由 npm 或 pnpm 生成自己的 lockfile 并维护 `node_modules`。
- `locus-scope` 不读取 `node_modules`；`locus-scope-node` 不读取 `.locus/`。

已由 npm 或 pnpm 管理依赖的项目，应改用 [npm 模式](使用npm.md)。
