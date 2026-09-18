# npm Package Control Plane

## Control Plane Role

本平面管理 npm/pnpm 托管的 Scope Package Environment、Node adapter、platform host 与 npm 发布链。

## Meta Reference

- 执行前读取 [`cp-meta.md`](cp-meta.md) 与 [`cpm-npm-packages.md`](cpm-npm-packages.md)。
- 权威设计见 [`documents/design/Package设计.md`](documents/design/Package设计.md)。
- 阶段收尾使用 `compact-control-plane`。

## Scope

- 入口：`packaging/npm/`、`cmd/locus-scope-node-host`、`internal/packageenv`、npm release scripts、npm E2E。
- npm/pnpm 负责 dependency resolution、lock、integrity、cache、installation、pack、publish 与 auth。
- Node 负责 package.json、安装布局与 exact resolved graph；Go 只负责 Scope Resolver 不变量。
- 不在范围：自建 package manager、Registry client、SemVer resolver、lock/store/materialization、standalone installer。

## Core Rules

- Package identity 为 `npm:<name>@<exact-version>`。
- 主 npm package 通过 `optionalDependencies` 分发 platform Go host。
- 公开 executable 只有 `locus-scope`；Go host 是私有 transport。
- 发布使用 npm 原生 pack/publish；Verdaccio 只用于工作区内 E2E。
- 测试状态留在仓库，package-manager cache/store 使用机器级配置。

## Task Board

当前无待执行任务。
