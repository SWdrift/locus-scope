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

当前无待执行任务。
