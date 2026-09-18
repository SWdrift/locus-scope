---
name: locus-use-node
description: "在 Node.js 项目中通过 npm 或 pnpm 安装和使用 Locus Scope，创建、验证、查询 Scope，并消费 npm Registry 中的 Locus Package。"
---

# Use Locus Scope with npm or pnpm

## 安装

```sh
pnpm add -D @sundw/locus-scope
pnpm exec locus-scope validate
```

npm 项目使用 `npm install --save-dev @sundw/locus-scope` 与 `npx locus-scope`。

## Package Scope

Package 使用标准 `package.json`：

```json
{
  "name": "@example/infra",
  "version": "1.2.3",
  "locus": { "entry": "locus.yaml" },
  "exports": { "./package.json": "./package.json" }
}
```

`locus.yaml` 通过 bare dependency name Import Package。版本范围只在 `package.json.dependencies` 声明；npm/pnpm lockfile 固定实际版本。

## 常用命令

```sh
pnpm exec locus-scope validate --json
pnpm exec locus-scope entity --json
pnpm exec locus-scope relation --json
pnpm exec locus-scope graph backend --depth 1 --json
pnpm exec locus-scope path backend infra:database --json
```

在 Scope 目录外使用 `--scope <dir>`。安装完成后的加载不隐式联网。Registry、认证、lockfile、cache 和 node_modules 完全遵循 npm/pnpm；不得把 package-manager cache/store 重定向到仓库。
