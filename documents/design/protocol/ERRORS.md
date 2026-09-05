# 错误契约

## 简述

Scope 与 Package application API 使用统一、可分类并保留 cause 的错误。CLI、Node host 和未来其他 adapter 只负责 transport 映射，不解析错误文本判断行为。

## 职责

本文定义 application error 的稳定字段、分类和传播规则。领域有效性由[核心协议](PROTOCOL.md)、[Scope 设计](../Scope设计.md)和 [Package 设计](../Package设计.md)定义；CLI 输出和退出状态由 [CLI](CLI.md)定义；成功结果由[核心 API](CORE_API.md)定义。

## 错误模型

```go
type Error struct {
    Code    Code
    Reason  string
    Message string
    Details map[string]string
    Cause   error
}
```

- `Code` 是少量跨领域分类，供消费层决定处理方式。
- `Reason` 是 `<domain>.<reason>` 格式的稳定领域标识。
    - 使用 `scope.*` 或 `pkg.*` 命名空间；
    - 每个 Reason 必须归属一个 Code；
    - 已发布 Reason 不得改变或复用其语义。
- `Message` 面向人类，可以改善措辞；程序不得解析它。
- `Details` 保存该 reason 所需的结构化上下文。
- `Cause` 支持 Go `errors.Is`、`errors.As` 和诊断，不进入外部协议。

## Code

| Code                  | 含义                                                                  |
| --------------------- | --------------------------------------------------------------------- |
| `invalid_argument`    | 调用参数缺失、格式错误或组合无效。                                    |
| `invalid_data`        | Manifest、definition、lock、Registry response 或 archive 不符合协议。 |
| `not_found`           | 请求的资源不存在。                                                    |
| `failed_precondition` | Workspace、lock、store 或 package environment 不满足操作前提。        |
| `conflict`            | identity、不可变版本或事务状态冲突。                                  |
| `integrity_failed`    | cache、store、download 或 archive 不满足声明 integrity。              |
| `unauthenticated`     | 缺少或拒绝认证凭据。                                                  |
| `permission_denied`   | 身份有效但没有操作权限。                                              |
| `unavailable`         | Registry、文件系统或依赖服务不可用。                                  |
| `internal`            | 无法归入以上类别的实现错误。                                          |

未知 Code 按 `internal` 处理。已有 Code 的含义不得改变；只有消费层确实需要不同处理策略时才新增 Code。

## Reason

> TODO：typed error 落地时，按真实错误产生点登记首批 Reason 及其 Code。

## 传播规则

- 最接近错误语义的层负责确定 Code 和 Reason。
- 上层保留 Code、Reason 和 cause，只增加必要上下文。
- 未分类错误在 application API 边界转换为 `internal`。

