# 测试设计

## 简述

测试证明 Core semantics、Package 安装和 CLI 的可观察行为和易错点和复杂逻辑，不按源码函数逐一覆盖。

## 职责

本文统一定义测试分层、fixture、隔离和完成标准。

## 分层

测试层级由行为边界、运行成本和失败定位决定，不按模块维护封闭的用例清单。新增行为选择能完整观察其契约的最低层级；下列范围是指导，不限制未来测试。

### Core

`internal/scope/*_test.go` 适合无需网络和持久化基础设施即可观察的协议、Workspace 与诊断行为。优先从公开行为验证多个内部步骤，不逐个测试 decode 或排序 helper。

### Package

`internal/packages/*_test.go` 适合 lock、artifact、cache、物化和 Source resolution 等包管理行为。可以在边界注入工作区路径和测试 Registry，但不得用 mock 结果代替被测状态转换。

### E2E

`test/e2e/` 验证跨进程、网络协议或持久化边界的关键闭环。场景数量和文件名不固定，但应保持最少，并使用真实 CLI、ORAS client 和 loopback OCI Distribution API。

## 数据与隔离

- 可复用 Scope source tree 存放在 `test/e2e/case/`。
- 每个场景确定性写入 `temp/e2e-run/<case>/` 并保留现场。
- registry、cache、project、binary 和结果均位于该场景目录。
- 测试不读取用户凭据、配置、cache 或外部网络状态。
- 归档错误场景在测试中构造，不保存大量二进制 fixture。

## CLI 验证

二进制构建到 `temp/e2e-run/<case>/bin/` 后作为独立子进程执行。断言应对应场景的可观察契约；优先解析 JSON，文本模式保留必要 smoke。

## 完成标准

```text
go test ./...
pnpm --dir .tools/markdown run check:links
```

当前 Package 闭环基线包括首次 tag 解析、lock 复用、`--frozen` 和安装后跨 Package Entity 解析；后续新增行为按相同原则补充测试。
