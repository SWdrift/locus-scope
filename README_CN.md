# locus-scope

[![npm version](https://img.shields.io/npm/v/%40sundw%2Flocus-scope?logo=npm)](https://www.npmjs.com/package/@sundw/locus-scope)

[中文](README_CN.md) | [English](README.md)

可组合 Entity 图的 Scope 协议及轻量工具，定义身份、关系、作用域和组合方式。

Locus Scope 将具有身份的事物及其关系组织成有边界的图。Entity 可以表示环境、资源、能力、代码、知识、逻辑结构或其他领域对象；Scope 提供 ownership、命名、可见性与组合边界。一个可分发的 npm Package 对应一个 Scope。

## 使用

项目以 npm Package 提供统一的 `locus-scope` 命令和 library API。npm 或 pnpm 负责依赖解析、lockfile、缓存和安装；runtime 从实际安装环境解析 Package graph，并在同一进程装配、验证 Workspace。

| 使用方式 | 适用情况 | 命令入口 |
| --- | --- | --- |
| [使用 npm](documents/2-使用npm.md) | 使用 npm 或 pnpm 管理 Locus Package，并查询、验证或修改 Scope Workspace。 | `locus-scope` |

从创建第一个 Scope 开始，请阅读[基本使用](documents/1-基本使用.md)。

Agent 可按任务选择对应 Skill：

| Node.js Package 工作流 |
| --- |
| [`$use-locus-scope`](.agents/skills/use-locus-scope/SKILL.md) |

<details>
<summary>从源码构建</summary>
要求使用根 `package.json` 声明的 Node.js LTS 版本，以及其中固定的 pnpm：

```powershell
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run build
pnpm run release:npm
```

TypeScript 构建产物位于 `temp/ts-build/`；npm release tarball 位于 `temp/release/npm/`。

</details>

## 文档

- [基本使用](documents/1-基本使用.md)：创建、检查、组合和查询 Scope。
- [使用 npm](documents/2-使用npm.md)：安装统一 CLI，由 npm 或 pnpm 管理 Package。
- [CLI 参考](documents/4-CLI参考.md)：Workspace 查询、图操作、diff、mutation、输出和退出状态。

## License

[MIT](LICENSE)
