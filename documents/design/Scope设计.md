# Scope 设计

## 简述

v1 从一个本地 root Scope 开始，沿 Imports 读取全部依赖，验证 Export 和 Relation，最终得到可查询的内存 Workspace。本文始终使用 `app` 导入 `infra` 的同一个例子说明文件如何变成 Workspace。

## 职责

本文负责本地 Source、文件发现、Workspace 装配、诊断和 `locus-scope` CLI 契约。不负责重新定义协议，也不负责远程分发、持久化、编辑或执行。

- Entity、Scope、Import、Export、Projection 和 Relation 的语义以 [PROTOCOL.md](protocol/PROTOCOL.md) 为唯一权威来源。
- Package Source 的获取、lock、cache 和安装流程由[Package设计](Package设计.md)负责。
- 代码目录与依赖方向只在[当前架构](../current-architecture.md)中维护，本文不重复目录树。

## 术语表

- **root Scope**：一次加载或 CLI 操作的入口 Scope。后文从 `app` 开始，因此 Workspace 的 `Root` 指向 `app`；“root”只表示入口，不禁止 Scope graph 中出现循环。
- **Source**：Loader 对一个可加载 Scope 的定位结果，包含稳定 identity `Key` 和可读目录 `LocalPath`。例如 `infra` 的 `LocalPath` 是 `/project/composition/infra`，本地 `Key` 是 `file:///project/composition/infra`。
- **Source identity / `ScopeKey`**：`Source.Key` 的类型，用于在 Workspace 中注册和区分 Scope。manifest `id`、Import alias 和目录参数写法都不能代替它。
- **reachable graph**：从 root Scope 沿全部 Imports 能到达的 Scope 闭包。本例是 `app` 和 `infra`；它由 Import 声明决定，与文件系统子目录遍历无关。
- **Resolver**：根据声明 Import 的 Source 和 Import value 返回目标 Source 的组件。本例的本地 Resolver 接收 `app` Source 和 `../infra`，返回 `infra` Source。
- **Workspace**：从 root Scope 装配并验证完成的内存图。本例的 Workspace 以 `app` 为 Root，包含 2 个 Scope、5 个 Entity 和 4 条解析后的 Relation；它不是目录，也不是单个 Scope 的别名。
- **`EntityKey`**：解析后的 Entity identity，由原始 owner 的 `ScopeKey` 和 Scope 内规范 ID 组成。本例的 `infra:database` 最终是 `{Scope: file:///project/composition/infra, ID: database}`。

## example：把 app 和 infra 装配成 Workspace

下面直接沿用[核心协议示例](protocol/examples/README.md)。假设这棵目录树位于 `/project/composition`：

```text
composition/
├── app/
│   ├── locus.yaml
│   └── services.yaml
└── infra/
    ├── locus.yaml
    ├── resources.yaml
    └── backend.yaml
```

`app/locus.yaml` 把相邻的 `infra` 目录导入为 Projection `infra`，并公开自己的 `api` 和再次公开的 `infra:database`：

```yaml
id: app

imports:
    infra: ../infra

exports:
    - api
    - infra:database
```

`app/services.yaml` 定义一个 Entity 和两条跨 Scope Relation：

```yaml
entities:
    - id: api
      type: service
      runtime: go
      port: 8080

relations:
    - [api, uses, infra:database]
    - [api, dispatches_to, infra:backend/worker]
```

`infra/locus.yaml` 只公开 `database` 和 `backend/worker`：

```yaml
id: infra

exports:
    - database
    - backend/worker
```

其余两个 definition documents 提供具体内容：

| 文件                   | 读取后的内容                                                                          |
| ---------------------- | ------------------------------------------------------------------------------------- |
| `infra/resources.yaml` | Entity `server`、`database`；Relation `server hosts database`                         |
| `infra/backend.yaml`   | Group `backend` 中的 Entity `worker`、`monitor`；Relation `worker reports_to monitor` |

## Scope 装配核心设计

以上文件以 `app` 为 root Scope 加载时，必须得到以 `file:///project/composition/app` 为 Root、包含 2 个 Scope、5 个 Entity 和 4 条 Relation 的 Workspace；`infra:database` 的 owner 必须是 `infra` Source。

| 边界          | 约束                                                                                                                                                                                                          |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| root 选择     | `--scope <dir>` 直接选择目录；未指定时从当前目录向父目录查找最近的 Scope manifest。                                                                                                                           |
| 本地 identity | `Source.LocalPath` 是消除符号链接后的规范绝对目录，`Source.Key` 是对应的规范 `file://` URI；`Manifest.ID` 和输入路径写法不参与 identity。同一 `ScopeKey` 在一次装配中只能对应一个 `LocalPath`。               |
| 文件发现      | 只读取 Scope 目录的直接文件；必须且只能存在一个 `locus.yaml`、`locus.yml` 或 `locus.json`；其余小写 `.yaml`、`.yml`、`.json` definition documents 按文件名排序，子目录不递归。                                |
| 解码          | YAML 与 JSON 使用同一严格结构；manifest `id`、Import alias/value 和 Export reference 必须有效。Entity `id` 单独保存，其余字段进入属性 map；Group 在读取时展开，展开后的 ID 参与重复检查，运行时不保留 Group。 |
| 本地 Import   | Import value 可以是相对声明 Scope 目录的路径或绝对目录；Resolver 必须据此返回目标 Source。                                                                                                                 |
| 装配          | Scope 解码后先按 `Source.Key` 注册，再由 Resolver 解析 Imports 并加载尚未注册的 Source；循环 Import 复用已注册 Scope。Import alias 在运行时映射到目标 `ScopeKey`。                                            |
| 解析          | `Workspace.Resolve` 必须遵守 [PROTOCOL.md](protocol/PROTOCOL.md) 的可见性和 ownership 规则；返回的 `EntityKey` 始终包含原始 owner。Relation 两端使用同一解析路径。                                            |
| 完成          | 全部 reachable Scopes 加载后先验证 Exports，再解析 Relations；Relation 按起点 ScopeKey、起点 ID、名称、终点 ScopeKey、终点 ID 排序。任一步失败都不返回 Workspace。                                            |
| 诊断          | 错误必须保留实际声明位置和失败边界：文件路径、Relation 行号、声明或 reference、相关 Scope；Manifest、Import、重复 ID、Group 展开和 SourceKey 冲突在各自边界报错。                                             |

## CLI

`locus-scope` 是 Scope 查询的薄入口，提供：

- `locus-scope validate`：加载并验证完整 reachable graph，成功时输出根 Scope 的来源 identity，以及 Scope、Entity 和 Relation 的数量。不会只检查根 Scope 的 locus.yaml 和 definition documents，而会沿着 imports 加载全部可达 Scope，并验证整个依赖闭包。
- `locus-scope scope show`：显示根 Scope 的 manifest ID、来源 identity、Import alias 及其目标、Export reference。
- `locus-scope scope list`：列出 reachable graph 中的全部 Scope，并标记根 Scope；每项包含 manifest ID 和来源 identity。
- `locus-scope entity list`：列出全部 reachable Entity；每项包含所属 Scope 的 manifest ID、Entity ID 和 owner Scope 的来源 identity。
- `locus-scope entity show <ref>`：从根 Scope 解析 `<ref>`，显示最终 Entity 的原始 owner、ID 和全部属性。
- `locus-scope relation list`：列出验证后的全部 Relation，显示 Relation 名称，以及解析到原始 owner 后的起点和终点。
- `locus-scope resolve <ref>`：从根 Scope 解析 `<ref>`，只返回最终 Entity 的 manifest ID、Entity ID 和 owner Scope 来源 identity，不读取其属性。

对于所有命令，有 option：

- `--scope <dir>` 直接选择目录；未指定时从当前目录向父目录查找最近的 Scope manifest。
- `--json` 文本输出供人阅读；JSON 字段和集合顺序必须稳定，供 Agent 和脚本消费。

另外所有命令必须先加载并验证完整 reachable graph，不能只检查 root Scope

## 验收

本设计按以下可观察行为验收：

- 从嵌套目录能够发现最近的 root Scope；不同路径写法指向同一实际目录时生成同一个规范 `file://` identity。
- [核心协议示例](protocol/examples/README.md)能够从 `app` 加载 `infra`，得到 2 个 Scope、5 个 Entity 和 4 条 Relation，并把 `infra:database` 解析到 `infra` owner。
- 再次 Export、私有成员、Group、循环 Import、相同 Manifest ID 的不同来源、重复 Entity ID 和无效 reference 的成功或失败边界都有 fixture 证明；错误包含对应文件、声明或 Scope 上下文。
- 构建后的真实 CLI 能完成上述验证和查询；完整测试分层、fixture、隔离规则与执行命令见[测试设计](测试设计.md)。
