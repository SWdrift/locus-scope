# Scripts

仓库脚本统一负责构建、测试、CLI 部署和本地 Zot。所有命令都从仓库根目录执行。

## 运行原则

- 默认 target 是仓库工作区，所有产物位于 `temp/`。
- 只有显式传入 `-User` 才会写入用户的 `~/.locus/`。
- 用户部署不修改 `PATH`；用户卸载不删除 OCI cache 或其他 `.locus` 状态。
- PowerShell 要求 7+，Go 最低版本以根目录 `go.mod` 为准。

## Scripts

| 脚本 | 默认行为 |
| --- | --- |
| `scripts/build.ps1` | 构建 `locus-scope` 和 `locus-pkg`。 |
| `scripts/deploy-local.ps1` | 构建并把两个 CLI 部署到当前 target。 |
| `scripts/clean-local.ps1` | 清理当前 target 的 CLI；按需同时卸载 Zot。 |
| `scripts/zot.ps1` | 安装、校验和管理当前 target 的 Zot。 |
| `scripts/test.ps1` | 使用仓库内 Zot 执行 Go 测试。 |
| `scripts/package-release.ps1` | 构建 Windows AMD64 发布制品和 Inno Setup 安装包。 |
| `scripts/install-user.ps1` | 使用已构建安装包为当前用户安装所选组件。 |
| `scripts/uninstall-user.ps1` | 卸载当前用户的 Locus 程序并默认保留用户数据。 |
| `scripts/sync-branches.ps1` | 将 `dev` 合并到 `main` 和 `master`，分别推送到 Gitee 和 GitHub，最后切回 `dev`。 |

## Target

| Target | 选择方式 | CLI | Zot |
| --- | --- | --- | --- |
| 仓库工作区 | 默认 | `temp/local/bin/` | `temp/zot/` |
| 当前用户 | `-User` | `~/.locus/bin/` | `~/.locus/zot/` |

`-UserLocusRoot <path>` 可覆盖用户 Locus 根目录，但必须与 `-User` 一起使用，且路径末级必须是 `.locus`。同一操作链中的 deploy、clean 和 Zot 命令必须选择同一个 target。

## 部署与卸载

| 命令 | 效果 |
| --- | --- |
| `pwsh -File scripts/deploy-local.ps1` | 构建并部署到 `temp/local/bin/`。 |
| `pwsh -File scripts/deploy-local.ps1 -User` | 构建并部署到 `~/.locus/bin/`。 |
| `pwsh -File scripts/deploy-local.ps1 -WithZot` | 部署工作区 CLI，并安装工作区 Zot。 |
| `pwsh -File scripts/deploy-local.ps1 -User -WithZot` | 部署用户 CLI，并安装用户 Zot。 |
| `pwsh -File scripts/clean-local.ps1` | 删除 `temp/build/` 和 `temp/local/`，保留 Zot。 |
| `pwsh -File scripts/clean-local.ps1 -WithZot` | 在默认清理基础上停止并删除 `temp/zot/`。 |
| `pwsh -File scripts/clean-local.ps1 -User` | 只删除用户目录中的两个 CLI。 |
| `pwsh -File scripts/clean-local.ps1 -User -WithZot` | 删除用户 CLI，并停止、删除用户 Zot。 |

`deploy-local.ps1` 的 `-WithZot` 只安装 Zot，不启动服务。配合 `-ZotBinary <path>` 可从已有且 SHA-256 匹配的 Zot 二进制离线安装。用户 clean 只删除 `locus-scope`、`locus-pkg` 和显式选择的 Zot，不触碰 `~/.locus/oci`。

## Zot

Zot 固定监听 `127.0.0.1:18080`，同一时间只能运行一个 target 的实例。默认从固定 GitHub Release 下载，并校验版本和 SHA-256。

```powershell
pwsh -File scripts/zot.ps1 <action> [-User]
```

| Action | 行为 |
| --- | --- |
| `install` | 安装二进制并生成配置；`-InstallSource <path>` 可改用本地二进制。 |
| `verify` | 校验二进制版本、SHA-256 和配置。 |
| `start` | 后台启动并等待 `/readyz` 与 `/v2/` 可用。 |
| `status` | 检查记录的进程和 Registry endpoint；省略 action 时执行此项。 |
| `stop` | 停止由当前 target 管理的后台进程。 |
| `serve` | 前台运行，直接输出日志。 |
| `uninstall` | 停止服务并删除该 target 的二进制、配置、日志和 Registry 数据。 |

## 构建与测试

| 命令 | 行为 |
| --- | --- |
| `pwsh -File scripts/build.ps1` | 按 `go env GOOS/GOARCH` 构建，使用 `-trimpath`。 |
| `pwsh -File scripts/test.ps1 all` | 使用 `-count=1` 执行 `go test ./...`。 |
| `pwsh -File scripts/test.ps1 e2e` | 使用 `-count=1` 执行 `go test ./test/e2e`。 |

构建产物位于：

```text
temp/build/<goos>-<goarch>/
├── locus-scope[.exe]
└── locus-pkg[.exe]
```

测试前需先运行 `pwsh -File scripts/zot.ps1 install`。测试脚本复用已运行的工作区 Zot，否则临时启动它；同时设置 `LOCUS_TEST_REGISTRY=http://127.0.0.1:18080`，结束时只停止由本次测试启动的实例。`temp/e2e-run/` 保留可复现现场。

## Windows 发布制品

生成带两个 CLI 的压缩包、独立 Zot 和 Windows 安装包：

```powershell
pwsh -File scripts/package-release.ps1
```

命令从仓库根目录 `VERSION` 读取发布版本；显式 `-Version` 仅用于校验且必须与该文件一致。命令固定构建 `windows/amd64`，下载并校验仓库锁定版本的 Zot，然后调用 Inno Setup 6 的 `ISCC.exe`。`ISCC.exe` 可位于 `PATH` 或 Inno Setup 标准安装目录，也可通过 `-IsccPath <path>` 或 `ISCC_PATH` 指定。离线构建可通过 `-ZotBinary <path>` 使用 SHA-256 匹配的现有 Zot 二进制。

最终制品位于：

```text
temp/release/windows-amd64/
├── locus-windows-amd64.zip
├── zot-windows-amd64.exe
├── locus-setup-windows-amd64.exe
└── SHA256SUMS
```

安装包包含两个可选 CLI、可选 Zot、Zot 用户级管理脚本和分发许可证，不在安装时访问网络。安装根目录固定为 `~/.locus/`；可选任务负责配置当前用户 `PATH` 和 Zot 登录自启动。

本机安装和卸载冒烟测试：

```powershell
pwsh -File scripts/install-user.ps1
pwsh -File scripts/uninstall-user.ps1
```

`install-user.ps1` 默认静默安装两个 CLI 和 Zot、添加当前用户 `PATH`，但不启用 Zot 登录自启动。可用 `-Components scope,pkg` 选择组件、`-NoPath` 禁止修改 `PATH`、`-ZotAutoStart` 启用 Zot 登录自启动，或用 `-Interactive` 显示安装向导；`-SetupPath` 可指定其他安装包。

`uninstall-user.ps1` 默认静默卸载并保留 Zot registry 和 OCI cache；`-Interactive` 显示卸载确认及“删除 Zot 仓库数据”选项，`-UninstallerPath` 可指定其他卸载器。两个脚本会修改真实当前用户安装状态，仅用于明确的本机安装验证，日志分别写入 `temp/install-user.log` 和 `temp/uninstall-user.log`。

## 常用工作流

仓库内开发：

```powershell
pwsh -File scripts/deploy-local.ps1 -WithZot
pwsh -File scripts/zot.ps1 start
pwsh -File scripts/test.ps1 all
pwsh -File scripts/clean-local.ps1 -WithZot
```

同步发布分支（要求工作区干净；远端目标分支发生分叉时会停止，不自动改写历史）：

```powershell
pwsh -File scripts/sync-branches.ps1
```

用户安装与卸载：

```powershell
pwsh -File scripts/deploy-local.ps1 -User -WithZot
pwsh -File scripts/zot.ps1 start -User
pwsh -File scripts/clean-local.ps1 -User -WithZot
```
