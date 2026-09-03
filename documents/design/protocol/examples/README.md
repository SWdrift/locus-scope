# Scope composition example

本示例只演示核心协议，不代表文件发现、codec 或 Import path 的最终实现契约。

```text
composition/
├── app/
│   ├── locus.yaml
│   └── services.yaml
└── infra/
    ├── locus.yaml
    ├── resources.yaml
    └── backend.yaml
```

- `infra` 公开根 Entity `database` 和 Group Entity `backend/worker`。
- `app` 通过 Projection `infra` 引用二者。
- `app` 公开自己的 `api`，并再次公开 `infra:database`。
- 若第三个 Scope 以 `product` 导入 `app`，它可以使用 `product:api` 和 `product:infra:database`，但不能使用未由 `app` export 的 `product:infra:backend/worker`。

Entity 的 `type`、`engine`、`runtime`、`port` 和 `metadata` 都是示例属性，不属于 Core schema。
