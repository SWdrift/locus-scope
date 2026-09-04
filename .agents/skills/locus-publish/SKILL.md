---
name: locus-publish
description: "发布 Locus Scope Package 到 OCI Registry。用于用户要求发布、推送、分发或更新 Scope Package，解释 publish、tag、digest、Registry 认证或本地 Zot 发布流程时。"
metadata:
  domain: locus-scope
---

# Locus Scope · 发布 Package

## 目标

把一个本地 root Scope 的 source tree 发布为 OCI 1.1 artifact，并保留命令返回的不可变 manifest digest。发布由 `locus-pkg` 完成；`locus-scope` 只负责加载、验证和查询 Workspace。

## 发布前检查

1. 确认待发布目录包含且只包含一个 Scope manifest：`locus.yaml`、`locus.yml` 或 `locus.json`。
2. 若待发布 source tree 含 OCI Import，先在 root Scope 运行 `locus-pkg install`，再运行 `locus-scope validate`；没有 OCI Import 时直接运行 `locus-scope validate`。这是完整 reachable graph 的发布前检查；`publish` 自身只检查本地 Package source tree、Package 边界和 OCI reference 语法，不获取并验证远端依赖图。
3. 检查 Package 边界：包内本地 Import 必须使用 `/` 分隔的相对路径，且不能逃出 Package root；跨 Package Import 使用 `oci://` reference。
4. 选择可写的 OCI Registry repository 和 tag。发布目标必须是无 fragment 的 tag reference；不能使用 digest reference。省略 tag 等价于 `:latest`，但正式发布优先使用明确 tag。
5. 私有 Registry 的登录和凭据沿用 Docker config 与 credential helper。不要把用户名、密码或 token 写入 Scope 文件、命令示例或仓库。

## 发布命令

显式指定 Scope：

```text
locus-pkg --scope ./infra publish oci://registry.example.com/locus/infra:v1
```

也可以先进入 Scope 目录，让 CLI 从当前目录向父目录发现最近的 manifest：

```text
locus-pkg publish oci://registry.example.com/locus/infra:v1
```

供 Agent 或脚本消费时使用稳定 JSON：

```text
locus-pkg --json --scope ./infra publish oci://registry.example.com/locus/infra:v1
```

成功结果必须记录规范化后的 `target` 和不可变 `digest`。不要把 tag 当作 Package identity。

## 本地 Zot

Windows 安装包选择 Zot 后，可从开始菜单启动服务；仓库开发环境使用：

```powershell
pwsh -File scripts/deploy-local.ps1 -User -WithZot
pwsh -File scripts/zot.ps1 start -User
```

然后发布到仅本机可访问的 Registry：

```text
locus-pkg --scope . publish oci://localhost:18080/locus/my-scope:v1
```

`localhost:18080` 只适合本机开发。跨机器分发必须使用其他机器可访问的 OCI Registry。

## 发布语义

- artifact 包含 root Scope source tree；`locus.lock` 和所有 `.locus` 目录不会进入 artifact。
- 相同内容和相关文件 mode 重复发布得到相同 digest。
- 内容变化后再次发布同一 tag，tag 会指向新 digest；旧 digest identity 不变。
- 已安装项目的 `locus.lock` 固定原 digest，不会因 tag 更新自动漂移。
- Locus 使用 OCI Distribution 与 ORAS 生态，不实现 Registry Server、账号系统、SemVer、版本范围或 dependency solver。
- 仅 `localhost` 和环回 IP 自动使用 HTTP；其他 Registry 使用 HTTPS。
- 发布和安装都不执行 provisioning、reconciliation 或实际部署漂移检查。

## 完成检查

- 发布命令成功并返回 target 与 manifest digest。
- target 是预期 Registry、repository 和 tag。
- digest 使用 `sha256:<hex>` 形式；后续沟通、审计和复现优先引用 digest。
- 若需要验证消费者路径，在独立 Scope 中声明该 OCI reference，运行 `locus-pkg install`，再运行 `locus-scope validate`；不要用同一待发布目录伪装消费端。
