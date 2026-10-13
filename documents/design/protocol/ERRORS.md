# 错误契约

## 简述

Scope application API 使用统一、可分类并保留 cause 的 application Error。CLI、library consumer 和未来其他 adapter 只负责 transport 映射，不解析错误文本判断行为。

## 职责

本文定义 application error 的稳定字段、分类和传播规则。领域有效性由[核心协议](PROTOCOL.md)、[Scope 设计](../Scope设计.md)和 [Package 设计](../Package设计.md)定义；CLI 如何向用户输出错误以及选择退出状态，由 [CLI](CLI.md#输出与退出状态)定义。

## 错误模型

```ts
class ApplicationError extends Error {
    readonly code: ErrorCode
    readonly reason: string
    readonly message: string
    readonly details?: Record<string, string>
    readonly cause?: unknown
}
```

- `Code` 是少量跨能力分类，供消费层决定处理方式。
- `Reason` 是 `scope.<reason>` 格式的稳定领域标识。
    - 使用 `scope.*` 命名空间；
    - 每个 Reason 必须归属一个 Code；
    - 已发布 Reason 不得改变或复用其语义。
- `Message` 面向人类，可以改善措辞；程序不得解析它。
- `Details` 保存该 reason 所需的结构化上下文。
- `Cause` 使用标准 JavaScript `Error.cause` 保存诊断信息，不进入外部协议。

## Code

| Code                  | 含义                                                                  |
| --------------------- | --------------------------------------------------------------------- |
| `invalid_argument`    | 调用参数缺失、格式错误或组合无效。                                    |
| `invalid_data`        | Manifest、definition 或 resolved Environment 不符合协议。             |
| `not_found`           | 请求的资源不存在。                                                    |
| `conflict`            | identity 或 mutation 状态冲突。                                       |
| `permission_denied`   | 身份有效但没有操作权限。                                              |
| `internal`            | 无法归入以上类别的实现错误。                                          |

未知 Code 按 `internal` 处理。已有 Code 的含义不得改变；只有消费层确实需要不同处理策略时才新增 Code。

## Reason

`Reason` 是稳定的、可供程序消费的错误标识。每个 Reason 必须有唯一 Code 和明确语义；具体 message 可以改善措辞，但不得改变 Reason 的含义。

### 调用输入：`invalid_argument`

| Reason | 说明 |
| --- | --- |
| `scope.selector_invalid` | 命令 selector、位置参数、选项或参数组合格式错误。 |
| `scope.filter_invalid` | Filter 语法、路径、操作符或值格式错误。 |
| `scope.reference_unresolved` | 调用提供的 Entity/Scope/Relation reference 无法解析。 |
| `scope.graph_seed_invalid` | Graph/Impact 的 seed 或 stdin Entity envelope 格式错误。 |
| `scope.field_invalid` | Mutation field 名称、路径或赋值格式错误。 |
| `scope.diff_input_invalid` | Diff 两侧输入数量、selector 或 selector 组合格式错误。 |
| `scope.mutation_input_invalid` | Mutation command input、JSON payload 或位置参数组合格式错误。 |

### Source 与 package 加载：`not_found`、`permission_denied`、`invalid_data`

| Reason | Code | 说明 |
| --- | --- | --- |
| `scope.source_not_found` | `not_found` | 请求的 Scope source、目录或文件不存在。 |
| `scope.source_unreadable` | `permission_denied` | Scope source 存在，但当前进程无权读取。 |
| `scope.source_invalid` | `invalid_data` | Source 路径存在，但不是可加载的 Scope source。 |
| `scope.manifest_missing` | `invalid_data` | Scope 目录中没有可用的 `locus.yaml`、`locus.yml` 或 `locus.json` manifest。 |
| `scope.manifest_ambiguous` | `invalid_data` | Scope 目录中同时存在多个 manifest，无法确定唯一入口。 |
| `scope.manifest_invalid` | `invalid_data` | Manifest 存在，但字段、类型或值不符合协议。 |
| `scope.package_invalid` | `invalid_data` | npm package manifest、`locus.entry` 或 package 元数据不符合包协议。 |
| `scope.dependency_unresolved` | `invalid_data` | 声明为 Locus package 的依赖无法解析或加载。 |

### Workspace 数据：`invalid_data`、`conflict`

| Reason | Code | 说明 |
| --- | --- | --- |
| `scope.definition_invalid` | `invalid_data` | Entity/Relation definition 文件无法解析，或结构不符合 definition 协议。 |
| `scope.entity_id_invalid` | `invalid_data` | Entity 的 id 缺失、类型错误或包含禁止的内容。 |
| `scope.import_unresolved` | `invalid_data` | Scope import 无法解析到目标 Scope。 |
| `scope.export_unresolved` | `invalid_data` | Scope export 引用无法解析到可导出的 Entity。 |
| `scope.relation_invalid` | `invalid_data` | Relation 缺少必要字段，或字段类型/值不符合协议。 |
| `scope.relation_start_unresolved` | `invalid_data` | Relation 的起点引用无法解析。 |
| `scope.relation_end_unresolved` | `invalid_data` | Relation 的终点引用无法解析。 |
| `scope.entity_duplicate` | `conflict` | 同一 Scope 中出现重复的 Entity identity。 |
| `scope.relation_duplicate` | `conflict` | Workspace 中出现重复的 Relation identity。 |
| `scope.source_identity_conflict` | `conflict` | 同一个 Scope identity 指向了互相冲突的 source。 |

### 查询结果：`not_found`、`permission_denied`

| Reason | Code | 说明 |
| --- | --- | --- |
| `scope.object_not_found` | `not_found` | 请求的 Scope、Group、Entity 或 Relation 不存在。 |
| `scope.path_unreachable` | `not_found` | 起点和终点存在，但没有满足条件的有向路径。 |
| `scope.object_read_only` | `permission_denied` | 请求修改只读对象或只读 source。 |

### Mutation：`invalid_data`、`conflict`、`internal`

| Reason | Code | 说明 |
| --- | --- | --- |
| `scope.mutation_invalid` | `invalid_data` | Mutation 请求结构完整，但无法形成符合协议的数据。 |
| `scope.mutation_conflict` | `conflict` | Mutation 与当前 identity、provenance 或 Workspace 状态冲突。 |
| `scope.mutation_write_failed` | `internal` | Mutation 已进入写入流程，但 atomic write 或 reload 失败。 |

### 未分类故障：`internal`

| Reason | 说明 |
| --- | --- |
| `scope.internal` | 错误无法归入其他已声明 Reason，通常表示实现错误或未分类故障。 |

Reason 的新增、删除或语义改变属于协议变更；仅改善 message 不属于 Reason 变更。

## 公开诊断

- `message` 面向人类；程序不得解析 message 判断错误类型。
- `details` 只包含稳定、必要且非敏感的结构化上下文，例如相对文件名、Scope ref、Entity ref、字段名和行号。
- `cause` 和 stack trace 只用于内部诊断，不进入 CLI、JSON 输出或其他外部协议。
- 公开输出不得包含工作区绝对路径、用户目录、凭据、环境变量或其他敏感路径。
- Scope 内文件使用相对于 Scope root 的路径；无法安全归一化的路径不得公开。
- 同一个 Reason 的 `details` 字段语义必须保持稳定；缺失字段表示该上下文不可用，不得用空字符串伪造。

## 传播规则

- 最接近错误语义的层负责确定 Code 和 Reason。
- 上层保留 Code、Reason 和 cause，只增加必要上下文。
- 未分类错误在 application API 边界转换为 `internal`。
- 包装不得重复添加同一 operation、resource 或 location。
- Workspace load 和 mutation 保持 fail-fast；失败不返回可消费的部分结果。
- Mutation 首次 reload 失败后会尝试恢复原文件或删除新文件，并执行二次 reload。任一 rollback cleanup 或二次 reload 失败时，`scope.mutation_invalid` 的 `cause` 为 `AggregateError`，包含首次 reload 错误和全部 rollback 错误；不得将其视为已确认恢复。



