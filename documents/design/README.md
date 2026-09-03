# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [核心协议](protocol/PROTOCOL.md)：权威定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
- [v1 实现设计](implementation-v1.md)：定义第一版本地 Source、Workspace 装配、诊断、CLI 与非目标。

## 阅读顺序

- 协议使用者：核心协议。
- 工具使用者和实现者：核心协议 → v1 实现设计。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

