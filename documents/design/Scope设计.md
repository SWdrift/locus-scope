# Scope 设计

## 简述

v1 从一个本地 root Scope 开始，沿 Imports 读取全部依赖，验证 Export 和 Relation，最终得到可查询的内存 Workspace。本文通过基本 Scope 装配和显式 Group 两个示例说明文件如何变成 Workspace。

## 职责

本文负责本地 Source、文件发现、Workspace 装配、诊断和 `locus-scope` CLI 契约。不负责重新定义协议，也不负责 package 获取、持久化、编辑或执行。

- Entity、Scope、Import、Export、Projection 和 Relation 的语义以 [PROTOCOL.md](protocol/PROTOCOL.md) 为唯一权威来源。
- Package environment、npm identity、lock/store 和安装流程由[Package设计](Package设计.md)负责。
- 代码目录与依赖方向只在[当前架构](../current-architecture.md)中维护，本文不重复目录树。

## 术语表

- **root Scope**：一次加载或 CLI 操作的入口 Scope。基本示例从 `app` 开始，因此 Workspace 的 `Root` 指向 `app`；“root”只表示入口，不禁止 Scope graph 中出现循环。
- **Source**：Loader 对一个可加载 Scope 的定位结果，包含稳定 identity `Key` 和可读目录 `LocalPath`。基本示例中 `app` 的 `LocalPath` 是 `/project/app`，本地 `Key` 是 `file:///project/app`。
- **Source identity / `ScopeKey`**：`Source.Key` 的类型，用于在 Workspace 中注册和区分 Scope。manifest `id` 和输入路径写法不能代替它。
- **reachable graph**：从 root Scope 沿全部 Imports 能到达的 Scope 闭包；它由 Import 声明决定，与文件系统子目录遍历无关。基本示例没有 Import，因此只包含 `app`。
- **Resolver**：根据声明 Import 的 Source 和 Import value 返回目标 Source 的组件。本地路径由 local resolver 处理；bare npm name 由 importer-relative package environment 处理，详见[Package设计](Package设计.md)。
- **Workspace**：从 root Scope 装配并验证完成的内存图。基本示例的 Workspace 以 `app` 为 Root，包含 1 个 Scope、2 个 Entity 和 1 条 Relation；它不是目录，也不是单个 Scope 的别名。
- **`EntityKey`**：解析后的 Entity identity，由原始 owner 的 `ScopeKey` 和 Scope 内规范 ID 组成。基本示例的 `api` 最终是 `{Scope: file:///project/app, ID: api}`。

## example：基本 Scope 装配

一个项目以唯一的 root Scope manifest 作为入口。依赖 Package、lock 和物化状态由[Package设计](Package设计.md)定义，不在本例重复：

```text
app/
├── locus.yaml
└── services.locus.yaml
```

`app/locus.yaml` 声明项目的 root Scope，并公开 `api`：

```yaml
id: app

exports:
    - api
```

`app/services.locus.yaml` 定义两个 Entity 和一个 Relation：

```yaml
entities:
    - id: api
      type: service
      runtime: go
      port: 8080

    - id: worker
      type: service
      runtime: go

relations:
    - [api, dispatches_to, worker]
```

加载后，Workspace 必须以 `file:///project/app` 为 Root，包含 1 个 Scope、2 个 Entity 和 1 条 Relation；`api` 和 `worker` 的 owner 都是 `app` Source。

## example：显式 Group

Group 为同一 Scope 内的 Entity ID 添加显式前缀，与 definition document 所在目录无关：

```text
service/
├── locus.yaml
├── api.locus.yaml
└── model/
    └── backend.locus.yaml
```

`api.locus.yaml` 未声明 Group，因此 Entity 位于 Scope 的根命名空间：

```yaml
entities:
    - id: api
```

`model/backend.locus.yaml` 显式声明 `backend` Group：

```yaml
group: backend

entities:
    - id: api
    - id: worker

relations:
    - [api, calls, worker]
```

读取结果是 `api`、`backend/api` 和 `backend/worker` 三个 Entity；其中两个名为 `api` 的声明具有不同规范 ID，不发生冲突。Relation 两端展开为 `backend/api` 和 `backend/worker`。`model/` 只组织文件，不产生隐式 Group；Group 不是 Scope，也不创建 imports、exports 或可见性边界。

## Scope 装配核心设计

| 边界          | 约束                                                                                                                                                                                                                                                                                                                        |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| root 选择     | `--scope <dir>` 直接选择目录且不执行向上发现；未指定时从当前目录沿祖先链查找最近的 Scope manifest。查至文件系统根仍未找到就失败，不再查询用户目录、package cache 或 Registry。 |
| manifest      | 一个 Scope 必须且只能有一个位于其根目录的 `locus.yaml`、`locus.yml` 或 `locus.json`。                                                                                                                                                                                                                                      |
| 本地 identity | `Source.LocalPath` 是消除符号链接后的规范绝对目录，`Source.Key` 是对应的规范 `file://` URI；`Manifest.ID` 和输入路径写法不参与 identity。同一 `ScopeKey` 在一次装配中只能对应一个 `LocalPath`。                                                                                                                             |
| 文件发现      | Loader 从 Scope 根目录递归发现小写 `*.locus.yaml`、`*.locus.yml` 和 `*.locus.json` definition documents，按使用 `/` 分隔的规范相对路径排序后读取；普通 `.yaml`、`.yml` 和 `.json` 文件不是 Locus 输入。`.git`、`.locus` 目录始终跳过，目录符号链接不跟随。                                                          |
| 发现排除      | Scope 根目录可提供 `.locusignore`。每个非空、非 `#` 注释行是一条使用 `/` 的 Scope 相对模式：无 `/` 的模式匹配任意层级 basename，含 `/` 或以 `/` 开头的模式相对 Scope 根匹配，末尾 `/` 只匹配目录；支持单路径段内的 `*`、`?` 和字符类，匹配目录时跳过整棵子树。不支持 `!`、`**`、反斜杠、空路径和 `.`/`..` 路径段，无效规则必须带文件与行号报错。该文件只控制 Definition document 发现，不改变 manifest、Import 或 Package 发布内容。 |
| 目录边界      | 普通子目录只组织文件，不产生 Group、Source identity、Import、Export 或可见性边界。后代目录一旦包含 Scope manifest，就是独立 Scope 边界；父 Scope 的递归发现不得进入，也不得自动将其加入 Workspace，只有 manifest `imports` 显式引用时才作为独立 Source 加入 reachable graph。                                                    |
| Group         | Group 继续按[核心协议](protocol/PROTOCOL.md#group)由 definition document 的 `group` 字段显式声明，与目录路径无关。无论文档位于根目录还是普通后代目录，没有显式 Group 的 Entity 都属于 Scope 根命名空间；同一个 Group 可以出现在多个目录的文档中。Group 展开后的完整 ID 参与唯一性检查，运行时不保留 Group。                          |
| 解码          | YAML 与 JSON 使用同一严格结构；manifest `id`、Import alias/value 和 Export reference 必须有效。Entity `id` 单独保存，其余字段进入属性 map。                                                                                                                                                                                 |
| Import        | 本地 root Source 可使用相对声明 Scope 目录的路径或绝对目录。bare npm name 由 importer 的 package dependency edge 解析。npm package Source 只能使用 bare package Import，不能以本地路径越过 package 边界。 |
| 装配          | Scope 解码后先按 `Source.Key` 注册，再由 Resolver 解析 Imports 并加载尚未注册的 Source；循环 Import 复用已注册 Scope。Import alias 在运行时映射到目标 `ScopeKey`。                                                                                                                                                           |
| 解析          | `Workspace.Resolve` 必须遵守 [PROTOCOL.md](protocol/PROTOCOL.md) 的可见性和 ownership 规则；返回的 `EntityKey` 始终包含原始 owner。Relation 两端使用同一解析路径。                                                                                                                                                           |
| 完成          | 全部 reachable Scopes 加载后先验证 Exports，再解析 Relations；Relation 按起点 ScopeKey、起点 ID、名称、终点 ScopeKey、终点 ID 排序。任一步失败都不返回 Workspace。                                                                                                                                                           |
| 诊断          | 错误必须保留实际声明位置和失败边界：文件路径、Relation 行号、声明或 reference、相关 Scope；Manifest、Import、重复 ID、Group 展开和 SourceKey 冲突在各自边界报错。                                                                                                                                                            |

## CLI

`locus-scope` 是 Scope 查询的薄入口，提供：

- `locus-scope validate`：加载并验证完整 reachable graph，成功时输出根 Scope 的来源 identity，以及 Scope、Entity 和 Relation 的数量。不会只检查根 Scope 的 locus.yaml 和 definition documents，而会沿着 imports 加载全部可达 Scope，并验证整个依赖闭包。
- `locus-scope scope show`：显示根 Scope 的 manifest ID、来源 identity、Import alias 及其目标、Export reference。
- `locus-scope scope list`：列出 reachable graph 中的全部 Scope，并标记根 Scope；每项包含 manifest ID 和来源 identity。
- `locus-scope entity list`：列出全部 reachable Entity；每项包含所属 Scope 的 manifest ID、Entity ID 和 owner Scope 的来源 identity。
- `locus-scope entity show <ref>`：从根 Scope 解析 `<ref>`，显示最终 Entity 的原始 owner、ID 和全部属性。
- `locus-scope relation list`：列出验证后的全部 Relation，显示 Relation 名称，以及解析到原始 owner 后的起点和终点。
- `locus-scope resolve <ref>`：从根 Scope 解析 `<ref>`，只返回最终 Entity 的 manifest ID、Entity ID 和 owner Scope 来源 identity，不读取其属性。
- `locus-scope version`：输出构建时注入的 CLI 版本，不发现或加载 Scope；`--version` 与其等价。

对于所有命令，有 option：

- `--scope <dir>` 直接选择目录；未指定时从当前目录的祖先链选择最近的 Scope manifest，不做用户级回退。
- `--json`：输出字段和集合顺序稳定的 JSON，供 Agent 和脚本消费；`version` 输出 `name` 和 `version`。

除 `help` 和 `version` 外，所有命令必须先加载并验证完整 reachable graph，不能只检查 root Scope。

## 验收

本设计按以下可观察行为验收：

- 从普通嵌套目录能够发现最近的 root Scope；未找到时不做用户级回退；不同路径写法指向同一实际目录时生成同一个规范 `file://` identity。
- Loader 递归读取普通子目录内的 `*.locus.yaml`、`*.locus.yml` 和 `*.locus.json`，忽略其他 YAML/JSON；目录不产生隐式 Group，后代 Scope manifest 截断递归且只有经 `imports` 才进入 reachable graph。
- 基本装配示例必须得到以 `app` 为 Root、包含 1 个 Scope、2 个 Entity 和 1 条 Relation 的 Workspace。
- Group 示例必须得到 `api`、`backend/api` 和 `backend/worker`，并把同文档内的短 Relation 引用展开到 `backend` Group。
- 再次 Export、私有成员、显式 Group、循环 Import、相同 Manifest ID 的不同来源、重复 Entity ID 和无效 reference 的成功或失败边界都有 fixture 证明；错误包含对应文件、声明或 Scope 上下文。
- 构建后的真实 CLI 能完成上述验证和查询；完整测试分层、fixture、隔离规则与执行命令见[测试设计](测试设计.md)。
- 普通本地项目无需 `package.json`；bare package Import 缺少可用 package environment 时，诊断明确要求运行 `locus-pkg install`。
- `version`、`--version` 及其 JSON 输出无需有效 Scope，并返回构建时注入的版本。
