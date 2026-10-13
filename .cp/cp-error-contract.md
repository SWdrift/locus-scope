# Error Contract Control Plane

## Control Plane Role

本平面追踪 Locus Scope 的错误 Code、Reason、公开诊断和 CLI 映射，防止出现未声明的错误类型、未被代码引用的假 Reason、同一 Reason 多个 Code、裸错误绕过分类，以及公开输出泄露敏感路径。

本平面与 `cp-scope-management.md` 共同约束 CLI/application/domain；与 `cp-npm-packages.md` 的关系仅限 package/environment 加载错误。

## Meta Reference

- 执行前读取 [`cp-meta.md`](cp-meta.md) 与本平面。
- 权威协议：[`documents/design/protocol/ERRORS.md`](../documents/design/protocol/ERRORS.md)。
- CLI 退出码：[`documents/design/protocol/CLI.md`](../documents/design/protocol/CLI.md)。
- 阶段收尾使用 `compact-control-plane`。

## Scope

- 入口：`src/domain/errors.ts`、`src/domain`、`src/application`、`src/cli`、`src/package-environment.ts`、错误相关单元/E2E 测试。
- 控制对象：`ErrorCode`、`ApplicationError`、Reason registry、错误工厂、CLI 输出和退出码。
- 不在范围：重写业务流程、引入独立日志系统、改变 transport protocol version、暴露 cause/stack。

## Core Rules

- 每个 Reason 必须在 `documents/design/protocol/ERRORS.md` 声明一次，并且只能映射到一个 Code。
- 代码引用的每个 `scope.*` Reason 必须已声明；协议中没有代码引用的 Reason 必须标为预留或删除，不保留假条目。
- application/domain 在最接近错误语义的边界分类；CLI 只负责格式化、脱敏和退出码映射，不解析 message。
- 未分类异常统一为 `scope.internal`；不得为了隐藏异常而静默吞掉错误。
- `invalid_argument` 退出 `2`；其他 Code 退出 `1`。
- 公开 message/details 必须稳定、可操作、非敏感；禁止公开绝对路径、用户目录、凭据、环境变量、cause 和 stack。
- Scope 内文件路径使用相对 Scope root 的路径；details 字段保持稳定语义。
- 每新增、删除或改变 Reason 语义，必须同步更新协议、实现引用和行为测试。
- 每个已声明 Reason 至少有一个正向行为测试；测试应验证 Code、Reason、退出码和公开输出。

## Task Board

- [x] 按调用输入、Source/package 加载、Workspace 数据、查询、Mutation、未分类故障重排 Reason 文档。
- [x] 为现有和新增 Reason 补充解释、Code 归属和公开诊断规则。
- [ ] 盘点代码中所有 Reason 引用，建立“代码引用 ↔ 协议声明”一一对应检查。
- [ ] 将 domain/application 的裸 Error 迁移到正确的错误工厂和 Reason。
- [ ] 统一 CLI 普通文本与 `--json` 错误输出，加入路径脱敏和稳定 details。
- [ ] 增加每个错误 Code 的 CLI/application 行为测试，覆盖退出码、Reason、message 和敏感信息过滤。
- [ ] 验证完整 unit、E2E、本地 tarball consumer；确认不存在未引用 Reason、非法 Code 或未分类用户输入错误。
