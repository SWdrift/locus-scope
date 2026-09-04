---
name: locus-use
description: "指导用户从安装 Locus Scope 到创建、验证、查询和安装 Package，并解释 Entity、Relation、Scope、Import、Export、ownership 与离线复现理念。用于入门、安装后配置、基本使用、Package 消费或理念说明。"
metadata:
  domain: locus-scope
---

# Locus Scope · 从安装到使用

## 目标

用最短路径让用户安装 CLI、创建第一个 Scope、查询 Entity 图，并按需安装 OCI Package。先给可执行步骤，再解释支撑这些步骤的模型和理念。

## 安装

### Windows 发布包

运行 `locus-setup-windows-amd64.exe`，至少选择 `locus-scope`。需要发布或安装 OCI Package 时同时选择 `locus-pkg`；需要本机 Registry 时选择 Zot。安装目录固定为 `%USERPROFILE%\.locus`。

安装器可以把 `%USERPROFILE%\.locus\bin` 加入当前用户 `PATH`。若未选择该任务，使用完整路径调用 CLI，或由用户自行配置 PATH。安装包包含所选组件，安装时不要求 Go、pnpm、PowerShell 7 或联网。

安装后新开一个终端。`--help` 只能确认命令入口可调用：

```text
locus-scope --help
```

版本兼容检查不能只依赖 `--help`。先创建目录 `locus-smoke/`，并在其中写入只含以下内容的 `locus-smoke/locus.yaml`：

```yaml
id: smoke
```

再执行真实加载和验证：

```text
locus-scope --scope ./locus-smoke validate
```

需要发布或安装 Package 时，再确认 `locus-pkg --help` 可调用。只安装 `locus-scope` 时，没有 `locus-pkg` 是预期行为。

### 仓库开发环境

从源码工作时在仓库根目录运行：

```powershell
pwsh -File scripts/deploy-local.ps1
```

CLI 位于 `temp/local/bin/`。只有明确需要写入当前用户安装目录时才使用 `-User`；需要本地 Zot 时追加 `-WithZot` 并单独启动它。

## 创建和使用第一个 Scope

建立目录和 `locus.yaml`：

```yaml
id: app
```

在同一目录添加 `entities.yaml`：

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

验证并查询：

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity list
locus-scope --scope ./app entity show database
locus-scope --scope ./app relation list
locus-scope --scope ./app resolve database
```

进入 Scope 目录后可以省略 `--scope`。CLI 会从当前目录向父目录寻找最近的 Scope manifest。Agent 或脚本应追加 `--json` 获取字段和顺序稳定的输出。

Relation 的定义输入和 CLI JSON 输出不是同一种表示：

- YAML definition document 使用三元组 `[from, relation, to]`。
- `locus-scope --json relation list` 输出解析后的对象；`from` 和 `to` 都包含 owner 的 `scope_id`、Source identity `scope` 和 Entity `id`。
- `--json` 只选择 CLI 输出格式，不会改变 definition document 的输入语法。

例如，上述 YAML 的 JSON 查询结果形如：

```json
{
  "relations": [
    {
      "from": {
        "scope_id": "app",
        "scope": "file:///project/app",
        "id": "backend"
      },
      "name": "uses",
      "to": {
        "scope_id": "app",
        "scope": "file:///project/app",
        "id": "database"
      }
    }
  ]
}
```

## 组合本地 Scope

在 root Scope 的 manifest 中为另一个 Scope 声明 Import：

```yaml
id: app

imports:
    infra: ../infra
```

导入内容始终通过 Projection 前缀访问，例如 `infra:database`。目标 Scope 未显式 export 的成员不可见。

## 安装并使用 Package

假设远端 Package 的 root Scope 已显式 export `database`。项目仍通过 Import 声明该 Package，不引入另一套依赖文件：

```yaml
id: app

imports:
    infra: oci://registry.example.com/locus/infra:v1
```

在 root Scope 中执行：

```text
locus-pkg install
locus-scope validate
locus-scope resolve infra:database
```

`locus-pkg install` 解析完整 Package 依赖闭包，生成或更新 `locus.lock`，并把内容物化到 `.locus/packages/`。之后 `locus-scope` 只读取 manifest、lock 和项目物化目录，不访问 Registry 或用户级 OCI cache。

CI 或严格复现使用：

```text
locus-pkg install --frozen
```

`--frozen` 要求现有 lock 与完整依赖一致，不修改 lock；本地缺失的已锁内容仍可按 digest 获取。

### CI

项目应把 Workspace 验证放进 CI：

```text
locus-pkg install --frozen
locus-scope --json validate
```

没有 OCI Package 依赖时可以省略第一条。`validate` 检查声明文件、可达 Scope、可见性、引用和 Relation 等静态图语义；它不读取实际部署状态，因此不能检查或证明 provisioning 结果、运行状态或部署漂移。

## 理念

- **Entity 图优先**：Entity 表示任何需要独立身份、属性或关系的对象；Relation 是两个 Entity 之间的有向语义边。
- **最小核心与开放属性**：Entity 只有 `id` 必需，其余属性是对 Core 不透明的开放数据。`validate` 不替业务执行 Entity 属性中的端口范围、业务路径、环境策略或其他领域校验；需要这些约束的项目应在自己的 schema、策略或 CI 中实现。
- **Scope 是边界**：Scope 同时提供 ownership、命名和可见性边界。Entity 在所属 Scope 内唯一，默认私有，只有显式 Export 才公开。
- **组合而非复制**：Import 创建带别名的 Projection，不展平、不复制 Entity，也不改变原始 ownership。Scope 可以继续 Import、Export 和重新组合。
- **来源决定身份**：manifest `id`、Import alias 和路径写法都不是 Source identity。本地 Source 使用规范 `file://` identity；Package 使用不可变 OCI manifest digest。
- **声明与快照分离**：manifest 声明 OCI reference，`locus.lock` 把可变 tag 固定到 digest，`.locus/packages` 保存项目可直接加载的内容。
- **安装联网，使用离线**：获取和缓存属于 `locus-pkg install`；验证和查询属于 `locus-scope`。日常读取不隐式访问 Registry，结果不会随远端 tag 漂移。
- **开放基础设施**：分发复用 OCI Registry 与 ORAS 生态。Locus 不自建 Registry Server、账号系统、SemVer solver 或隐式依赖模型。
- **描述而非执行**：Locus 负责描述、组合、分发和查询 Entity 图，不负责 provisioning，也不负责把声明状态与实际系统持续 reconciliation。

## 常见边界

- `locus-scope install` 不是有效命令；Package 安装命令是 `locus-pkg install`。
- `locus-scope validate` 验证完整 reachable graph，不只检查 root manifest。
- `validate` 不执行 provisioning、reconciliation 或实际部署漂移检查，也不替项目验证开放属性中的领域策略。
- `:` 分隔 imported Projection，`/` 表示 Scope 内 Group 路径。
- 同名 manifest ID 不代表同一 Scope；ownership 和 Source identity 仍由实际来源决定。
- 缺少 lock entry 或 `.locus/packages` 内容时，先运行 `locus-pkg install`，不要手工拼装物化目录。
