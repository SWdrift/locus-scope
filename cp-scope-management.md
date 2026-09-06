# Scope Management Control Plane

## Control Plane Role

本控制平面管理 `locus-scope` 与 `locus-scope-node` 的 Workspace 查询、图分析、对象写入、provenance、语义 diff 和验证能力大版本迭代。两套 CLI 必须共享命令、JSON、stdout/stderr 与退出语义；差异仅限 Workspace 加载：standalone CLI 从本地/package 环境加载，Node CLI 由 Node adapter 组装 package environment 后调用 Go host。

本平面独立于 [`cp-npm-packages.md`](cp-npm-packages.md)：后者管理 npm 分发与 Package 生命周期，本平面管理加载完成后的 Scope Workspace 公共管理契约。涉及 Node bridge 或 Package Environment 时遵循两者；不得借本迭代重开 npm 分发设计。

## Meta Reference

- 执行相关任务前依次读取 [`cp-meta.md`](cp-meta.md) 和本文件。
- 随后读取 [`cpm-scope-management.md`](cpm-scope-management.md) 中已沉淀的实现决策与坑点；若其与当前权威设计或代码冲突，以后两者为准。
- 权威契约依次核对 `documents/design/protocol/PROTOCOL.md`、`documents/design/protocol/CORE_API.md`、`documents/design/protocol/CLI.md`、`documents/design/protocol/ERRORS.md`、`documents/design/Scope设计.md` 与 `documents/design/测试设计.md`；同一规则只在一份权威设计中完整定义。
- 涉及 npm Workspace 加载时再读取 [`cp-npm-packages.md`](cp-npm-packages.md) 与 [`cpm-npm-packages.md`](cpm-npm-packages.md)。
- 创建或扩展本平面时使用 `create-control-plane`；任务或阶段收尾时使用 `compact-control-plane`。

## Scope

- 控制对象：`scope`、`group`、`entity`、`relation` 统一查询，`graph`、`path`、`impact` 图操作，Entity/Relation mutation，`diff`、`validate`，以及这些能力共用的 application error 分类与 transport 映射。
- 当前公共入口：`cmd/locus-scope`、`cmd/locus-scope-node-host`、`internal/scopecli`、`internal/scopeapp`、`internal/scope`、`packaging/npm/locus-scope` Node adapter。
- 直接调用边界：standalone 的 `pkgapp.LoadWorkspace` 与 Node adapter 构造 host request 的加载路径；加载后必须进入同一 Go command/application surface。
- 最小新增内部 API：公共 Filter/Selector、完成 Scope/import/projection 后的 Graph Adapter、开放对象 mutation/provenance/atomic-write 服务，以及归一化语义子图 diff。
- 不在范围：查询 DSL、插件、图数据库、TUI/WebUI、新配置体系、Group mutation、Relation ID、Gonum 类型进入 Locus 模型或公共 JSON。

## Core Rules

### 公共契约

- `locus-scope` 与 `locus-scope-node` 的命令表、参数解析、帮助语义、稳定 JSON、错误 JSON、stdout/stderr 和退出码必须一致；只允许 Workspace 加载方式不同。现有 `list/show/resolve` 在 major version 中干净迁移到统一查询入口，不保留第二套长期命令约定。
- 统一查询遵循：无参数为 list、裸 ref 为 show、一个或多个 `field<op>value` 为 find；支持 `=`、`!=`、`^=`、`*=`、`~=`、嵌套字段和 AND 组合。
- Filter/Selector 是唯一对象筛选实现，复用于四类查询、图命令 `--via` 及其他对象筛选点；正则仅使用 Go `regexp`，不扩展为表达式语言。
- Entity 保持 `{id,...}`，以 `id` 定位；Relation 保持 `{from,type,to,...}`，以三元组定位。开放字段允许任意嵌套；结构定位字段不得由 `set` 修改。
- 所有新增命令提供确定、稳定排序的 JSON。成功结果写 stdout；错误写 stderr，`--json` 错误保持 `{"error":"..."}`；非 JSON 输出同样保持确定顺序。

### 错误处理

- 本迭代同时整理并落地 [`ERRORS.md`](documents/design/protocol/ERRORS.md) 的统一 application error：错误在最接近失败语义的层确定 `Code`、`Reason`、`Details` 和 cause，上层只补上下文，不匹配错误文本或重复分类。
- `scopeapp`、Workspace loader、Filter/Selector、Graph、mutation 和 diff 使用同一错误类型；CLI 与 Node host 只做 transport 映射。参数/命令输入错误退出 `2`，加载、验证、查询、权限、冲突和事务错误退出 `1`。
- CLI v1 公共错误 JSON 继续保持 `{"error":"..."}`；Node host protocol v2 可携带结构化内部错误，但不得改变两个 CLI 对外可观察的 JSON、stderr 和退出语义。
- 新增 mutation、query、graph 和 diff reason 时优先归入 `ERRORS.md` 既有 Code；只有消费层确实需要新处理策略时才新增 Code。错误 details 不得泄露凭据或工作区外敏感路径。

### 图与 diff

- Workspace 完成 Scope/import/projection 后才构图；Graph Adapter 隔离 Gonum directed multigraph，实现允许同节点对多种 Relation，公共模型和 JSON 不暴露 Gonum 类型。
- `graph` 默认 `depth=1`；`path` 返回一条确定的最短有向路径并保留沿途 Relation；`impact` 从 Entity seed 反向遍历；`--via` 只筛 Relation。
- `graph -` 与 `impact -` 从 stdin 接收 Entity seed；批量 Scope/Group seed 通过查询 JSON 与管道组合，不增加特殊 selector 语法。
- `diff` 两侧只接受 `scope:<ref>`、`group:<ref>`、`path:<file|dir>`，允许混合；先解析为归一化 Entity/Relation 子图，再按 Entity id、Relation 三元组比较开放字段并稳定排序，不引入通用文本/JSON diff runtime。

### 写入与安全

- mutation 输入统一支持多个 `field=value`、完整 root JSON 和 stdin `-`；value 优先解析为 JSON value，失败后作为字符串。positional 与 root JSON 同时给出定位字段时必须一致。
- `entity add` 与 `relation add` 默认写入当前可写 root Scope 的 `scope.locus.yaml`，不存在则创建；`--file` 仅选择新增声明目标，不改变 provenance 含义。
- `set/unset/remove` 必须按 provenance 修改原声明文件；dependency Scope 只读。`--source` 返回实际声明来源，不得与创建目标 `--file` 混淆。
- 更新流程固定为 `load → clone/change → validate → atomic write → reload`；任何失败不得产生悬空 Relation、修改只读 dependency 或留下半更新文件/内存状态。
- Group 只由声明自然形成，不提供 `group add/remove`。

### 实施与验收

- 先更新已确定的权威设计，再实现；先冻结共享 CLI/application 接口，再迁移 standalone 与 Node 调用方，禁止在两端复制 parser、help、JSON shaping 或业务语义。
- 测试覆盖可观察契约和失败事务边界；新增命令必须有稳定 JSON golden tests，并在 standalone 与 Node 两条加载路径验证同命令、同 JSON、同退出语义。
- mutation 测试的声明、fixture 和运行状态严格留在工作区；E2E 声明放 `test/e2e/case/`，运行现场放并保留于 `temp/e2e-run/`。
- 每阶段只处理本迭代新增或加重的高置信坏味道；不顺手重构 Package 分发或无关 Core semantics。

## Task Board

当前无待执行任务。
