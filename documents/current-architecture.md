# locus-scope 目标实现架构

## 目录

```text
cmd/
├── locus-pkg/
├── locus-scope/
└── locus-scope-node-host/

internal/
├── npm/
├── packageenv/
├── purepkg/
├── scope/
└── scopecli/

packaging/
├── npm/
│   ├── locus-scope/
│   ├── locus-scope-win32-x64/
│   ├── locus-scope-linux-x64/
│   ├── locus-scope-linux-arm64/
│   ├── locus-scope-darwin-x64/
│   └── locus-scope-darwin-arm64/
└── windows/
    └── locus.iss

scripts/
├── inno-setup.ps1
├── local-build.ps1
├── local-clean.ps1
├── local-deploy.ps1
├── npm-registry.ps1
├── release-package.ps1
├── test-run.ps1
├── user-install.ps1
└── user-uninstall.ps1
```

## 依赖

```mermaid
flowchart LR
    LP[cmd/locus-pkg] --> PP[internal/purepkg]
    PP --> N[internal/npm]
    PP --> PE[internal/packageenv]
    PE --> S[internal/scope]
    SC[cmd/locus-scope] --> PP
    SC --> CLI[internal/scopecli]
    H[cmd/locus-scope-node-host] --> PE
    H --> CLI
    CLI --> S
    JS[@locus/scope adapter] --> H
```

`internal/scope` 是唯一 Scope/Graph semantic core，不依赖 npm、package.json、lock、cache、Node 或 Registry 类型。`internal/packageenv` 是 resolved package graph 到 `scope.Resolver` 的唯一 adapter。`internal/npm` 只负责 npm protocol、SemVer、Registry、SRI、archive 和 pack/publish artifact。`internal/purepkg` 负责 Pure Locus lock/store/resolution/transaction。`internal/scopecli` 提供两个 Go entrypoint 共用的 query/validation dispatcher。

`@locus/scope` 只构造 importer-relative descriptor、选择 platform host 并转发进程 I/O；不解析 Locus definitions。standalone `locus-scope` 使用 Pure Locus lock/store，Node host 使用 npm/pnpm 已安装环境，但二者进入相同 `packageenv`、`scope.Load` 和 `scopecli`。

## 目录规则

- `internal/` 只按稳定职责建立 package，不使用 `utils`、`common`、`src` 或按文件类型分组。
- `packaging/` 聚合全部交付包定义；npm package source 位于 `packaging/npm/`，Windows installer 定义位于 `packaging/windows/`。
- package manager 与 delivery adapter 不得复制 Scope、Entity、Relation、Import、ownership 或 Workspace 语义。
- 项目级 Verdaccio 与测试状态只写 `temp/`；Node package-manager cache/store 继续使用机器级全局配置。
- 根 `VERSION` 是 standalone 与六个 npm package 的发行版本真相。
