# locus-scope

可组合 Entity 图的 Scope 协议及轻量工具，定义了身份、关系、作用域和组合方式。

Locus Scope 将具有身份的事物及其关系组织成有边界的图。Entity 可以表示环境、资源、能力、代码、知识、逻辑结构或其他领域对象，属性和关系不受预设模型限制。

Scope 提供 Entity 的命名空间、组合与引用边界，并可通过 Import / Export 组合其他 Scope。Scope 也可以作为 OCI Artifact 发布到 [OCI Registry](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)，从而在不同项目和环境之间分发与复用。

## Quick Start

### Window

- Windows 用户可直接运行 `locus-setup-windows-amd64.exe`，按需选择 `locus-scope`、`locus-pkg`、[Zot](https://zotregistry.dev/) 和当前用户 `PATH`。
- 安装根目录固定为 `%USERPROFILE%\.locus`；安装包包含全部组件。
- 安装 Zot 后可通过开始菜单启动、停止和查看状态，也可选择登录 Windows 后自动启动。卸载默认保留 Zot 仓库数据和 `%USERPROFILE%\.locus\oci` cache。

### Linux

TODO

<details>
<summary>从源码构建与本地部署</summary>

要求：

- Go 1.26 或更新版本（源码构建；最低版本以 `go.mod` 为准）
- PowerShell 7（仓库脚本）
- 可选：OCI Registry，例如 Zot、Harbor、GHCR

构建：

```powershell
pwsh -File scripts/build.ps1
```

部署到仓库工作区：

```powershell
pwsh -File scripts/deploy-local.ps1
```

产物位于 `temp/local/bin/`。只有显式传入 `-User` 才会部署到用户的 `~/.locus/bin/`：

```powershell
pwsh -File scripts/deploy-local.ps1 -User
```

追加 `-WithZot` 可同时安装对应 target 下的本地 Zot。清理或卸载使用匹配的 option：

```powershell
pwsh -File scripts/clean-local.ps1
pwsh -File scripts/clean-local.ps1 -User -WithZot
```

脚本不会修改 `PATH`，完整选项和数据删除边界见 [`scripts/README.md`](scripts/README.md)。

</details>

## example：创建、安装与发布 Scope

一个项目只有一个 root `locus.yaml` 和一份 `locus.lock`；普通子目录只组织 Definition documents。

### 1. 创建 Scope

创建 `app/locus.yaml`：

```yaml
id: app

exports:
    - backend
```

在普通子目录中创建 `app/model/services.locus.yaml`：

```yaml
entities:
    - id: database
      type: postgres
      host: db.internal
      port: 5432

    - id: backend
      type: service

relations:
    - [backend, uses, database]
```

目录不会产生隐式 Group；未声明 `group` 的 `database` 和 `backend` 都位于 Scope 根命名空间。Definition document 必须使用 `.locus.yaml`、`.locus.yml` 或 `.locus.json` 后缀，普通 YAML/JSON 不会被读取：

```text
app/
├── locus.yaml
├── docker-compose.yaml
└── model/
    └── services.locus.yaml
```

Loader 递归普通子目录，但遇到包含 Scope manifest 的后代目录时停止；该 Scope 只有经 `imports` 才会加入 Workspace。可选的 `app/.locusignore` 用来排除无需遍历的路径：

```text
# Scope-relative paths
generated/
*.draft.locus.yaml
```

`.locusignore` 使用 `/` 分隔的 Scope 相对模式，支持单路径段内的 `*`、`?` 和字符类；不支持 `!` 与 `**`。匹配目录会跳过整棵子树；`.git` 和 `.locus` 始终跳过。

验证和查询：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity list
locus-scope --scope ./app entity show database
locus-scope --scope ./app relation list
locus-scope --scope ./app resolve database
```

在 Scope 内部执行时可以省略 `--scope`，CLI 会从当前目录沿祖先链查找最近的 Scope manifest；找不到就直接失败，不做用户级 Scope 回退。

### 2. 安装 Package

项目通过 OCI reference 引用远程 Scope：

```yaml
id: app

imports:
    infra: oci://registry.example.com/locus/infra:v1

exports:
    - backend
```

在项目根执行一次安装：

```text
locus-pkg --scope ./app install
```

安装会解析完整 reachable graph，统一生成一份 lock 和项目物化状态：

```text
app/
├── locus.yaml
├── locus.lock
├── model/
│   └── services.locus.yaml
└── .locus/
    └── packages/
        └── sha256-abc.../
            ├── locus.yaml
            └── ...
```

`locus.lock` 将可变 tag 固定到不可变 OCI digest。之后 `locus-scope` 只使用 `locus.lock` 和 `.locus/packages` 离线装配 Workspace，不访问 Registry：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app resolve infra:database
```

严格复用现有 lock：

```text
locus-pkg --scope ./app install --frozen
```

### 3. Publish

待发布 Package 同样以单个 root `locus.yaml` 为入口：

```text
infra/
├── locus.yaml
└── resources.locus.yaml
```

发布：

```text
locus-pkg --scope ./infra publish oci://registry.example.com/locus/infra:v1
```

省略 `--scope` 时，从当前目录沿祖先链查找最近的 Scope manifest。Publish 检查并把 root Scope source tree 编码为 OCI 1.1 artifact，推送成功后输出目标 tag 和不可变 manifest digest。

相同内容重复发布得到相同 digest；内容变化后再次发布同一 tag，会让该 tag 指向新 digest。已有项目的 `locus.lock` 仍固定原 digest，不会自动漂移。

Registry、认证和传输由 OCI / ORAS 生态处理；Locus 没有实现 Registry Server 或独立的账号系统。

#### 使用本地 Zot

若安装时选择了 Zot ，可从开始菜单启动、停止和查看状态；若安装时选择了登录后自动启动，则无需手动启动。Zot 启动后监听 `127.0.0.1:18080`。

进入待发布的 Scope，将它发布到本机 Registry：

```text
locus-pkg --scope . publish oci://localhost:18080/locus/my-scope:v1
```

其他项目通过完整 OCI reference 引用该 Package：

```yaml
id: my-app
imports:
    shared: oci://localhost:18080/locus/my-scope:v1
```

在引用项目中获取 Package、生成 `locus.lock` 并物化依赖：

```text
locus-pkg --scope . install
locus-scope --scope . validate
```

本地 Zot 不需要 `docker login`，只允许本机访问。`localhost:18080` 不能供其他电脑使用；跨机器共享应改用可访问的 OCI Registry，并在 publish 参数和 `locus.yaml` 中填写其地址。

## CLI

`locus-scope` 负责加载、验证和查询 Workspace；`locus-pkg` 负责发布和安装 OCI Package。

### `locus-scope`

| 命令                            | 作用                                                                       |
| ------------------------------- | -------------------------------------------------------------------------- |
| `locus-scope validate`          | 验证完整 Workspace，并输出 root identity 及 Scope、Entity、Relation 数量。 |
| `locus-scope scope show`        | 显示 root Scope 的 manifest ID、source identity、Imports 和 Exports。      |
| `locus-scope scope list`        | 列出所有可达 Scope，并标记 root Scope。                                    |
| `locus-scope entity list`       | 列出所有可达 Entity 及其原始 owner。                                       |
| `locus-scope entity show <ref>` | 从 root Scope 解析 Entity reference，并显示 owner、ID 和属性。             |
| `locus-scope relation list`     | 列出验证后的 Relation 及两端 Entity 的原始 owner。                         |
| `locus-scope resolve <ref>`     | 只解析 Entity reference，返回最终 owner 和 ID，不返回属性。                |
| `locus-scope version`           | 输出构建时注入的版本，不发现或加载 Scope。                                |

通用参数：

| 参数            | 作用                                                                 |
| --------------- | -------------------------------------------------------------------- |
| `--scope <dir>` | 指定 root Scope；省略时从当前目录向父目录查找最近的 Scope manifest。 |
| `--json`        | 输出字段和顺序稳定的 JSON，供 Agent 和脚本消费。                     |
| `--version`     | 等价于 `version` 指令。                                           |

### `locus-pkg`

| 命令                          | 作用                                                                                                    |
| ----------------------------- | ------------------------------------------------------------------------------------------------------- |
| `locus-pkg publish <oci-tag>` | 检查所选 Scope source tree，构建并发布 OCI artifact，成功后输出规范化 target 和不可变 manifest digest。 |
| `locus-pkg install`           | 解析完整 Package 依赖闭包，获取并物化 artifact，验证 Workspace，成功后提交 `locus.lock`。               |
| `locus-pkg version`            | 输出构建时注入的版本，不发现 Scope，也不读取凭据或 Registry。                                       |

参数：

| 参数            | 适用命令             | 作用                                                                 |
| --------------- | -------------------- | -------------------------------------------------------------------- |
| `--scope <dir>` | `publish`、`install` | 指定 root Scope；省略时从当前目录向父目录查找最近的 Scope manifest。 |
| `--json`        | `publish`、`install`、`version` | 输出稳定 JSON；错误也以 `{"error":"..."}` 输出。                     |
| `--frozen`      | `install`            | 要求现有 lock 与完整依赖一致，且不修改 lock。                        |
| `--version`     | 全部                 | 等价于 `version` 指令。                                               |

option 可以位于子命令前后；`version` 和 `--version` 无需 Scope。`publish` 的 `<oci-tag>` 必须是无 fragment 的 OCI tag reference，不能使用 digest reference。

## 技术栈

- **Go 1.26+**：最低支持版本由 `go.mod` 的 `go` directive 定义。
- **OCI Image Spec 1.1 / Distribution API**

## License

[MIT](LICENSE)
