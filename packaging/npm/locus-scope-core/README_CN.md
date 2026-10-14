# @sundw/locus-scope-core

[中文](README_CN.md) | [English](README.md)

为 Node.js 和 TypeScript 应用提供进程内 Workspace 图。加载并验证组合后的 Scope，再通过 Core application API 查询 Entity 和 Relation，无需依赖 CLI 或解析命令。

要求 Node.js 24 或更高版本。本包使用 ES modules，并包含 TypeScript 类型声明。

## 安装

```sh
pnpm add @sundw/locus-scope-core
```

也支持 npm。依赖安装、lockfile 和缓存由 npm 或 pnpm 管理；Core 消费已解析的 Package Environment。

## 加载 Workspace

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

使用显式目录加载图：

```ts
import {
  ApplicationService,
  buildPackageEnvironment,
  loadPackageEnvironment,
  parseFilters
} from "@sundw/locus-scope-core";

const environment = await buildPackageEnvironment("./workspace");
const workspace = await loadPackageEnvironment(environment);
const application = new ApplicationService(workspace);
const api = workspace.resolve(workspace.root, "api");

// 省略 depth，遍历全部正向可达 Entity，支持有环图。
const graph = application.graph([api]);

// 查询直接入边、出边对应的相邻节点及匹配 Relation。
const adjacent = application.adjacent({
  entities: [api],
  direction: "both",
  via: parseFilters(["type=depends_on"])
});

console.log(graph.nodes, adjacent.relations);
```

Entity 身份为 `{ scope, id }`，局部 ID 不是全局唯一标识。将 EntityKey 传入图操作前，应相对于已加载 Scope 解析 Projection 引用。

## Application API

Application 提供 Entity、Relation、Scope、Group 查询，以及 `graph`、`path`、`impact`、`adjacent`、`diff`、`mutate` 和 `validate`。结果使用 Entity/Relation DTO，并可包含来源信息。`graph(seeds, 0)` 只返回 seeds；`adjacent` 支持 `incoming`、`outgoing` 和 `both`。

将已加载 Workspace 视为只读快照。Mutation 成功后重新加载，并为新 Workspace 创建 Application。旧快照保持可读，但不会自动反映后续写入；不支持直接修改内部集合。

需要 `locus-scope` 命令时使用 `@sundw/locus-scope`；仅需 library API 时使用本包。

## 文档

- [基本使用](https://github.com/SWdrift/locus-scope/blob/master/documents/1-基本使用.md)
- [使用 npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-使用npm.md)
- [CLI 参考](https://github.com/SWdrift/locus-scope/blob/master/documents/3-CLI参考.md)
- [核心 API 参考](https://github.com/SWdrift/locus-scope/blob/master/documents/4-核心API参考.md)
- [项目总览](https://github.com/SWdrift/locus-scope/blob/master/README_CN.md)

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
