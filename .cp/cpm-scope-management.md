# Scope Management Memory

## 稳定决策

- `locus-scope` 是唯一用户级 CLI；TypeScript command runner 位于 `src/cli`，业务语义位于 `src/application`。
- Node adapter 只构造 resolved Environment；不复制 Scope 语义或命令实现。
- Entity 以 `(scope,id)` 定位；Relation 以 resolved `(from,type,to)` 定位。
- Filter 是查询和 graph `--via` 的共享筛选实现。
- provenance 在 decode/load 形成；dependency Scope mutation 必须拒绝。
- graph/path/impact 基于 Workspace Relation 建立 outgoing/incoming adjacency，并使用确定性 BFS。
- `diff` 比较归一化 Entity/Relation 子图；mutation 使用原子写与 reload 验证。
- CLI 参数错误退出 `2`，加载、验证、查询、权限、冲突和事务错误退出 `1`。

## 已解决的基线缺口

- mutation rollback 在恢复原文件、删除新文件或二次 reload 失败时，统一保留原始 reload 错误并通过 `AggregateError` 暴露全部 rollback 错误；不得静默丢弃。
