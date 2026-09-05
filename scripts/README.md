# Scripts

仓库脚本统一负责构建、测试、CLI 部署、project-local Verdaccio 和发布打包。所有命令都从仓库根目录执行。
脚本名统一为 `<domain>-<action>.ps1`，使同一领域的命令在目录中自然聚合。


## 运行原则

- 默认 target 是仓库工作区，临时文件和构建产物只写入 `temp/`。
- 只有显式传入 `-User` 的部署/清理脚本才会写入当前用户的 `~/.locus/`。
- 脚本不创建仓库或用户 `.npmrc`，也不通过参数、环境变量或配置把 npm/pnpm cache 或 store 重定向到仓库。
- `scripts/test-run.ps1` 不启动、停止或读取开发 Registry；E2E 测试拥有自己的隔离 Registry 子进程。
- PowerShell 要求 7+，Go 最低版本以根目录 `go.mod` 为准，Node.js 要求 20.6+，pnpm 版本由根 `package.json` 固定。

## 脚本索引

| 脚本 | 默认行为 |
| --- | --- |
| `scripts/branch-sync.ps1` | 将 `dev` 合并到发布分支并推送，最后切回 `dev`。 |
| `scripts/inno-setup.ps1` | 管理 project-local Inno Setup 编译器。 |
| `scripts/local-build.ps1` | 构建独立 `locus-scope` 和 `locus-pkg`。 |
| `scripts/local-clean.ps1` | 清理当前 target 的独立 CLI。 |
| `scripts/local-deploy.ps1` | 构建并把两个独立 CLI 部署到当前 target。 |
| `scripts/npm-registry.ps1` | 安装依赖并管理 project-local Verdaccio。 |
| `scripts/release-package.ps1` | 生成 Windows 独立制品和六个 npm Package tarball。 |
| `scripts/test-run.ps1` | 运行 Go 与 Node suites；Registry 生命周期由测试自身负责。 |
| `scripts/user-install.ps1` | 使用已构建安装包为当前用户安装所选独立 CLI。 |
| `scripts/user-uninstall.ps1` | 卸载当前用户的独立 CLI 并保留用户项目数据。 |

## 构建与本地部署

```powershell
pwsh -File scripts/local-build.ps1
pwsh -File scripts/local-deploy.ps1
```

构建产物位于：

```text
temp/build/<goos>-<goarch>/
├── locus-scope[.exe]
└── locus-pkg[.exe]
```

默认部署目标是 `temp/local/bin/`。仅在明确需要用户级部署时使用：

```powershell
pwsh -File scripts/local-deploy.ps1 -User
pwsh -File scripts/local-clean.ps1 -User
```

`-UserLocusRoot <path>` 可覆盖用户 Locus 根目录，但必须与 `-User` 同用，且路径末级必须是 `.locus`。用户清理只删除两个 CLI，不触碰项目或其他用户数据。仓库 target 清理删除 `temp/build/` 和 `temp/local/`：

```powershell
pwsh -File scripts/local-clean.ps1
```

## Project-local Verdaccio

Verdaccio 版本由根 `package.json` 与 `pnpm-lock.yaml` 固定，监听 `http://127.0.0.1:4873/`。匿名读取允许，发布必须认证。可复用策略位于 `scripts/internal/verdaccio.dev.yaml`，运行时会把 storage、htpasswd 与日志的绝对路径物化到 `temp/verdaccio-dev/config.yaml`。

```powershell
pwsh -File scripts/npm-registry.ps1 <action>
```

| Action | 行为 |
| --- | --- |
| `install` | 在仓库根运行 `pnpm install --frozen-lockfile`，并初始化 temp-only state。 |
| `start` | 启动 project-local Verdaccio，记录进程信息并等待 `/-/ping`。 |
| `status` | 同时验证 PID、可执行文件、命令行/config ownership 和 endpoint。 |
| `stop` | 只停止通过上述 ownership 校验的进程；已停止时幂等成功。 |
| `logs` | 输出 Registry、stdout 和 stderr 日志；`-Follow` 持续跟随。 |
| `reset` | 先停止 owned process，再删除 `temp/verdaccio-dev/`；必须传 `-Force`。 |

完整开发循环：

```powershell
pwsh -File scripts/npm-registry.ps1 install
pwsh -File scripts/npm-registry.ps1 start
pwsh -File scripts/npm-registry.ps1 status
pwsh -File scripts/npm-registry.ps1 logs
pwsh -File scripts/npm-registry.ps1 stop
pwsh -File scripts/npm-registry.ps1 reset -Force
```

脚本永远不会写 `.npmrc`。需要测试认证时，显式把 package manager userconfig 放在 state root 中：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
$env:NPM_CONFIG_REGISTRY = 'http://127.0.0.1:4873/'
```

这是调用者选择的隔离配置；`npm-registry.ps1` 本身不会创建或修改它。不要设置 `NPM_CONFIG_CACHE`、`PNPM_HOME` 或 `pnpm config set store-dir` 指向仓库。

## 测试

先安装根 Node dependencies：

```powershell
pwsh -File scripts/npm-registry.ps1 install
```

然后运行：

```powershell
pwsh -File scripts/test-run.ps1 e2e
pwsh -File scripts/test-run.ps1 all
```

- `e2e` 执行 `go test ./test/e2e -count=1`。E2E 自己创建、验证并停止隔离 Verdaccio，全部状态留在 `temp/e2e-run/npm/`。
- `all` 执行 `go test ./... -count=1`（包含 E2E）和 `pnpm run test:node`。

测试脚本只检查根 Node dependencies 是否已安装，不读取 `temp/verdaccio-dev/`，也不接管开发者已运行的服务。

## Project-local Inno Setup

Inno Setup 默认以 portable 模式安装到仓库 `temp/tools/inno-6.7.3/`，编译器固定为 `temp/tools/inno-6.7.3/ISCC.exe`。下载的固定版本 installer 缓存在 `temp/tools/innosetup-6.7.3.exe`，安装前校验 SHA-256 和 Authenticode 签名。

```powershell
pwsh -File scripts/inno-setup.ps1 install
pwsh -File scripts/inno-setup.ps1 status
pwsh -File scripts/inno-setup.ps1 path
pwsh -File scripts/inno-setup.ps1 reset -Force
```

`install` 幂等安装或验证固定版本；`status` 验证 managed manifest 与 `ISCC.exe` 内容；`path` 只输出已验证的编译器绝对路径；`reset -Force` 删除安装目录但保留已验证的 installer cache，便于离线重装。全部安装、日志和缓存状态都留在项目 `temp/tools/`。

`release-package.ps1` 默认调用该脚本的 `path` action，不搜索系统 `PATH` 或全局安装。只有显式传入 `-IsccPath` 或设置 `ISCC_PATH` 时才覆盖 managed compiler。

## Windows 安装包

生成发布制品：

```powershell
pwsh -File scripts/release-package.ps1
```

脚本从根 `VERSION` 读取版本；可选 `-Version` 只能用于一致性校验。默认编译器由 `scripts/inno-setup.ps1` 管理并位于 `temp/tools/inno-6.7.3/ISCC.exe`；首次构建前运行其 `install` action。交付源集中在 `packaging/`：npm package 位于 `packaging/npm/`，Windows installer 定义位于 `packaging/windows/`。
两个 Windows binary 与 license 先暂存到 `temp/release-stage/windows-amd64/`；最终独立制品位于：

```text
temp/release/windows-amd64/
├── locus-windows-amd64.zip
├── locus-setup-windows-amd64.exe
└── SHA256SUMS
```

安装包只包含 `locus-scope`、`locus-pkg` 和 Locus license。它不分发或管理 Registry，不在安装时联网。安装根固定为 `%USERPROFILE%\.locus`，组件可独立选择，可选任务只负责当前用户 `PATH`。

安装/卸载冒烟入口：

```powershell
pwsh -File scripts/user-install.ps1
pwsh -File scripts/user-uninstall.ps1
```

`user-install.ps1` 默认静默安装两个 CLI 并添加用户 `PATH`；使用 `-Components scope` 或 `-Components pkg` 选择单个组件，`-NoPath` 禁止修改 `PATH`，`-Interactive` 显示安装向导。`user-uninstall.ps1` 默认静默卸载程序并保留用户数据。两个脚本只用于明确的当前用户安装验证，日志写入 `temp/install-user.log` 和 `temp/uninstall-user.log`。

## npm Package 发布制品

同一个 release 命令还会把 `locus-scope-node-host` 交叉构建到五个 platform Package，并打包 `@locus/scope`：

```text
temp/release/npm/
├── stage/
│   ├── locus-scope/
│   ├── locus-scope-win32-x64/
│   ├── locus-scope-linux-x64/
│   ├── locus-scope-linux-arm64/
│   ├── locus-scope-darwin-x64/
│   └── locus-scope-darwin-arm64/
├── locus-scope-<version>.tgz
├── locus-scope-<platform>-<version>.tgz
└── SHA256SUMS
```

所有 staged manifest 的版本都同步为根 `VERSION`，`@locus/scope` 的五个 optional dependency pins 同步为完全相同的版本。脚本使用 `pnpm pack` 生成标准 tarball，但不会向任何公共 Registry 发布。

## 分支同步

要求工作区干净；远端目标分支分叉时停止，不自动改写历史：

```powershell
pwsh -File scripts/branch-sync.ps1
```
