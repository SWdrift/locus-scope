---
name: locus-publish
description: "发布 Locus Scope Package 到 npm Registry。用于用户要求发布、推送、分发或更新 Scope Package，解释 publish、version、integrity、Registry 认证或本地 Verdaccio 发布流程时。"
metadata:
  domain: locus-scope
---

# Locus Scope · 发布 Package

## 目标

把一个 npm Package 中唯一的 Scope 发布为标准 npm tarball。发布由独立 `locus-pkg` 完成；`locus-scope` 只负责加载、验证和查询 Workspace。一个可发布 Package 对应一个 Scope，稳定 identity 是 `npm:<name>@<version>`。

## 发布前检查

1. Package root 必须有严格 JSON `package.json`，包含合法 `name`、`version`、非空 `files` 和 `locus.entry`。
2. `locus.entry` 必须是 Package 内相对路径，并指向 packed tree 中唯一的 `locus.yaml`、`locus.yml` 或 `locus.json`。
3. Definition documents 使用 `*.locus.yaml`、`*.locus.yml` 或 `*.locus.json`；普通 YAML/JSON 不会被 Loader 读取。
4. `files` 只列相对 literal 文件或目录。拒绝 glob、`.npmignore`、bundled dependencies、symlink 和依赖 lifecycle script 的打包。
5. 分发 Package 内不得使用本地或绝对 Import。每个 bare Package Import 必须是无 version/subpath 的 npm 名，并出现在 `dependencies` 中。
6. 如果 `package.json` 有 `exports`，必须保留精确的 `"./package.json": "./package.json"`，让 Node adapter 能从 importer 上下文发现 Package。
7. 运行 `locus-pkg pack` 或 `locus-pkg publish` 前用 `locus-scope validate` 验证完整 Workspace。需要 Pure Locus 依赖时先运行 `locus-pkg install`。
8. 选择可写 Registry，并确认当前 `name@version` 尚未发布。npm 版本不可变；内容变化必须提升 `version`。
9. token 只通过 npm userconfig 或环境提供，不得写入 `package.json`、`locus.lock`、`.locus`、日志或回复。

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

## 打包与发布命令

进入 Package root 后先检查本地 tarball：

```text
locus-pkg pack
```

发布到按配置选择的 Registry：

```text
locus-pkg publish
```

显式覆盖 Registry：

```text
locus-pkg publish --registry https://registry.example.com/
```

供 Agent 或脚本消费时使用稳定 JSON：

```text
locus-pkg --json publish --registry https://registry.example.com/
```

成功结果包含 `name`、`version`、`registry` 和 `integrity`。必须记录确切 `name@version` 与 SHA-512 integrity；不要只记录可移动的 `latest` dist-tag。

## Registry 与认证

Registry 选择顺序：

1. 命令 `--registry`
2. `NPM_CONFIG_REGISTRY`
3. 项目 `.npmrc`
4. 用户 `.npmrc`

匹配 Package scope 的 `<scope>:registry` 覆盖 default registry。认证使用目标 registry 最长路径前缀匹配的 `:_authToken` Bearer token，也可用 `NPM_TOKEN`。`NPM_CONFIG_USERCONFIG` 可指定隔离 userconfig。用户名/密码/login 字段不是受支持的运行时凭据来源。

只允许 loopback host 使用明文 HTTP；其他 Registry 必须使用 HTTPS。不得跨 origin 或 redirect 转发 Authorization。

## 本地 Verdaccio

仓库开发环境使用 project-local Verdaccio：

```powershell
pnpm install --frozen-lockfile
pnpm run registry:start
```

它只监听 `http://127.0.0.1:4873/`，匿名读取允许，发布要求认证。把开发登录隔离在 temp state，而不是改真实用户配置：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
$env:NPM_CONFIG_REGISTRY = 'http://127.0.0.1:4873/'
locus-pkg publish
```

完成后：

```powershell
pnpm run registry:stop
```

脚本不写 `.npmrc`，也不重定向 package-manager cache/store。state、storage、auth 和日志都在 `temp/verdaccio-dev/`；需要清空时运行 `pnpm run registry:reset`。

## 发布语义

- tarball 使用标准 `package/` prefix，先按 reduced `files` contract 确定内容，再按 slash path 排序并规范化 metadata。
- root `package.json` 与 root README/LICENSE/LICENCE/NOTICE 自动包含；`.git`、`.locus`、`node_modules`、`locus.lock` 和生成归档排除。
- 发布 body 包含 version metadata、`dist-tags.latest`、SHA-1 shasum、SHA-512 integrity 和一个 base64 tarball attachment。
- 同一 `name@version` 是不可变 identity；Registry 冲突不是可覆盖更新。
- Pure Locus consumer 通过 Registry SemVer、`locus.lock` 和项目 `.locus` 使用 Package；npm/pnpm consumer 通过自己的 lock 与安装布局使用相同 Package。
- 发布和安装不执行 provisioning、reconciliation 或实际部署漂移检查。

## 完成检查

- `locus-pkg pack` 的文件列表符合显式 `files` contract，并只含一个 `locus.entry`。
- 发布成功返回预期 name、version、registry 和 integrity，输出不含 token。
- Registry 中 version metadata 的 tarball URL 与 integrity 可读取，确切版本不能被第二次发布覆盖。
- 若需验证消费路径，在独立项目的 `package.json.dependencies` 声明版本范围，在 root Scope 用 bare Package 名 Import，然后选择一种环境验证：
  - Pure：`locus-pkg install` 后运行 `locus-scope validate`。
  - npm/pnpm：安装 `@sundw/locus-scope` 后运行 `locus-scope-node validate`。
