# Scope Management Control Plane

## Control Plane Role

本平面管理统一 `locus-scope` CLI 的 Workspace 查询、图遍历、diff、mutation、验证与错误映射。

## Meta Reference

- 执行前读取 [`cp-meta.md`](cp-meta.md) 与 [`cpm-scope-management.md`](cpm-scope-management.md)。
- 权威协议见 [`documents/design/protocol/`](documents/design/protocol/README.md)。
- 阶段收尾使用 `compact-control-plane`。

## Scope

- 入口：`src/domain`、`src/application`、`src/cli`、`src/package-environment.ts`、`src/packageenv.ts`。
- Workspace 是稳定事实图接口；上层分析、规划和执行不进入 domain core。
- CLI 仅做 parsing、dispatch、format 与 exit mapping；业务逻辑保留在 `src/application`。

## Core Rules

- Source → Decode → Load → Compose → Validate → Workspace 数据流保持独立。
- query/filter、graph/path/impact、diff、mutation 保持稳定 JSON 与确定排序。
- graph/path/impact 使用 outgoing/incoming adjacency 与 BFS，不引入图框架抽象。
- dependency Scope 只读；mutation 遵循 load → clone/change → validate → atomic write → reload。
- 无效命令/参数退出 `2`；运行和语义错误退出 `1`。

## Task Board

- [ ] 为中大型 Workspace 建立 graph/path/impact/diff/query 基准，重点检查 JavaScript queue 和开放属性深拷贝成本；未有基准前不宣称大规模生产性能。

## Confidence Matrix

| 维度 | 当前把握 | 依据和限制 |
| --- | ---: | --- |
| 3.1 已登记语义的功能 parity | 10/10 | 33 个 parity/unit tests、完整 fixture domain 测试、完整 npm lifecycle E2E、本地 tarball consumer CLI/library 验证通过；仅保留 3.1 自身 rollback cleanup 缺口。 |
| 小规模 Node CLI / library 使用 | 10/10 | 本地 tarball 安装、validate、query、graph、path、diff、mutation、CLI/library parity 与 E2E 均通过。 |
| 中等规模 Workspace | 6/10 | 语义覆盖较完整，但缺少代表性 Workspace 的性能与内存基准。 |
| 大规模 graph/diff/query | 4–5/10 | 尚无规模基准；queue、开放属性 clone 和完整 semantic diff 的成本未量化。 |
| 并发 mutation / 多进程写入 | 5/10 | atomic write 和 rollback 已覆盖；业务级并发冲突、锁和版本检测未覆盖。 |
| 跨平台生产稳定性 | 5/10 | 当前主要在 Windows 验证，Linux/macOS filesystem、symlink 和权限矩阵未完成。 |
| 大规模生产承诺 | 5/10 | 在规模基准、并发测试和跨平台矩阵完成前禁止作此承诺。 |
