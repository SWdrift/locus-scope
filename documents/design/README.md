# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [核心协议](protocol/PROTOCOL.md)：权威定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
- [base-基本设计](base-基本设计.md)：定义第一版本地 Source、Workspace 装配、诊断、CLI 与非目标。
- [base-包管理设计](base-包管理设计.md)：定义 OCI Package、Source identity、lock、cache、安装流程与 source-aware loader。
- [base-测试设计](base-测试设计.md)：定义测试分层、fixture、隔离、OCI E2E 和完成标准。

## 阅读顺序

- 协议使用者：核心协议。
- 本地工具使用者和实现者：核心协议 → base-基本设计。
- Package 工具使用者和实现者：核心协议 → base-基本设计 → base-包管理设计。
- 测试实现者：对应功能设计 → base-测试设计。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

