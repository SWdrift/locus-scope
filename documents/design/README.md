# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [核心协议](protocol/PROTOCOL.md)：权威定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
- [Scope 设计](Scope设计.md)：以基本 Scope 装配和显式 Group 为例，定义 root 选择、递归 Definition document 发现、目录与嵌套 Scope 边界、Workspace 装配、诊断和 CLI。
- [Package设计](Package设计.md)：定义 OCI Package、Source identity、发布与安装、lock、cache、项目物化和 source-aware loader。
- [测试设计](测试设计.md)：登记测试分层、已覆盖场景、fixture、隔离、OCI E2E 和完成标准。
- [Windows安装包设计](Windows安装包设计.md)：定义 Windows 安装包、发布暂存、最终制品和用户安装目录，以及 Zot 生命周期入口与卸载数据边界。

## 阅读顺序

- 只使用协议：阅读[核心协议](protocol/PROTOCOL.md)。
- 开发 Scope 装配：依次阅读[核心协议](protocol/PROTOCOL.md)和[Scope 设计](Scope设计.md)。
- 开发 Package 发布或安装能力：在 Scope 设计之后阅读[Package设计](Package设计.md)。
- 设计或实现测试：在 Scope 设计和Package设计之后阅读[测试设计](测试设计.md)。
- 构建或维护 Windows 安装包：阅读[Windows安装包设计](Windows安装包设计.md)。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

