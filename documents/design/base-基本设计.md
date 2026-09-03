# 基本设计

## 简述

v1 将本地 Scope 文件完整装配为可验证、可查询的内存 Workspace。协议语义以 [PROTOCOL.md](protocol/PROTOCOL.md) 为唯一权威来源；本文只定义本地实现边界和工具行为。

## 职责

本文负责本地 Source、文件发现、Workspace 装配、诊断和 CLI 契约。不负责重新定义协议，也不负责远程分发、持久化、编辑或执行。

代码目录与依赖方向只在 [当前架构](../current-architecture.md) 中维护，本文不重复目录树。

## 本地 Source 与文件发现

一个 Scope 对应一个目录。目录中必须且只能存在 `locus.yaml`、`locus.yml`、`locus.json` 之一；同目录下其他小写 `.yaml`、`.yml`、`.json` 文件是 definition documents。发现不递归，definition documents 按文件名排序读取。

显式 `--scope <dir>` 直接选择目录；未指定时从当前目录向父目录寻找最近的 manifest。Import value 是相对声明 Scope 目录或绝对目录的本地路径。运行时使用消除符号链接后的规范化绝对目录作为 `ScopeKey`，不使用 `Manifest.ID` 或 import alias 代替来源 identity。

YAML 和 JSON 共用同一严格文档结构解码路径。Entity 的 `id` 单独保存，其余字段原样进入不透明属性 map。Group 在读取单个 definition document 时展开到 Entity ID 和该文件内局部 Relation 引用，运行时不保留 Group 对象。

## Workspace 装配与解析

`scope.Load` 使用队列发现 reachable Scope：Scope 自身解码完成后立即注册，再规范化并入队其 imports，因此循环 Import 不需要递归、拓扑排序或特殊豁免。

`Workspace.Resolve(from, ref)` 按第一个 `:` 逐级消费 Projection。每跨越一次 Projection，目标 Scope 都必须显式 export 剩余 reference；最终结果始终是原始 owner 的 `EntityKey{Scope, ID}`。Import 和再次 Export 不复制 Entity。所有 Scope 加载后先验证 exports，再以同一 resolver 解析 Relation 两端并形成稳定 identity 的边。

集合按 ScopeKey、Entity ID 和 Relation tuple 稳定排序后对外展示。错误保留 manifest 或 definition 文件路径、Relation 行号、声明内容、reference、Scope 和具体失败边界。

## CLI

`cmd/locus-scope` 是只调用 `scope` package 的薄入口，提供：

```text
locus-scope validate
locus-scope scope show
locus-scope scope list
locus-scope entity list
locus-scope entity show <ref>
locus-scope relation list
locus-scope resolve <ref>
```

所有命令接受 `--scope <dir>` 和 `--json`。文本输出用于人工检查；JSON 输出使用固定字段和稳定集合顺序，供 Agent 与脚本消费。`validate` 总是加载并验证完整 reachable graph，而不是只检查 root 文件。

## 验证口径

协议 examples 作为 acceptance fixture 直接加载。额外 fixture 覆盖多级再次 Export、私有 imported Entity、Group Relation、循环 Import、来源 ownership、展开后重复 ID、无效 Relation、重复逻辑 Scope ID 和上下文诊断。`go test ./...` 包含真实 CLI 子进程 smoke test。
