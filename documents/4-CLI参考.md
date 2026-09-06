# CLI 参考

Locus Scope 提供三个命令行入口：

- `locus-scope`：从本地文件、`locus.lock` 和 `.locus/` 加载 Workspace。
- `locus-scope-node`：从 npm 或 pnpm 已安装的依赖加载 Workspace。
- `locus-pkg`：安装、更新、打包和发布 npm-compatible Locus Package。

`locus-scope` 与 `locus-scope-node` 支持相同的 Workspace 命令，只有 Package 加载环境不同。

## 命令入口

直接使用独立 Workspace CLI：

```text
locus-scope [--scope <dir>] [--json] <command>
```

在 pnpm 项目中使用 Node adapter：

```text
pnpm exec locus-scope-node [--scope <dir>] [--json] <command>
```

npm 项目将前缀替换为 `npx locus-scope-node`。Package 命令统一使用：

```text
locus-pkg [options] <command>
```

除 `help` 和 `version` 外，Workspace 命令执行前会加载并验证所有可达 Scope。省略 `--scope` 时，CLI 从当前目录向上查找最近的 Scope。Package 命令从当前目录向上查找适用的项目或 Package 根目录。

## Workspace 参数

以下参数同时适用于 `locus-scope` 和 `locus-scope-node`：

| 参数 | 说明 |
| --- | --- |
| `--scope <dir>` 或 `--scope=<dir>` | 使用指定目录作为 root Scope；否则从当前目录发现最近的 Scope。 |
| `--json` | 输出稳定的单行 JSON；错误格式为 `{"error":"<message>"}`。 |
| `--source` | 在查询结果中增加声明来源。 |
| `--version` | 不加载 Workspace，直接输出 CLI 版本。 |
| `--help`、`-h` | 不加载 Workspace，直接输出命令帮助。 |

除非命令语法另有说明，参数可以放在命令前后。

## 引用与筛选

### Scope、Group 与 Entity 引用

| 引用 | 含义 |
| --- | --- |
| `.` | root Scope。 |
| `infra` | 别名为 `infra` 的 imported Scope。 |
| `platform:infra` | 沿 alias chain 解析的嵌套 Import。 |
| `.#services` | root Scope 中的 `services` Group。 |
| `infra#storage` | imported `infra` Scope 中的 `storage` Group。 |
| `database` | 当前 Scope 上下文中的 Entity。 |
| `infra:database` | 通过 `infra` Import 访问的 exported Entity。 |

### Filter

Filter 格式为 `field<operator>value`，多个 filter 使用 AND 语义。

| Operator | 含义 |
| --- | --- |
| `=` | 严格 JSON 相等。 |
| `!=` | 严格 JSON 不等。 |
| `^=` | 字符串以指定值开头。 |
| `*=` | 字符串包含指定值。 |
| `~=` | 使用正则表达式匹配字符串。 |

value 会优先解析为 JSON，失败后作为字符串。例如，`port=5432` 比较数字，`type=postgres` 比较字符串。点路径可遍历 JSON object，例如 `metadata.owner=platform`，但不会索引 array。字段不存在时不匹配任何 predicate。

以下保留字段用于查询 envelope metadata：

- `@scope`：所属 Scope。
- `@group`：声明所在的 Group。
- `@fromScope`：Relation resolved source 所在的 Scope。
- `@toScope`：Relation resolved target 所在的 Scope。

## Workspace 查询命令

### `scope`

列出 Scope、读取一个 Scope 或筛选 Scope：

```text
locus-scope scope
locus-scope scope .
locus-scope scope id=infra
locus-scope scope @scope^=file:
```

语法：

```text
scope [<scope-ref>] [<filter>...] [--source]
```

无参数时返回所有可达 Scope。单个非 filter 参数会作为 Scope reference。

### `group`

列出 Group、读取一个 Group 或筛选 Group：

```text
locus-scope group
locus-scope group .#services
locus-scope group type=layer
```

语法：

```text
group [<group-ref>] [<filter>...] [--source]
```

无参数时返回所有可达 Group。单个非 filter 参数会作为 Group reference。

### `entity`

列出 Entity、读取一个 Entity 或筛选 Entity：

```text
locus-scope entity
locus-scope entity database
locus-scope entity infra:database
locus-scope entity type=postgres port=5432
```

语法：

```text
entity [<entity-ref>] [<filter>...] [--source]
```

无参数时返回所有可达 Entity。单个非 filter 参数会作为 Entity reference。

### `relation`

列出或筛选 Relation：

```text
locus-scope relation
locus-scope relation type=uses
locus-scope relation @fromScope=. type=uses
```

语法：

```text
relation [<filter>...] [--source]
```

Relation 通过 filter 筛选，不使用单个 reference 定位。

## 关系图命令

### `graph`

从一个或多个 seed Entity 沿出边遍历 Relation：

```text
locus-scope graph backend
locus-scope graph backend --depth 2
locus-scope graph backend --depth 2 --via type=uses
locus-scope entity type=service --json | locus-scope graph - --depth 1
```

语法：

```text
graph [<entity-ref>|-] [--depth <n>] [--via <filter>...]
```

- 默认 depth 为 `1`。
- depth `0` 只返回去重后的 seed。
- 省略 seed 时，以 root Scope 拥有的所有 Entity 为 seed。
- `-` 从 stdin 读取一个 Entity query envelope 或 envelope 数组。
- `--via` 使用 Relation filter 限制允许遍历的 Relation。

### `path`

查询两个 Entity 之间一条确定的最短有向路径：

```text
locus-scope path frontend database
locus-scope path frontend infra:database --via type=uses
```

语法：

```text
path <from> <to> [--via <filter>...]
```

存在多条等长最短路径或平行 Relation 时，使用稳定的 Relation 顺序确定结果。目标不可达时命令失败。

### `impact`

沿入边反向遍历 Relation，查找受影响的 Entity：

```text
locus-scope impact database
locus-scope impact database --via type=uses
locus-scope entity type=postgres --json | locus-scope impact -
```

语法：

```text
impact <entity-ref|-> [--via <filter>...]
```

`-` 从 stdin 读取一个 Entity query envelope 或 envelope 数组。`--via` 使用 Relation filter 限制允许遍历的 Relation。

## Entity 写入命令

Mutation input 可以是多个 `field=value`、一个完整 JSON object，或使用 `-` 从 stdin 读取的 JSON。field assignment 的 value 优先解析为 JSON。点路径用于更新嵌套 object。

### `entity add`

在可写 root Scope 中新增 Entity：

```text
locus-scope entity add backend type=service replicas=2
locus-scope entity add '{"id":"backend","type":"service"}'
locus-scope entity add backend type=service --file model/services.locus.yaml
```

语法：

```text
entity add [<id>] [<mutation-input>] [--file <path>]
```

Entity 必须包含 `id`。省略 `--file` 时写入 `scope.locus.yaml`。目标必须是 root Scope 内且不属于 nested Scope 的 `*.locus.yaml` 文件。

### `entity set`

在 Entity 的原声明文件中更新非 identity 字段：

```text
locus-scope entity set backend replicas=3 metadata.owner=platform
locus-scope entity set backend '{"id":"backend","type":"service","replicas":3}'
```

语法：

```text
entity set [<ref>] [<mutation-input>]
```

省略 positional reference 时，JSON object 可以提供 `id`。两者同时存在时必须一致。object 字段递归 merge；`null` 会被保存为普通值，不会删除字段。

### `entity unset`

删除一个或多个非 identity 字段：

```text
locus-scope entity unset backend replicas metadata.owner
```

语法：

```text
entity unset <ref> <field>...
```

删除字段应使用 `unset`，而不是 `set field=null`。不能删除 `id`。

### `entity remove`

从原声明文件删除 Entity：

```text
locus-scope entity remove backend
```

语法：

```text
entity remove <ref>
```

如果删除 Entity 会留下悬空 Relation，命令会失败且不会写入文件。

## Relation 写入命令

Relation 通过 resolved `from`、`type`、`to` 三元组定位。Mutation input 与 Entity 写入使用相同的 field assignment、JSON object 和 stdin 规则。

### `relation add`

在可写 root Scope 中新增 Relation：

```text
locus-scope relation add backend uses database weight=1
locus-scope relation add '{"from":"backend","type":"uses","to":"database"}'
locus-scope relation add backend uses database --file model/relations.locus.yaml
```

语法：

```text
relation add [<from> <type> <to>] [<mutation-input>] [--file <path>]
```

三个定位字段可以使用 positional arguments，也可以由完整 JSON object 提供。省略 `--file` 时写入 `scope.locus.yaml`。

### `relation set`

在 Relation 的原声明文件中更新非定位字段：

```text
locus-scope relation set backend uses database weight=2
locus-scope relation set backend uses database '{"from":"backend","type":"uses","to":"database","weight":2}'
```

语法：

```text
relation set <from> <type> <to> [<mutation-input>]
```

不能修改 `from`、`type` 和 `to`。需要修改 Relation identity 时，应删除后重新添加。

### `relation unset`

删除一个或多个非定位字段：

```text
locus-scope relation unset backend uses database weight metadata.note
```

语法：

```text
relation unset <from> <type> <to> <field>...
```

不能删除定位字段 `from`、`type` 和 `to`。

### `relation remove`

按 resolved 三元组删除 Relation：

```text
locus-scope relation remove backend uses database
```

语法：

```text
relation remove <from> <type> <to>
```

所有写入命令都会先验证变更后的 Workspace，再以 atomic write 修改声明。dependency Scope 只读。写入失败时，文件和已加载 Workspace 都保持不变。

## 比较与验证命令

### `diff`

比较两个语义子图：

```text
locus-scope diff scope:. scope:infra
locus-scope diff group:.#services group:infra#services
locus-scope diff path:./before path:./after
locus-scope diff path:./before.locus.yaml path:./after.locus.yaml
```

语法：

```text
diff <selector-a> <selector-b>
```

Selector 包括：

- `scope:<scope-ref>`：所选 Scope 拥有的全部 Entity，以及两端都在选择范围内的 Relation。
- `group:<scope-ref>#<group-path>`：所选 Group 的 Entity，以及两端都在选择范围内的 Relation。
- `path:<path>`：作为比较输入加载的 Scope 目录或单个 Definition document。

结果在 `added`、`removed` 和 `changed` 下分别返回 Entity 和 Relation。changed item 包含 `before` 和 `after`。

### `validate`

加载并验证所有可达 Scope：

```text
locus-scope validate
locus-scope --scope ./app --json validate
```

语法：

```text
validate
```

结果包含 `valid`、root Scope URI，以及 Scope、Entity、Relation 的数量。

### `version`

不加载 Workspace，直接输出 CLI 版本：

```text
locus-scope version
locus-scope --json version
locus-scope-node version
```

### `help`

不加载 Workspace，直接输出命令用法：

```text
locus-scope help
locus-scope --help
locus-scope -h
```

## Package 参数

`locus-pkg` 参数可以放在 subcommand 前后：

| 参数 | 说明 |
| --- | --- |
| `--registry <url>` 或 `--registry=<url>` | 覆盖 npm Registry；否则使用环境和 `.npmrc` 配置解析结果。 |
| `--offline` | 禁止 Registry 请求，只使用已有 lock、缓存和展开状态。 |
| `--frozen-lockfile` | 要求 `package.json` 与 `locus.lock` 完全一致，且不修改声明或 lock。 |
| `--json` | 输出稳定 JSON；错误格式为 `{"error":"<message>"}`。 |
| `--version` | 输出 CLI 版本。 |
| `--help`、`-h` | 输出命令帮助。 |

`publish` 不接受 `--offline` 或 `--frozen-lockfile`。显式 `install <package-spec>` 不能与 `--frozen-lockfile` 一起使用。

## Package 命令

### `install`

按照 `package.json.dependencies` 安装完整依赖图：

```text
locus-pkg install
locus-pkg install --frozen-lockfile
locus-pkg install --offline --frozen-lockfile
```

添加或修改直接依赖，然后安装完整依赖图：

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg install @example/infra@^1.0.0 @example/model@2.0.0
```

语法：

```text
install [<package-spec>...]
```

命令会提交一致的 `package.json`、`locus.lock` 和 `.locus/` 状态。显式 Package spec 会更新直接依赖。

### `uninstall`

删除一个或多个直接依赖，并剪除不再可达的 Package：

```text
locus-pkg uninstall @example/infra
locus-pkg uninstall @example/infra @example/model
```

语法：

```text
uninstall <package>...
```

只接受 direct dependency name。

### `update`

在已声明的版本约束内更新直接依赖：

```text
locus-pkg update
locus-pkg update @example/infra
```

语法：

```text
update [<package>...]
```

省略 Package name 时更新所有直接依赖。指定名称时，只更新所选直接依赖及其 dependency closure。

### `list`

显示由 `locus.lock` 和 `.locus/` 解析的依赖树：

```text
locus-pkg list
locus-pkg --json list
```

语法：

```text
list
```

每个 dependency 包含 name、version、identity 和嵌套 dependencies。

### `pack`

验证当前 Package，并创建确定性的 npm `.tgz`：

```text
locus-pkg pack
locus-pkg --json pack
```

语法：

```text
pack
```

命令向上查找最近的、包含有效 `locus.entry` 的 `package.json`。文本输出包含 archive filename、Package name、version、integrity 和文件数量。

### `publish`

打包并发布当前不可变的 `name@version`，同时发布 `latest` dist-tag：

```text
locus-pkg publish
locus-pkg publish --registry https://registry.example.com/
```

语法：

```text
publish
```

Registry 认证使用 npm-compatible `.npmrc` 和环境配置。文本输出包含 Package name、version、Registry 和 integrity。

### `version`

不查找项目或读取 Registry 配置，直接输出 Package CLI 版本：

```text
locus-pkg version
locus-pkg --json version
```

### `help`

不查找项目，直接输出 Package 命令用法：

```text
locus-pkg help
locus-pkg --help
locus-pkg -h
```

## 输出与退出状态

| 结果 | stdout | stderr | 退出状态 |
| --- | --- | --- | --- |
| 成功 | 命令结果 | 空 | `0` |
| 无效命令或参数 | 空 | 错误信息 | `2` |
| 其他错误 | 空 | 错误信息 | `1` |

未知命令、未知参数、缺少参数、不支持的参数组合和无效 filter 都属于参数错误。使用 `--json` 时，成功和失败都使用稳定 JSON；失败格式为 `{"error":"<message>"}`。

