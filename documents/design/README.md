# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [核心协议](protocol/PROTOCOL.md)：权威定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
- [Scope 设计](Scope设计.md)：以本地 `app`/`infra` 装配为主例，定义 Source、Workspace、文件发现、解析、诊断、CLI 与非目标。
- [Package设计](Package设计.md)：定义 OCI Package、Source identity、发布与安装、lock、cache、项目物化和 source-aware loader。
- [测试设计](测试设计.md)：登记测试分层、已覆盖场景、fixture、隔离、OCI E2E 和完成标准。

## 阅读顺序

- 只使用协议：阅读[核心协议](protocol/PROTOCOL.md)。
- 开发 Scope 装配：依次阅读[核心协议](protocol/PROTOCOL.md)和[Scope 设计](Scope设计.md)。
- 开发 Package 发布或安装能力：在 Scope 设计之后阅读[Package设计](Package设计.md)。
- 设计或实现测试：在 Scope 设计和Package设计之后阅读[测试设计](测试设计.md)。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

