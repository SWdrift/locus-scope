# locus-scope

可组合 Entity 图的 Scope 协议及轻量工具，定义身份、关系、作用域和组合方式。

Locus Scope 将具有身份的事物及其关系组织成有边界的图。Entity 可以表示环境、资源、能力、代码、知识、逻辑结构或其他领域对象；Scope 提供 ownership、命名、可见性与组合边界。一个可分发的 npm Package 对应一个 Scope。

## 使用

提供两种使用模式。都有相同的 Scope 文件和查询语义，只是由不同工具管理 Package 依赖。

| 模式 | 适用情况 | 命令入口 |
| --- | --- | --- |
| [使用npm](documents/使用npm.md) | 已有 Node.js 项目；沿用 npm 或 pnpm 的安装、lockfile 和缓存。 | `locus-scope-node` |
| [使用独立CLI](documents/使用独立CLI.md) | 非 Node.js 项目；只使用本地 Scope；由 Locus 管理 Package lock 和离线缓存。 | `locus-scope`、`locus-pkg` |

从创建第一个 Scope 开始，请阅读[基本使用](documents/基本使用.md)。

<details>
<summary>从源码构建</summary>

要求 Go 1.26+、Node.js 20.6+ 和根 `package.json` 固定的 pnpm：

```powershell
pnpm run build
pnpm run deploy
```

产物位于 `temp/local/bin/`。用户级部署使用 `pnpm run deploy:user`；完整脚本说明见 [`scripts/README.md`](scripts/README.md)。

</details>

## 文档

- [基本使用](documents/基本使用.md)：以 npm 模式完成 Scope 创建、查询和组合，并说明独立 CLI 差异。
- [使用npm](documents/使用npm.md)：安装 Node adapter，由 npm 或 pnpm 管理 Package。
- [使用独立CLI](documents/使用独立CLI.md)：安装独立命令，管理 lock、离线状态和 Package 发布。
- [CLI](documents/design/protocol/CLI.md)：查询与 Package 管理指令、参数和输出约定。
- [设计文档](documents/design/README.md)：协议、Scope、Package、测试和 Windows 安装包的权威设计。

## License

[MIT](LICENSE)
