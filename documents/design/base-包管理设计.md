# 包管理设计

## 简述

Locus Package Infrastructure 负责把 OCI 中不可变的 Scope source tree 解析、获取并物化到本地，再通过统一的 Source resolver 装配 Workspace。它扩展 Source 获取能力，不改变 [核心协议](protocol/PROTOCOL.md) 中的 Entity、Relation、Import、Export、Projection 或 Group 语义。

## 职责

负责 Package artifact、Source identity、`locus.lock`、全局 OCI cache、项目物化目录、安装流程、loader 接口和 `locus-pkg install` 契约。

不负责定义新的 Scope 依赖模型、语义化版本或版本范围、dependency solver、自有 Registry Server、发布命令、签名与供应链策略。Scope graph 的唯一来源仍是各 Scope manifest 的 `imports`。

代码目录与依赖方向只在 [当前架构](../current-architecture.md) 中维护，本文不重复目录树。

## 已确认决策

| 主题 | 决策 |
| --- | --- |
| OCI payload | 单个 OCI 标准 tar+gzip layer |
| CLI | 独立的 `locus-pkg install` |
| 项目根 | root Scope 目录 |
| Lock 策略 | 默认复用已有解析并补全缺项；`--frozen` 禁止 lock 变化 |
| Loader | `scope.Load(root Source, resolve Resolver)` |

不引入 SemVer、版本范围或 dependency solver。Tag 是可变引用；Digest 是不可变 Package identity。

## Package artifact

Package 是一个可分发的 Scope source tree 快照，不等于 Scope。

- artifact 根目录必须是一个 root Scope；
- artifact 内可以包含其他通过相对路径导入的 Scope；
- artifact 内的相对 Import 必须保持在该 Package source tree 内；
- 跨 Package Import 必须使用 `oci://` reference。

v1 artifact 使用 OCI 1.1 manifest：

```text
artifactType:
  application/vnd.locus.scope.package.v1

layers:
  exactly one
  application/vnd.oci.image.layer.v1.tar+gzip
```

Package digest 指 OCI root manifest descriptor digest，不是 layer digest。Tag 解析、lock、`ScopeKey` 和项目物化目录都以该 manifest digest 为准。

tar 根目录直接对应 Package source tree，不增加包名或版本目录。安装端必须拒绝：

- artifact type 不匹配；
- layer 数量不是一个；
- layer media type 不匹配；
- 绝对路径、`.` 或 `..` 逃逸路径；
- 使用反斜杠制造 Windows 路径歧义的 entry；
- 重复路径；
- symlink、hardlink、device、FIFO 等非普通文件和目录 entry；
- 解包后缺少唯一 root Scope manifest 的 artifact。

layer descriptor 的 digest 与 size 由 OCI/ORAS 内容读取路径校验。物化先写临时目录，完整验证成功后再原子替换目标目录；失败不得留下看似已安装的 Package。

## Source 与 loader

`Manifest.ID`、Import alias、Source reference、`ScopeKey` 和 `LocalPath` 是不同概念：

- Source reference：manifest 中的原始字符串；
- `ScopeKey`：Workspace 内稳定标识一个实际 Scope source；
- `LocalPath`：当前机器上可读取该 Scope 的目录；
- Import alias：当前 Scope 中的 Projection 名称。

核心接口统一为：

```go
type Source struct {
    Key       ScopeKey
    LocalPath string
}

type Resolver interface {
    Resolve(from Source, reference string) (Source, error)
}

func Load(root Source, resolve Resolver) (*Workspace, error)
```

本轮执行 clean cutover：现有 `Load(rootDirectory string)` 调用方迁移到新签名，不保留同名兼容入口。`scope` package 提供构造本地 root Source 和本地 Resolver 的直接能力，CLI 不自行复制路径规范化规则。

loader 只负责：

1. 从 `Source.LocalPath` 解码 Scope；
2. 把每个 import reference 交给 `Resolver`；
3. 以返回的 `Source.Key` 注册和去重 Scope；
4. 使用返回的 `Source.LocalPath` 读取内容；
5. 完成既有 Export、Projection、Relation 和 validation 语义。

loader 不解析 OCI reference、不读取 lockfile、不访问 Registry，也不根据物化目录推导远程 identity。Resolver 可以是执行网络安装的 resolver，也可以是只读取 lock 和已安装目录的 offline resolver。

### SourceKey

本地 Scope：

```text
file:///规范化绝对目录
```

OCI Package root Scope：

```text
oci://registry.example.com/locus/infra@sha256:<hex>
```

Package 内其他 Scope：

```text
oci://registry.example.com/locus/infra@sha256:<hex>#/relative/scope/path
```

fragment 使用 `/` 分隔并基于 Package 根目录规范化。Package 内相对 Import 解析后必须仍位于同一 Package 根目录；不得通过 `..` 逃逸。不同项目中的同一 digest 因此产生相同 `ScopeKey`，不会因 `.locus/packages` 的绝对位置改变 Entity identity。

循环 Import 继续合法。loader 必须在完成单个 Scope 自身解码后立即按 `Source.Key` 注册，再调用 Resolver 发现后续 Source。

## Package reference

OCI Package 直接使用完整 OCI reference：

```text
oci://registry.example.com/locus/infra:v1
oci://registry.example.com/locus/infra@sha256:<hex>
```

不带 tag 或 digest 的 reference 等价于 `:latest`，属于可变引用。规范化后的原始可变 reference 是 lockfile 的 key；解析结果必须是同一 registry/repository 下的 digest reference。

Digest reference 直接提供不可变 identity，不要求写入 lockfile。

## Lock

项目根固定为显式 `--scope` 或从当前目录向上发现得到的 root Scope 目录。lockfile 固定为：

```text
<root-scope>/locus.lock
```

格式：

```yaml
version: 1

packages:
  "oci://registry.example.com/locus/infra:v1":
    resolved: "oci://registry.example.com/locus/infra@sha256:abc..."
```

`locus.lock` 是 Source resolution snapshot，不复制 Scope dependency graph。内容按原始 reference 排序，使用稳定 YAML 编码和原子替换写入。

普通 install：

1. 可变 reference 已存在合法 lock entry：直接复用 digest，不重新查询 tag；
2. 可变 reference 缺少 lock entry：向 Registry 解析 digest 并加入 lock；
3. digest reference：直接使用，不加入 lock；
4. 完整 reachable Scope graph 安装并验证成功后，写入本次 graph 的 lock snapshot；
5. 不再 reachable 的旧 entry 从新 snapshot 删除。

`--frozen`：

- 所有 reachable 可变 reference 都必须已有合法 lock entry；
- 不得新增、删除或修改 lock entry；
- 可以按已锁定 digest 从 Registry 获取本地缺失内容；
- digest reference 不依赖 lock；
- 当前 graph 与 lock snapshot 不一致时失败，且不得改写 `locus.lock`。

lock 只在完整安装成功后提交。解析或安装失败时，原 lockfile 保持不变。

## Cache 与项目物化

全局 cache 使用用户级共享 OCI Image Layout，按 registry/repository 分隔：

```text
~/.locus/oci/<registry>/<repository>/
├── oci-layout
├── index.json
└── blobs/
```

Registry 和 repository 必须先经过 OCI reference parser 验证，再映射为 cache 路径。不得把未验证的远程字符串直接拼接到文件路径。

项目物化目录固定为：

```text
<root-scope>/.locus/packages/
├── sha256-abc.../
└── sha256-def.../
```

目录名由 digest 的 algorithm 和 encoded value 组成。结构扁平，不建立嵌套 dependency tree。`.locus/packages/` 是可重建产物，通常不提交版本控制。

```text
OCI Registry
    ↓
~/.locus/oci/<registry>/<repository>/   shared OCI layout
    ↓
<root>/.locus/packages/sha256-.../      project materialization
    ↓
Source{Key, LocalPath}
    ↓
scope.Load
```

## Install

`locus-pkg install` 支持：

```text
locus-pkg install [--scope <dir>] [--frozen] [--json]
```

- `--scope` 缺省时沿用最近 root Scope 发现规则；
- `--frozen` 使用上述只读 lock 策略；
- `--json` 输出稳定结构，供 Agent 和脚本消费；
- 默认文本输出只报告 root、解析数、复用数、获取数、物化数和最终验证结果。

安装使用 resolver-driven loading，而不建立第二套 manifest walker：

```text
确定 root Source
    ↓
读取现有 locus.lock
    ↓
scope.Load(root, installing Resolver)
    ↓
Resolver 遇到本地 reference
    → 规范化 SourceKey 与 LocalPath
    → Package 内执行边界检查
    ↓
Resolver 遇到 OCI reference
    → 复用 lock 或向 Registry 解析 digest
    → 获取缺失 artifact 到全局 OCI layout
    → 校验并物化到 .locus/packages/
    → 返回 digest SourceKey + LocalPath
    ↓
loader 继续发现 Package 内相对 imports 和跨 Package imports
    ↓
完整 Workspace validation
    ↓
原子写入 locus.lock
```

installing Resolver 可以访问网络并积累待提交 lock snapshot。`locus-scope` 使用 offline Resolver：只读取 lock 和 `.locus/packages`，不解析 tag、不下载内容；缺少 lock entry 或物化目录时给出运行 `locus-pkg install` 的明确诊断。

## Registry 边界

Locus 使用 OCI Distribution，不实现 Registry Server。客户端使用 ORAS-go；认证、manifest/blob 传输和 Registry 兼容由 ORAS/OCI 提供。Zot 是推荐自建实现，但不是运行时依赖；Harbor、Distribution、GHCR 和云厂商 OCI Registry 均可替代。

## 实施计划

### Source-aware loader

- 引入 `Source` 和 `Resolver`；
- 将 Scope 的稳定 Key 与读取目录分离；
- 迁移 `locus-scope`、测试和所有 `scope.Load` 调用方；
- 验证本地、Package 内相对引用、跨 Package 引用和循环 identity。

### Package installation

- 实现严格 lockfile codec 和状态转换；
- 使用 ORAS-go 解析 descriptor、复制到 OCI layout 并按 digest 获取；
- 验证 artifact manifest 和单一 tar+gzip layer；
- 安全解包并原子物化；
- 通过 installing Resolver 递归发现跨 Package imports。

### CLI integration

- 新增 `cmd/locus-pkg` 的 `install`；
- 实现文本、JSON 和 `--frozen`；
- 为 `locus-scope` 接入 offline Resolver，保持查询阶段无网络副作用。

### 验证

测试分层、fixture、OCI E2E、隔离规则和完成标准统一见 [测试设计](base-测试设计.md)，本文不重复维护。

## 核心不变量

- Package Infrastructure 不解释 Entity 或 Relation。
- Scope graph 的唯一来源仍是 `imports`。
- 每次跨 Scope 的 Projection/Export 语义不变。
- Package identity 使用不可变 digest。
- `ScopeKey` 不依赖当前物化路径。
- `locus.lock` 固化可变 reference resolution，不复制 dependency graph。
- `.locus/packages` 是当前项目可重建的扁平物化集合。
- 全局 cache 使用 OCI Image Layout，不定义第二套 blob store。
- `scope.Load` 只依赖 Source resolver，不直接理解 OCI、lock 或 Registry。
- 循环 Import 合法，安装和加载均按稳定 SourceKey 去重。
