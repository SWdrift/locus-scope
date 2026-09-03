# locus-scope 设计文档

本目录维护已经确定、需要跨任务复用的设计。用户直接依赖的接口集中在公共契约中；系统内部语义、数据流和测试分别由对应设计维护。

## 文档入口

- [核心协议](核心协议.md)：权威定义 Entity、Scope、Export、Import、Projection、Relation、Definition document 和 Group。
- [当前实现快照](../current-architecture.md)：记录现有目录、模块、示例、验证基线和明确未实现项。

## 阅读顺序

- 协议使用者：核心协议。
- 实现者：核心协议 → 当前实现快照。

## 管理规则

- 项目、产品与协议名称统一写作 `locus-scope`。
- 同一规则只在一份权威文档中完整定义，其他文档使用链接。

