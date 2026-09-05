# npm 模式

npm 模式由 npm 或 pnpm 管理 Package 版本、lockfile、下载缓存和 `node_modules`，由 `@sundw/locus-scope` 提供查询入口。它适合现有 Node.js 项目、需要复用 npm 工具链的项目，以及同时包含 JavaScript Package 和 Locus Package 的依赖图。

## 安装

要求 Node.js 20.6 或更新版本。在项目中安装 Node adapter：

```text
pnpm add @sundw/locus-scope
# 或
npm install @sundw/locus-scope
```

`@sundw/locus-scope` 提供 `locus-scope-node`。先创建最小 Scope：

```yaml
# locus.yaml
id: app
```

验证安装和文件读取：

```text
pnpm exec locus-scope-node validate
# 或
npx locus-scope-node validate
```

完整的建模和查询步骤见[基本使用](基本使用.md)。

## 安装 Locus Package

Locus Package 是包含 `locus.entry` 的标准 npm Package。使用 pnpm：

```text
pnpm add @example/infra @sundw/locus-scope
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node resolve infra:database
```

使用 npm：

```text
npm install @example/infra @sundw/locus-scope
npx locus-scope-node validate
npx locus-scope-node resolve infra:database
```

消费项目的 `locus.yaml` 只引用 Package 名：

```yaml
id: app
imports:
    infra: "@example/infra"
```

Package 名不携带版本或子路径。版本范围由同目录的 `package.json.dependencies` 声明，并由 package manager lockfile 固定解析结果。

## 执行命令

pnpm 项目统一通过 `pnpm exec locus-scope-node` 执行：

```text
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node entity list
pnpm exec locus-scope-node entity show database
pnpm exec locus-scope-node relation list
pnpm exec locus-scope-node resolve infra:database
```

npm 项目把命令前缀替换为 `npx locus-scope-node`。在 Scope 目录之外执行时，通过 `--scope <dir>` 指定位置：

```text
pnpm exec locus-scope-node --scope ./app validate
```

Agent、脚本或 CI 可以追加 `--json` 获取稳定 JSON 输出。

## 依赖和运行边界

npm 模式遵循 package manager 的正常工作流：

- 使用 `pnpm add`、`pnpm update`、`pnpm remove`，或对应的 npm 命令变更依赖。
- 提交 `package.json` 和 package manager lockfile。
- CI 使用项目既有的 frozen-lockfile 安装方式，再运行 `locus-scope-node validate`。
- 不运行 `locus-pkg install`，也不读取 Pure Locus 的 `locus.lock` 或 `.locus/`。

Node adapter 从每个 Package 自己的依赖上下文解析直接依赖，因此 npm hoisting 和 pnpm symlink layout 不改变 Scope 语义。JavaScript 只负责提供已解析的 Package 信息；Scope 文件仍由与独立 CLI 相同的 Go 核心加载和验证。

## 发布 Package

可发布的 Locus Package 使用标准 npm metadata，但发布前需要由 `locus-pkg` 校验和打包：

```text
locus-pkg pack
locus-pkg publish --registry https://registry.example.com/
```

Package 作者因此需要额外安装独立的 `locus-pkg`。完整步骤见[独立 CLI 模式的“打包与发布”](使用独立CLI.md#打包与发布)，格式约束见[Package 设计](design/Package设计.md)。

## 与独立 CLI 模式的边界

两种模式使用相同的 Scope 文件和查询语义，但依赖状态不能混用：

- npm 模式读取 package manager lockfile 和 `node_modules`，命令是 `locus-scope-node`。
- 独立 CLI 模式读取 `locus.lock` 和 `.locus/`，命令是 `locus-scope`。
- 只使用本地 Scope、不需要 Package 时，两种 CLI 都可以直接加载相同文件。

非 Node.js 项目或希望脱离 npm 工具链时，应改用[独立 CLI 模式](使用独立CLI.md)。
