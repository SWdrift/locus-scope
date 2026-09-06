---
name: locus-publish-node
description: "使用 Node.js、npm 或 pnpm 轻量校验、打包并发布 Locus Scope Package。用于 npm 模式下发布、推送或更新 Package，以及检查 packed files、version、integrity 和 Registry 认证。"
metadata:
  domain: locus-scope
---

# Locus Scope · npm 发布 Package

## 目标

在 npm 模式中把一个 Scope 作为标准 npm Package 发布。使用本 Skill 自带的轻量脚本检查 Locus metadata，再以 `npm pack` 生成 tarball，并用裸 `npm publish` 发布同一 tarball；不要求安装 `locus-pkg`。

Skill 是 Agent 发布流程，不是 Registry 强制门禁。它不能阻止其他发布者绕过检查；需要 Pure Locus、强制确定性打包或非 Node.js 环境时使用 `$locus-publish` 和 `locus-pkg`。

## Package 约束

1. Package root 必须有严格 JSON `package.json`，包含合法 `name`、`version`、非空 `files` 和 `locus.entry`，且不得设置 `private: true`。
2. `locus.entry` 必须是 Package 内相对路径，文件名为 `locus.yaml`、`locus.yml` 或 `locus.json`，并被 `files` 包含。
3. Definition documents 使用 `*.locus.yaml`、`*.locus.yml` 或 `*.locus.json`；普通 YAML/JSON 不会被 Loader 读取。
4. `files` 只列相对 literal 文件或目录；不得使用 glob、反斜杠或 `.`/`..` 路径段。
5. 分发 Package 内不得使用本地、绝对、带版本或带 subpath 的 Import。每个 bare Package Import 必须出现在 `dependencies` 中。
6. 如果存在 `exports`，必须包含精确的 `"./package.json": "./package.json"`。
7. 不依赖 npm lifecycle script 生成发布文件；所有 pack/publish 命令均使用 `--ignore-scripts`。
8. npm version 不可变。发布前确认 `name@version` 尚未存在；内容变化必须提升 `version`。
9. token 只通过 npm userconfig 或环境提供，不得写入 Package、lock、`.locus/`、日志或回复。

最小 manifest：

```json
{
  "name": "@example/infra",
  "version": "1.0.0",
  "files": ["locus.yaml", "resources.locus.yaml"],
  "locus": {"entry": "locus.yaml"},
  "exports": {"./package.json": "./package.json"},
  "dependencies": {"@example/base": "^1.0.0"}
}
```

## 发布流程

以下命令在 Package root 执行。先运行随 Skill 分发的校验器；从本 Skill 目录解析脚本路径，不要复制或改写脚本：

```text
node <skill-dir>/scripts/validate-package.mjs .
```

该脚本只执行轻量、无网络的 manifest 与 entry 检查。Agent 仍须检查 Scope Import 与 `dependencies` 的对应关系，并在已安装依赖的 npm Workspace 中运行：

```text
pnpm exec locus-scope-node validate
# npm 项目：npx locus-scope-node validate
```

在仓库工作区内选择 `temp/` 下的全新目录，然后生成 tarball：

```text
npm pack --json --ignore-scripts --pack-destination <workspace>/temp/locus-publish
```

读取 JSON 结果中的 `filename`、`integrity` 和 `files`，确认：

- 包含 `package.json`、`locus.entry` 和预期 Definition documents；
- 不包含 `.npmrc`、凭据、`.git/`、`.locus/`、`node_modules/` 或 `locus.lock`；
- `name`、`version` 与校验过的 manifest 一致。

只发布刚检查过的确切 tarball，绝不重新从目录隐式打包：

```text
npm publish <workspace>/temp/locus-publish/<filename> --ignore-scripts
```

显式 Registry：

```text
npm publish <tarball> --ignore-scripts --registry https://registry.example.com/
```

发布后读取 Registry 中确切 `name@version` 的 metadata，确认 `dist.integrity` 与 `npm pack --json` 返回值一致。最终报告确切 `name@version`、Registry 和 SHA-512 integrity，不只报告可移动的 `latest`。

## Registry 与认证

Registry 和认证遵循 npm 自身规则。可使用命令 `--registry`、`NPM_CONFIG_REGISTRY`、项目或用户 `.npmrc`；scope registry 仍按 npm 规则覆盖默认值。Bearer token 使用目标 Registry 的 `:_authToken` 或 `NPM_TOKEN`。隔离配置可通过 `NPM_CONFIG_USERCONFIG` 指定，但不得把 package-manager cache/store 重定向到仓库。

只允许 loopback Registry 使用明文 HTTP；远端 Registry 必须使用 HTTPS。输出、日志和最终回复不得包含 token。

## 完成检查

- 轻量校验器成功。
- `locus-scope-node validate` 成功。
- `npm pack --json --ignore-scripts` 的 packed files 已检查。
- 发布对象是检查过的同一 tarball。
- Registry metadata 的 `name`、`version` 和 `dist.integrity` 与本地 pack 结果一致。
- 发布输出和仓库产物不含 token。

## 项目

- [SWdrift/locus-scope](https://github.com/SWdrift/locus-scope)
