# CLI

## 简述

locus-scope 提供两个 Scope 查询入口和一个 Package 管理入口：

- `locus-scope`：从本地文件、`locus.lock` 和 `.locus/` 加载 Workspace。
- `locus-scope-node`：从 npm 或 pnpm 已安装的依赖加载 Workspace。
- `locus-pkg`：安装、更新、打包和发布 npm-compatible Locus Package。

`locus-scope` 与 `locus-scope-node` 的查询指令和输出语义相同。

## 职责

本文是 CLI 指令、参数、输出和退出状态的公共契约。Scope 数据结构见[核心协议](PROTOCOL.md)；Package 格式、lock/store 和 Registry 规则见[Package 设计](../Package设计.md)；入门步骤见[基本使用](../../基本使用.md)。

## Scope 查询

以下指令同时适用于 `locus-scope` 和 `locus-scope-node`。

| 指令 | 作用 |
| --- | --- |
| `validate` | 加载并验证完整 Workspace，输出 root identity 以及 Scope、Entity、Relation 数量。 |
| `scope show` | 显示 root Scope 的 ID、Source identity、Imports 和 Exports。 |
| `scope list` | 列出全部可达 Scope，并标记 root Scope。 |
| `entity list` | 列出全部可达 Entity 及其原始 owner。 |
| `entity show <ref>` | 解析引用并显示 Entity 的 owner、ID 和属性。 |
| `relation list` | 列出验证后的 Relation 及两端 Entity 的原始 owner。 |
| `resolve <ref>` | 解析 Entity reference，只返回最终 owner 和 ID。 |
| `version` | 输出 CLI 版本，不查找或加载 Scope。 |
| `help` | 输出指令帮助，不查找或加载 Scope。 |

### 通用参数

| 参数 | 作用 |
| --- | --- |
| `--scope <dir>` | 指定 root Scope 目录；省略时从当前目录沿祖先目录查找最近的 `locus.yaml`、`locus.yml` 或 `locus.json`。 |
| `--json` | 输出字段和顺序稳定的 JSON；失败信息写入 stderr，格式为 `{"error":"..."}`。 |
| `--version` | 等价于 `version`。 |
| `--help`、`-h` | 等价于 `help`。 |

`<ref>` 是从 root Scope 解析的 Entity reference。Import alias 使用 `:` 分隔，Scope 内 Group path 使用 `/` 分隔。

## Package 管理

以下指令适用于 `locus-pkg`。

| 指令 | 作用 |
| --- | --- |
| `install` | 按 `package.json.dependencies` 安装完整依赖图，提交 `locus.lock` 和 `.locus/` 状态。 |
| `install <package-spec>...` | 添加或修改直接依赖，然后安装完整依赖图。 |
| `uninstall <package>...` | 删除指定直接依赖，并剪除不再可达的 Package。 |
| `update [<package>...]` | 在已声明的版本约束内更新指定直接依赖；省略名称时更新全部直接依赖。 |
| `list` | 从 `locus.lock` 和 `.locus/` 输出当前已解析依赖树。 |
| `pack` | 校验当前 Package，并生成确定性的 npm `.tgz`。 |
| `publish` | 打包并向 Registry 发布当前不可变 `name@version` 和 `latest` dist-tag。 |
| `version` | 输出 CLI 版本，不查找项目或读取 Registry 配置。 |
| `help` | 输出指令帮助，不查找项目。 |

`install`、`uninstall`、`update` 和 `list` 从当前目录向上查找同时包含 `package.json` 和 root Scope 文件的最近目录。`pack` 和 `publish` 查找最近包含有效 `locus.entry` 的 `package.json`。

### 通用参数

参数可以放在子命令前后。

| 参数 | 作用 |
| --- | --- |
| `--registry <url>` | 覆盖 npm Registry；否则依次读取环境变量、项目 `.npmrc` 和用户 `.npmrc`。 |
| `--offline` | 禁止 Registry 请求，只使用已有 lock、缓存和展开状态。 |
| `--frozen-lockfile` | 要求 `package.json` 与 `locus.lock` 完全一致，且不修改声明或 lock。不能与显式 `install <package-spec>` 一起使用。 |
| `--json` | 输出稳定 JSON；失败信息写入 stderr，格式为 `{"error":"..."}`。 |
| `--version` | 等价于 `version`。 |
| `--help`、`-h` | 等价于 `help`。 |

`publish` 不接受 `--offline` 或 `--frozen-lockfile`。

## 输出与退出状态

- 普通输出写入 stdout，错误写入 stderr。
- `--json` 同时约束成功与失败输出；凭据不得出现在任何输出中。
- exit code `0` 表示成功。
- exit code `1` 表示加载、验证、网络、协议或事务失败。
- exit code `2` 表示未知指令、未知参数、缺少参数、不允许的参数组合或无效的查询输入。
