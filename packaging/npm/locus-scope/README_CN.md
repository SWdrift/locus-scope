# @sundw/locus-scope

[中文](README_CN.md) | [English](README.md)

`locus-scope` 命令用于查询、验证和修改组合后的 Scope Workspace。领域与 Application 实现由 `@sundw/locus-scope-core` 提供；本包也导出 Core library API。

要求 Node.js 24 或更高版本。

## 安装

```sh
pnpm add @sundw/locus-scope
pnpm exec locus-scope --help
```

也支持 npm：

```sh
npm install @sundw/locus-scope
npx locus-scope --help
```

Scope package 的安装、更新、打包和发布使用 npm 或 pnpm，不属于 Locus CLI 命令。

## 查询 Workspace

在 `./workspace` 目录中创建：

```yaml
# locus.yaml
id: app
exports: [api, db]
```

```yaml
# entities.locus.yaml
entities:
  - id: api
  - id: db
relations:
  - from: api
    type: depends_on
    to: db
```

运行：

```sh
pnpm exec locus-scope --scope ./workspace validate --json
pnpm exec locus-scope --scope ./workspace entity api --json
pnpm exec locus-scope --scope ./workspace graph api --depth 1 --json
pnpm exec locus-scope --scope ./workspace adjacent api --direction outgoing --via type=depends_on --json
pnpm exec locus-scope --scope ./workspace path api db --json
```

`--scope` 显式选择 root 目录；省略时从当前目录向上发现最近的 Scope manifest。`--json` 输出机器可读的结果和结构化错误；`--source` 增加声明来源。

`adjacent` 默认使用 `--direction both`。`graph -`、`impact -` 和 `adjacent -` 从 stdin 接收 Entity query envelope 或 envelope 数组，通过 `{ scope, id }` 保留跨 Scope 身份。

成功退出码为 `0`，参数错误为 `2`，其他错误为 `1`。不要解析人类可读的错误文本；使用 JSON 错误的 `code` 和 `reason`。

## Library 使用

只需要 library API 时使用 `@sundw/locus-scope-core`。本包继续导出其 API，现有 consumer 可从 `@sundw/locus-scope` 导入 `ApplicationService`、`buildPackageEnvironment` 和 `loadPackageEnvironment`。

## 文档

- [基本使用](https://github.com/SWdrift/locus-scope/blob/master/documents/1-基本使用.md)
- [使用 npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-使用npm.md)
- [CLI 参考](https://github.com/SWdrift/locus-scope/blob/master/documents/3-CLI参考.md)
- [核心 API 参考](https://github.com/SWdrift/locus-scope/blob/master/documents/4-核心API参考.md)
- [项目总览](https://github.com/SWdrift/locus-scope/blob/master/README_CN.md)

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
