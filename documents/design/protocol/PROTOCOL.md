# PROTOCOL

本文是 `locus-scope` 核心语义的权威定义。示例使用 YAML 表达逻辑结构；YAML 本身不是 Core 强制的唯一 codec。

## Entity

Entity 是 `locus-scope` 中的基本对象。每个 Entity 只有一个必需标识：`id`。

Entity 可以具有任意属性。`locus-scope` 不规定这些属性的结构、字段、类型、内部写法或业务语义；属性由具体 Entity 类型、使用者或实现自行解释。

Entity 可以表示任何对象，也可以表示需要独立属性、身份或进一步参与关系的关系实例。

## Scope

Scope 是 Entity 的归属与命名边界。

- 每个 Entity 恰好属于一个 Scope。
- Entity 的规范 ID 只要求在所属 Scope 内唯一。
- Scope 不因 Import、Export 或 Projection 改变 Entity ownership。

## Export

Scope 内的 Entity 默认私有。只有被 Scope 显式 export 的 Entity 或 Projection 才构成该 Scope 的公开内容。

```yaml
exports:
    - api
    - infra:database
```

Export 不复制 Entity，也不改变 Entity ownership。

## Import

Scope 可以导入其他 Scope。每个 Import 在当前 Scope 中声明一个 Projection 前缀：

```yaml
imports:
    infra: infra-source
```

`infra-source` 只表示由实现解析的 Source 声明；Core 不规定它采用本地路径、Package reference 或其他定位形式。

Import 只使目标 Scope 的公开内容在当前 Scope 中可见：

- 未 export 的目标成员不可见；
- 导入内容不会隐式展平到当前 Scope；
- 被导入 Entity 仍归属于其原始 Scope；
- Scope 之间允许任意组合关系，包括循环。

`locus-scope` Core 只定义循环是合法的组合形态，不规定加载器、缓存或诊断如何实现循环处理。

## Projection

导入内容始终通过 `:` 分隔的 Projection 前缀访问：

```text
infra:server
```

Scope 可以再次 export 已导入的 Projection。若 `app` export `infra:database`，而另一个 Scope 以 `product` 导入 `app`，则该内容可写作：

```text
product:infra:database
```

`:` 保留给 Projection，不用于 Scope 内的 Group 路径。

## Relation

Relation 是两个 Entity 之间的有向边：

```text
<entity-ref> <relation> <entity-ref>
```

例如：

```text
api uses infra:database
server located_in datacenter
```

规则：

1. Relation 的起点和终点都必须解析为 Entity。
2. Relation 自身没有属性。
3. Relation 名称表达边的语义，但 `locus-scope` Core 不规定名称词表。
4. 需要属性、身份或进一步参与 Relation 的关系，必须建模为 Entity。

Relation 与 Entity 共同形成有向 Entity 图。

## Scope 定义格式

一个 Scope 由一个 manifest 和任意数量的 definition documents 组成：

```text
scope/
├── locus.yaml
└── *.yaml / *.json / ...
```

文件边界没有协议语义。具体实现可以选择存储和序列化方式，但逻辑结构必须保持本文定义的 Scope、Entity、Relation、Import 和 Export 语义。

### Manifest

`locus.yaml` 或等价 manifest 承载 Scope 级信息：

```yaml
id: app

imports:
    infra: infra-source

exports:
    - api
    - infra:database
```

本文只固定 `id`、`imports` 和 `exports` 的逻辑角色；路径解析、Source 获取和 codec 选择属于实现层。

### Definition document

一个 definition document 可以定义一个或多个 Entity，也可以定义 Relation；两者允许混写：

```yaml
entities:
    - id: api
      type: service
      runtime: go
      port: 8080

    - id: worker
      anything:
          any:
              structure: allowed

relations:
    - [api, calls, worker]
    - [api, uses, infra:database]
```

除 `id` 外，Entity 中的所有内容均为不透明属性。

## Group

Definition document 可以声明可选的 `group`：

```yaml
group: backend

entities:
    - id: api
    - id: worker

relations:
    - [api, calls, worker]
```

其定义结果等价于 Scope 内的：

```text
backend/api
backend/worker
```

Group 只是在读取该 definition document 时为其中声明的 Entity ID 增加局部前缀。加入前缀后的完整 Scope 内 ID 才参与唯一性判断。

在同一 grouped document 中，指向该 document 内声明 Entity 的未限定引用应用相同前缀。因此上例 Relation 的两端分别解析为 `backend/api` 和 `backend/worker`。

Group：

- 不是 Scope；
- 不改变 Entity ownership；
- 不参与 Import 或 Export 语义；
- 不创建额外可见性边界。

`/` 表示 Scope 内的 Group 路径，`:` 表示 imported Projection：

```text
api
backend/api
infra:server
infra:backend/api
```

## 引用与解析不变量

Entity reference 只能解析为以下两类目标：

1. 当前 Scope 中的 Entity，例如 `api` 或 `backend/api`；
2. 通过一个或多个 Projection 暴露的 Entity，例如 `infra:server` 或 `platform:infra:backend/api`。

解析必须保持以下边界：

- `/` 后仍处于同一 Entity 的 Scope 内 ID；
- 每个 `:` 都跨越一个显式 Import/再 Export Projection；
- Projection 只暴露目标 Scope 的公开内容；
- 解析不会把导入 Entity 重新归属于当前 Scope。

## Core invariants

1. 一个 Entity 恰好属于一个 Scope。
2. Entity 规范 ID 只在所属 Scope 内唯一。
3. Entity 可以具有任意属性，`locus-scope` 不规定属性的内部结构或语义。
4. Scope 内成员默认私有。
5. Export 决定 Scope 的公开内容。
6. Import 不改变 Entity ownership。
7. Import 内容始终通过 Projection 前缀访问，不隐式展平。
8. Imported Projection 可以再次 export。
9. Scope 之间允许任意组合关系，包括循环。
10. Relation 构成 Entity 之间的有向图。
11. Relation 本身无属性；需要独立语义的关系可以实体化。
12. Group 只是 definition format 中的局部 ID 前缀，不是新的 Scope。
13. `locus-scope` Core 不规定存储、序列化、包管理、传输、查询或执行实现。

