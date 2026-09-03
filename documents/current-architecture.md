# locus-scope 当前实现快照

## 目录

```text
cmd/
├── locus-scope/
└── locus-pkg/

internal/
├── scope/
└── packages/

documents/
test/
└── e2e/
```

## 依赖

```text
cmd/locus-scope → internal/scope + internal/packages
cmd/locus-pkg   → internal/scope + internal/packages
internal/packages → internal/scope
internal/scope    → 不依赖 internal/packages
```

`cmd` 只负责参数、调用和输出。`internal/scope` 负责 Core semantics；`internal/packages` 负责 OCI、lock、cache、物化和 Source resolver。项目不使用 `src/`。

