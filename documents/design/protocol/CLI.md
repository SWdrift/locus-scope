# CLI

## 简述

locus-scope 提供一个公开 Workspace 管理入口：

- `locus-scope`：由包声明的 Node.js LTS runtime 运行，从 npm 或 pnpm 已安装依赖构造 resolved Environment，并在同一进程执行命令。

Package 的安装、更新、lockfile、缓存、打包和发布使用 npm 或 pnpm 原生命令，不属于 Locus CLI。

## 职责

本文是 CLI 指令、参数、输出和退出状态的公共契约。Scope 数据结构见[核心协议](PROTOCOL.md)；application error 的模型、Code、Reason 与传播规则见[错误契约](ERRORS.md)；Package 格式和 resolved Environment 见[Package 设计](../Package设计.md)。

## Workspace 管理

以下指令适用于 `locus-scope`。

| 指令 | 作用 |
| --- | --- |
| `scope [<scope-ref>] [<filter>...] [--source]` | 无参数时列出 Scope；提供 ref 时返回一个 Scope；提供 filter 时筛选 Scope。 |
| `group [<group-ref>] [<filter>...] [--source]` | 无参数时列出 Group；提供 ref 时返回一个 Group；提供 filter 时筛选 Group。 |
| `entity [<entity-ref>] [<filter>...] [--source]` | 无参数时列出 Entity；提供 ref 时返回一个 Entity；提供 filter 时筛选 Entity。 |
| `relation [<filter>...] [--source]` | 无参数时列出 Relation；提供 filter 时筛选 Relation。 |
| `graph [<entity-ref>] [--depth <n>] [--via <filter>...]` | 从 Entity 沿出边返回限定深度的子图；使用 `-` 代替 ref 时从 stdin 读取 seed。默认 depth 为 1，depth 0 只返回去重 seed。 |
| `path <from> <to> [--via <filter>...]` | 返回一条确定的最短有向路径及沿途 Relation；等长路径和平行边按稳定 Relation 顺序决胜。 |
| `impact <entity-ref> [--via <filter>...]` | 从 Entity 沿入边反向返回受影响对象和 Relation；使用 `-` 代替 ref 时从 stdin 读取 seed。 |
| `entity add [<id>] [<mutation-input>] [--file <path>]` | 在可写 root Scope 中新增 Entity；省略 `--file` 时写入 `scope.locus.yaml`。 |
| `entity set [<ref>] [<mutation-input>]` | 按 provenance 更新 Entity 的非定位字段。 |
| `entity unset <ref> <field>...` | 删除 Entity 的一个或多个非定位字段。 |
| `entity remove <ref>` | 删除 Entity；存在悬空 Relation 时失败且不写入文件。 |
| `relation add [<from> <type> <to>] [<mutation-input>] [--file <path>]` | 在可写 root Scope 中新增 Relation；省略 `--file` 时写入 `scope.locus.yaml`。 |
| `relation set <from> <type> <to> [<mutation-input>]` | 按 resolved 三元组和 provenance 更新 Relation 的非定位字段。 |
| `relation unset <from> <type> <to> <field>...` | 删除 Relation 的一个或多个非定位字段。 |
| `relation remove <from> <type> <to>` | 按 resolved 三元组删除 Relation。 |
| `diff <selector-a> <selector-b>` | 比较两个 `scope:`、`group:` 或 `path:` 语义子图，按 Entity 和 Relation 返回 added、removed、changed。 |
| `validate` | 加载并验证全部可达 Scope，返回 Workspace 统计。 |
| `version` | 输出 CLI 版本，不加载 Workspace。 |
| `help` | 输出指令帮助，不加载 Workspace。 |

除 `version` 和 `help` 外，命令先加载并验证 Workspace。`--scope` 显式选择 root Scope；省略时从当前目录向上发现最近的 Scope。runtime 从当前 npm 或 pnpm 安装环境解析 Package graph，并在同一进程调用 domain/application API。

### 引用与筛选

- `<filter>` 为 `field<op>value`，operator 是 `=`、`!=`、`^=`、`*=`、`~=`；多个 filter 使用 AND。value 优先解析为 JSON value，失败后作为 string。
- 点路径只遍历 JSON object，不索引 array。`=`、`!=` 使用严格 JSON 深比较；其他 operator 只用于 string。字段不存在时任何 predicate 都不匹配。
- `@scope`、`@group`、`@fromScope`、`@toScope` 查询 envelope metadata；不带 `@` 的路径只查询开放对象。
- `scope-ref := "." | alias (":" alias)*`；`.` 是 root，alias chain 沿 root imports 解析。
- `group-ref := scope-ref "#" group-path`。Entity ref 使用既有 Projection 与 Group 规则。
- Graph 命令的 `--via` 复用 Relation filter。`graph -` 和 `impact -` 从 stdin 接收单个 Entity query envelope 或 envelope 数组；跨 Scope seed 使用 `key:{scope,id}`。

### 写入规则

- `<mutation-input>` 是多个 `field=value`、一个完整 root JSON object 或 stdin `-`。field assignment 支持 object 点路径；value 优先解析为 JSON value，失败后作为 string。
- Entity 是 `{id,...}`，Relation 是 `{from,type,to,...}`。positional 与 JSON 同时提供定位字段时必须一致；`set` 不能修改定位字段。
- JSON `set` 对非定位字段递归 merge，`null` 是普通值；删除字段只使用 `unset`。路径穿越非 object 时失败。
- grouped Entity 对外使用规范 id，写回时转换为 declaration-local id。
- `--file` 基于 root Scope，只允许位于 root 内、非嵌套 Scope 的 `*.locus.yaml`。dependency Scope 只读。
- `set`、`unset` 和 `remove` 根据 provenance 修改原声明文件。更新执行 `load → clone/change → validate → atomic write → reload`，失败不得留下文件或 Workspace 半更新。

### 通用参数

| 参数 | 作用 |
| --- | --- |
| `--scope <dir>` | 指定 root Scope 目录；省略时从当前目录沿祖先目录发现。 |
| `--json` | 输出稳定 JSON；失败格式见[输出与退出状态](#输出与退出状态)。 |
| `--source` | 在查询对象 envelope 中增加实际声明 provenance。 |
| `--version` | 等价于 `version`，不加载 Workspace。 |
| `--help`、`-h` | 等价于 `help`，不加载 Workspace。 |

Entity query envelope 固定包含 `key` 和开放 `object`；仅当 root 可解析时包含 `ref`。Relation envelope 固定包含 `fromKey`、`toKey` 和声明 ref 组成的开放 `object`。`--source` 增加 Scope identity、manifest id、Scope-relative file 和当前快照可用的 line/index；line/index 不是 identity。

`diff` 的 `scope:`、`group:` 从当前 Workspace 选择语义子图；`path:` 接受 Scope 目录或单个 definition document。单文件使用 file URI 作为 synthetic Scope，只解析文件内 Entity/Relation，外部 Relation ref 失败。两侧归一化后按 Entity key 和 resolved Relation 三元组比较；changed 使用 `{before,after}`。


## 输出与退出状态

错误的模型和分类由[错误契约](ERRORS.md#错误模型)定义；CLI 根据错误 Code 确定公开输出和退出状态。

| 执行结果 | stdout | stderr | 退出状态 |
| --- | --- | --- | --- |
| 成功 | 命令的成功结果 | 空 | `0` |
| Code 为 `invalid_argument` | 空 | 错误信息 | `2` |
| 其他错误 Code | 空 | 错误信息 | `1` |

- 未知指令、未知参数、缺少参数、不允许的参数组合和无效 filter 归入 `invalid_argument`。
- `--json` 同时约束成功与失败输出；失败至少包含 `error`、`code` 和 `reason`，安全的结构化上下文可通过 `details` 提供；成功 schema 由对应命令定义。
- runtime 直接完成 Environment、Workspace 和 application 执行；不存在独立 host protocol 或平台 host package。
- 公开输出不包含 cause、stack trace、凭据或工作区外敏感路径。
