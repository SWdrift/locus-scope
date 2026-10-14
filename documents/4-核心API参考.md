# 核心 API 参考

[中文](4-核心API参考.md) | [English](4-Core-API-Reference.md)

本篇介绍 `@sundw/locus-scope-core` 当前可调用的 TypeScript API，用于在 Node.js 应用中加载 Workspace 并消费内存图。Scope 建模见[基本使用](1-基本使用.md)，依赖安装见[使用 npm](2-使用npm.md)，命令行入口见 [CLI 参考](3-CLI参考.md)。稳定能力边界以[核心 API 契约](design/protocol/CORE_API.md)为准；本文不新增协议。

## 安装与加载

要求 Node.js 24 或更高版本，使用 ES modules：

```sh
pnpm add @sundw/locus-scope-core
# 或
npm install @sundw/locus-scope-core
```

`@sundw/locus-scope` 也导出 Core API；仅需 library 时直接依赖 Core。

在 `./app` 创建以下文件：

```yaml
# locus.yaml
id: app
exports: [api, db]
```

```yaml
# entities.locus.yaml
entities:
  - id: api
    type: service
  - id: db
    type: database
relations:
  - from: api
    type: depends_on
    to: db
```

加载并验证本地 Scope 及其 Import 闭包：

```ts
import {
  ApplicationService,
  ApplicationError,
  buildPackageEnvironment,
  loadPackageEnvironment,
  parseFilters
} from "@sundw/locus-scope-core";

const loadWorkspace = async () => loadPackageEnvironment(await buildPackageEnvironment("./app"));

let application = new ApplicationService(await loadWorkspace());
const workspace = application.getWorkspace();
const api = workspace.resolve(workspace.root, "api");
const db = workspace.resolve(workspace.root, "db");
```

`buildPackageEnvironment` 使用显式 Scope 目录构造已安装 npm/pnpm dependency graph；它不安装依赖。`loadPackageEnvironment` 返回已加载并验证的 Workspace。只使用本地 Scope 时可以没有 `package.json`。加载失败直接抛错，不会返回部分图。

## 身份与引用

`EntityKey` 为 `{ scope: ScopeKey, id: string }`；`ScopeKey` 是字符串，不是 manifest 的 `id`。不同 Scope 的同名 Entity 是不同节点。

`workspace.resolve(fromScope, reference)` 相对于已加载的 `fromScope` 解释引用。例如 `infra:server` 通过 Projection 解析，并遵循 Import/Export 可见性。不同 Projection 指向同一个 Entity 时得到同一个 EntityKey。图 API 接收 EntityKey，不接收 Projection 字符串。

## 查询与 DTO

当前查询入口是以下方法，不是一个接受命令字符串的 `Query` 函数：

| 方法                                       | 返回值                        |
| ------------------------------------------ | ----------------------------- |
| `queryScopes(predicates?, withSource?)`    | Scope DTO 数组                |
| `getScope(reference, withSource?)`         | 单个 Scope DTO                |
| `queryGroups(predicates?, withSource?)`    | Group DTO 数组                |
| `getGroup(reference, withSource?)`         | 单个 Group DTO                |
| `queryEntities(predicates?, withSource?)`  | Entity DTO 数组               |
| `getEntity(reference, withSource?)`        | 相对于 root 解析的 Entity DTO |
| `entityByKey(key, withSource?)`            | 指定规范身份的 Entity DTO     |
| `queryRelations(predicates?, withSource?)` | Relation DTO 数组             |

```ts
const services = application.queryEntities(parseFilters(["type=service"]), true);
const entity = application.entityByKey(api);
const relations = application.queryRelations(parseFilters(["type=depends_on"]));
```

Entity DTO 为 `{ key, ref?, object, source? }`；Relation DTO 为 `{ fromKey, toKey, object, source? }`。`object` 保留开放属性；Entity 包含 `id`，Relation 包含声明的 `from`、`type`、`to`。只有可从 root 解析的 Entity 才包含 `ref`。`withSource` 默认 `false`，启用后返回声明来源，不改变身份。

Core 根入口中的 `Entity`、`Relation` 是 domain 类型；需要 DTO 类型时使用 `EntityView`、`RelationView`、`ScopeView`、`GroupView`。

## 筛选

`parseFilters(string[])` 生成 `Predicates`；多个条件使用 AND。支持 `=`、`!=`、`^=`、`*=`、`~=`。值优先解析为 JSON，失败后作为字符串；点路径遍历 object，不索引 array。缺失字段不匹配。

`@scope`、`@group`、`@fromScope`、`@toScope` 用于适用的 envelope metadata，采用 application 的 Scope reference（例如 `.`、`infra`），不是直接比较 ScopeKey。完整筛选规则见 [CLI 参考](3-CLI参考.md#filter)。图 API 的 `via` 使用同一 Relation predicates。

## 图操作

所有图操作同步返回 DTO，不改变 Workspace。`via` 默认 `[]`，`withSource` 默认 `false`。

| 方法                                      | 结果字段                                |
| ----------------------------------------- | --------------------------------------- |
| `graph(seeds, depth?, via?, withSource?)` | `seeds`、`depth?`、`nodes`、`relations` |
| `path(from, to, via?, withSource?)`       | `from`、`to`、`nodes`、`relations`      |
| `impact(seeds, via?, withSource?)`        | `seeds`、`affected`、`relations`        |
| `adjacent(request, withSource?)`          | `seeds`、`nodes`、`relations`           |

### Graph

```ts
const allReachable = application.graph([api]);
const seedsOnly = application.graph([api], 0);
const oneHop = application.graph([api], 1, parseFilters(["type=depends_on"]));
```

省略 depth 遍历全部正向可达节点，使用 visited 处理环。depth 为 `0` 时只返回去重 seeds，无 Relation；负数是参数错误。无上限结果省略 `depth` 字段。CLI 的默认 depth 不决定 Core API 的默认行为。

### Path 与 Impact

```ts
const shortest = application.path(api, db);
const reverseReachable = application.impact([db]);
```

Path 返回确定的最短有向路径，节点和 Relation 按路径顺序排列；没有路径时抛出 `scope.path_unreachable`。同一起终点返回该节点。Impact 沿入边反向遍历，`affected` 不包含 seeds。

### Adjacent

```ts
const neighbors = application.adjacent(
  {
    entities: [api],
    direction: "both",
    via: parseFilters(["type=depends_on"])
  },
  true
);
```

`direction` 必须为 `incoming`、`outgoing` 或 `both`，没有隐式默认值。结果 `seeds` 为去重输入，`nodes` 包含输入及直接相邻节点，`relations` 为匹配边。空 entities 返回空数组；双向自环和多个输入匹配同一 Relation 时只返回一次。不同 type 的平行边保留。实体按规范身份排序，Relation 保持稳定顺序。

## Validate

```ts
const summary = application.validate();
```

返回 `{ valid, root, scopes, entities, relations }`。Application 使用已验证 Workspace，此方法返回其统计信息；重新检查文件变化需要重新加载 Workspace，不是调用 `validate()`。

## Diff

```ts
const difference = await application.diff("scope:.", "path:./other-app", async directory =>
  loadPackageEnvironment(await buildPackageEnvironment(directory))
);
```

`diff(left, right, loadPath)` 接受 `scope:`、`group:`、`path:` selector。`scope:.` 和 `group:.#backend` 从当前 Workspace 选择内容；`path:` 调用宿主提供的 loader。以上 loader 加载目录；需要单个 definition document 时可由宿主使用 `loadDefinitionFile`。

结果为 `{ added, removed, changed }`，分别包含 Entity 和 Relation；changed 项为 `{ before, after }`。比较依据规范 EntityKey、Relation 三元组和开放属性，而不是原文件格式。

## Mutate 与快照

```ts
const mutation = await application.mutate(
  {
    kind: "entity",
    action: "set",
    ref: "api",
    fields: { owner: "platform" }
  },
  loadWorkspace
);

application = new ApplicationService(mutation.workspace);
```

`mutate(request, reload)` 返回 Promise，成功得到 `{ result, workspace }`。request 使用 `kind: "entity" | "relation"` 与 `action: "add" | "set" | "unset" | "remove"`；Entity 用 `ref` 定位，Relation 用 `from`、`type`、`to` 定位。

- add 使用 `object`，可指定 root 内的 `file`。
- set 使用 `fields` 更新非定位属性。
- unset 使用 `unset` 字段名数组。
- remove 删除指定声明。

依赖 Scope 只读；set/unset/remove 根据 provenance 写回，定位字段不可修改。宿主 reload 必须重新加载并验证；失败沿用回滚契约。详细规则见[写入规则](design/protocol/CLI.md#写入规则)。

加载完成的 Workspace 视为只读快照。成功 Mutation 返回新 Workspace，原 Application 不自动切换；旧 Workspace 和索引仍可读取，但不会反映后续写入。不支持直接修改内部集合。

## 错误处理

```ts
try {
  application.path(db, api);
} catch (error) {
  if (error instanceof ApplicationError && error.reason === "scope.path_unreachable") {
    console.log("没有有向路径");
  } else {
    throw error;
  }
}
```

ApplicationError 提供 `code`、`reason`、可选 `details` 和内部 `cause`。消费方按结构化字段分支，不解析 message，也不公开 cause 或 stack。完整分类见[错误契约](design/protocol/ERRORS.md)。
