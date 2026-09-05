# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [公共协议](protocol/README.md)：汇总用户和工具直接依赖的稳定契约。
  - [核心协议](protocol/PROTOCOL.md)：定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
  - [核心 API](protocol/CORE_API.md)：定义 Scope 与 Package 领域供 CLI、Node host 和未来 Web adapter 共用的进程内 application API。
  - [CLI](protocol/CLI.md)：定义 `locus-scope`、`locus-scope-node` 的 Workspace 检查和 `locus-pkg` 的 Package 管理指令、参数与输出约定。
- [Scope 设计](Scope设计.md)：以基本 Scope 装配和显式 Group 为例，定义 root 选择、递归 Definition document 发现、目录与嵌套 Scope 边界、Workspace 装配、检查和诊断。
- [Package设计](Package设计.md)：定义 npm-compatible Package、双 Package Environment、Registry、lock/store、pack/publish 和 Node bridge。
- [测试设计](测试设计.md)：登记测试分层、已覆盖场景、fixture、隔离、npm lifecycle E2E 和完成标准。
- [Windows安装包设计](Windows安装包设计.md)：定义只包含 standalone CLI 的 Windows 安装包、发布暂存、最终制品和用户安装目录。

## 阅读顺序

- 只使用协议：阅读[核心协议](protocol/PROTOCOL.md)。
- 使用或集成命令行：阅读 [CLI](protocol/CLI.md)。
- 开发消费层或应用服务：依次阅读[核心协议](protocol/PROTOCOL.md)和[核心 API](protocol/CORE_API.md)。
- 开发 Scope 装配：依次阅读[核心协议](protocol/PROTOCOL.md)和[Scope 设计](Scope设计.md)。
- 开发 Package 发布或安装能力：在 Scope 设计之后阅读[Package设计](Package设计.md)和 [CLI](protocol/CLI.md)。
- 设计或实现测试：在 Scope 设计和Package设计之后阅读[测试设计](测试设计.md)。
- 构建或维护 Windows 安装包：阅读[Windows安装包设计](Windows安装包设计.md)。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

