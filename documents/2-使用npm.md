# 使用 npm

npm 模式由 npm 或 pnpm 管理 Package 版本、lockfile、下载缓存和 `node_modules`，由 `@sundw/locus-scope` 提供 Workspace 操作入口。它适合现有 Node.js 项目、需要复用 npm 工具链的项目，以及同时包含 JavaScript Package 和 Locus Package 的依赖图。

## 安装

要求使用 Package 声明的 Node.js LTS 版本。在项目中安装 TypeScript Locus package：

```text
pnpm add @sundw/locus-scope
# 或
npm install @sundw/locus-scope
```

`@sundw/locus-scope` 提供 `locus-scope`。先创建最小 Scope：

```yaml
# locus.yaml
id: app
```

验证安装和文件读取：

```text
pnpm exec locus-scope validate
# 或
npx locus-scope validate
```

完整的建模和 CLI 操作步骤见[基本使用](1-基本使用.md)。

## 安装 Locus Package

Locus Package 是包含 `locus.entry` 的标准 npm Package。先在消费项目的 `locus.yaml` 中以 Package 名声明 Import：

```yaml
id: app
imports:
    infra: "@example/infra"
```

Package 名不携带版本或子路径。版本范围由同目录的 `package.json.dependencies` 声明，并由 package manager lockfile 固定解析结果。

使用 pnpm 安装并查询：

```text
pnpm add @example/infra
pnpm exec locus-scope validate
pnpm exec locus-scope entity infra:database
```

使用 npm：

```text
npm install @example/infra
npx locus-scope validate
npx locus-scope entity infra:database
```

## 执行命令

pnpm 项目统一通过 `pnpm exec locus-scope` 执行：

```text
pnpm exec locus-scope validate
pnpm exec locus-scope entity
pnpm exec locus-scope entity database
pnpm exec locus-scope relation
pnpm exec locus-scope graph backend --depth 1
```

npm 项目把命令前缀替换为 `npx locus-scope`。在 Scope 目录之外执行时，通过 `--scope <dir>` 指定位置：

```text
pnpm exec locus-scope --scope ./app validate
```

默认输出格式化 JSON；Agent、脚本或 CI 可以追加 `--json` 获取稳定的单行 JSON。

## 依赖和运行边界

npm 模式遵循 package manager 的正常工作流：

- 使用 `pnpm add`、`pnpm update`、`pnpm remove`，或对应的 npm 命令变更依赖。
- 提交 `package.json` 和 package manager lockfile。
- CI 使用项目既有的 frozen-lockfile 安装方式，再运行 `locus-scope validate`。
- 不为 Locus 建立第二套 Package lock、缓存或安装状态。

- Scope Import 只写依赖目标，不写版本或安装位置。npm 或 pnpm 负责版本解析和实际安装；package-environment adapter 从每个 Package 自己的依赖上下文生成指向 exact identity 的 importer-relative edge。Package Scope 的 Import 只沿自己的 edge 解析，不能从 root dependencies 或全局 Package 集合查找同名 Package。runtime 只消费这个 resolved Environment，并加载、验证 Scope 文件。

## 发布 Package

Agent 发布时使用 `$locus-publish-node`。可发布的 Locus Package 使用标准 npm metadata：先运行 Skill 自带的轻量校验脚本，再验证 Workspace，最后打包并发布同一个 npm tarball：

```text
node <skill-dir>/scripts/validate-package.mjs .
pnpm exec locus-scope validate
npm pack --json --ignore-scripts --pack-destination <workspace>/temp/locus-publish
npm publish <workspace>/temp/locus-publish/<filename> --ignore-scripts --registry https://registry.example.com/
```

发布前必须检查 `npm pack --json` 返回的文件列表和 SHA-512 integrity；发布后确认 Registry 中确切 `name@version` 的 `dist.integrity` 一致。Skill 是 Agent 工作流而非 Registry 强制门禁。

## 运行边界

项目只有一种 Package environment：

- npm 或 pnpm 负责版本解析、安装、lockfile 和缓存。
- runtime 发现 importer-relative 的已安装 Package graph，并在同一进程执行共享 domain/application API。
- 只使用本地 Scope 时仍使用同一个 `locus-scope` 命令，也不要求存在 Package Import。
