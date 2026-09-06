# Scripts

根 `package.json` 的 package scripts 是仓库任务的唯一公共入口；所有命令都从仓库根目录通过 `pnpm run` 执行。`scripts/` 只保存入口背后的实现，不作为需要记忆的第二套命令接口。

## 目录职责

```text
scripts/
├── build.mjs
├── clean.mjs
├── deploy.mjs
├── registry.mjs
├── test.mjs
├── branch-sync.mjs
├── config/
│   ├── remark.config.mjs
│   └── verdaccio.dev.yaml
├── lib/
│   └── workspace.mjs
└── windows/
    ├── inno-setup.ps1
    ├── release.ps1
    ├── user-install.ps1
    └── user-uninstall.ps1
```

- 根目录 `.mjs` 编排跨平台仓库任务。
- `lib/` 只保存多个任务共享的工作区边界、进程执行和路径安全能力。
- `config/` 保存仓库任务使用的工具配置和静态配置。
- `windows/` 保存 Inno Setup 和当前用户安装等 Windows 专用实现。
- 下载工具、运行状态、fixtures 和构建产物一律写入 `temp/`。
- 工具依赖及版本由根 `package.json` 和 `pnpm-lock.yaml` 管理。

## 运行原则

- 默认 target 是仓库工作区。
- 只有显式运行 `deploy:user`、`clean:user`、`user:install` 或 `user:uninstall` 才会写入当前用户的 `~/.locus/`。
- 脚本不创建仓库或用户 `.npmrc`，也不通过参数、环境变量或配置把 npm/pnpm cache 或 store 重定向到仓库。
- `pnpm test` 不启动、停止或读取开发 Registry；E2E 测试拥有自己的隔离 Registry 子进程。
- Node.js 要求 20.6+，pnpm 版本由根 `package.json` 固定；Windows 专用任务要求 PowerShell 7+。

## 命令索引

| 命令 | 行为 |
| --- | --- |
| `pnpm run build` | 构建当前平台的独立 `locus-scope` 和 `locus-pkg`。 |
| `pnpm run clean` | 清理工作区内的 build 和 local deployment。 |
| `pnpm run deploy` | 构建并部署两个 CLI 到 `temp/local/bin/`。 |
| `pnpm run deploy:user` | 构建并部署两个 CLI 到当前用户的 `~/.locus/bin/`。 |
| `pnpm test` | 运行完整 Go 与 Node suites。 |
| `pnpm run test:e2e` | 只运行 E2E suite。 |
| `pnpm run check` | 运行仓库静态检查。 |
| `pnpm run release` | 生成 Windows 独立制品和六个 npm Package tarball。 |
| `pnpm run publish:npm` | 按平台包优先、主包最后的顺序发布六个 npm Package tarball。 |
| `pnpm run registry:start` | 启动 project-local Verdaccio。 |
| `pnpm run registry:status` | 验证 Registry 进程 ownership 和 endpoint。 |
| `pnpm run registry:logs` | 输出当前 Registry 日志。 |
| `pnpm run registry:logs:follow` | 持续跟随当前 Registry 日志。 |
| `pnpm run registry:stop` | 停止 owned Registry 进程。 |
| `pnpm run registry:reset` | 停止 Registry 并删除 `temp/verdaccio-dev/`。 |
| `pnpm run inno:install` | 安装或验证 project-local Inno Setup。 |
| `pnpm run inno:status` | 验证 managed Inno Setup。 |
| `pnpm run inno:reset` | 删除 managed Inno Setup，保留 installer cache。 |
| `pnpm run user:install` | 使用已构建安装包为当前用户安装 CLI。 |
| `pnpm run user:uninstall` | 卸载当前用户 CLI，保留项目数据。 |
| `pnpm run branch:sync` | 将 `dev` 合并并推送到两个发布分支。 |

依赖安装使用 package manager 的标准命令，不再提供脚本包装：

```powershell
pnpm install --frozen-lockfile
```

## 构建与本地部署

```powershell
pnpm run build
pnpm run deploy
```

构建产物位于：

```text
temp/build/<goos>-<goarch>/
├── locus-scope[.exe]
└── locus-pkg[.exe]
```

默认部署目标是 `temp/local/bin/`。用户级部署和清理必须使用显式入口：

```powershell
pnpm run deploy:user
pnpm run clean:user
```

需要覆盖用户 Locus 根目录时：

```powershell
pnpm run deploy:user -- --user-locus-root X:\path\.locus
pnpm run clean:user -- --user-locus-root X:\path\.locus
```

覆盖路径末级必须是 `.locus`。用户清理只删除两个 CLI，不触碰项目或其他用户数据。

## Project-local Verdaccio

Verdaccio 固定监听 `http://127.0.0.1:4873/`。匿名读取允许，发布必须认证。可复用策略位于 `scripts/config/verdaccio.dev.yaml`，运行时配置、storage、htpasswd、进程记录和日志位于 `temp/verdaccio-dev/`。

```powershell
pnpm install --frozen-lockfile
pnpm run registry:start
pnpm run registry:status
pnpm run registry:logs
pnpm run registry:stop
pnpm run registry:reset
```

脚本永远不会写 `.npmrc`。需要测试认证时，显式把 package manager userconfig 放在 Registry state root 中：

```powershell
$env:NPM_CONFIG_USERCONFIG = Join-Path $PWD 'temp\verdaccio-dev\userconfig'
npm adduser --auth-type=legacy --registry http://127.0.0.1:4873/
$env:NPM_CONFIG_REGISTRY = 'http://127.0.0.1:4873/'
```

不要设置 `NPM_CONFIG_CACHE`、`PNPM_HOME` 或 `pnpm config set store-dir` 指向仓库。

## 测试

```powershell
pnpm run test:e2e
pnpm test
```

- `test:e2e` 执行 `go test ./test/e2e -count=1`。
- `test` 执行 `go test ./... -count=1` 和 Node package tests。
- E2E 自己创建、验证并停止隔离 Verdaccio，全部状态保留在 `temp/e2e-run/npm/`。

## Project-local Inno Setup

Inno Setup 以 portable 模式安装到 `temp/tools/inno-6.7.3/`。installer cache、安装日志和 managed manifest 也都位于 `temp/tools/`。

```powershell
pnpm run inno:install
pnpm run inno:status
pnpm run inno:path
pnpm run inno:reset
```

`release` 默认使用该 managed compiler；只有显式向 Windows 实现传入 `-IsccPath` 或设置 `ISCC_PATH` 时才覆盖。

## 发布制品

```powershell
pnpm run release
```

根 `VERSION` 是 standalone 与六个 npm package 的版本真相。Windows 制品位于：

```text
temp/release/windows-amd64/
├── locus-windows-amd64.zip
├── locus-setup-windows-amd64.exe
└── SHA256SUMS
```

npm tarball、stage 和校验和位于 `temp/release/npm/`。发布任务只生成制品，不向公共 Registry 发布。

生成制品后运行 `pnpm run publish:npm`，默认向 `https://registry.npmjs.org/` 按平台包优先、主包最后的顺序发布六个公开 Package。附加参数会传给 `pnpm publish`；需要验证其他 Registry 时可显式覆盖，例如 `pnpm run publish:npm -- --tag next --registry http://127.0.0.1:4873/`。

当前用户安装验证使用：

```powershell
pnpm run user:install
pnpm run user:uninstall
```

## 分支同步

`pnpm run branch:sync` 要求工作区干净；远端目标分支分叉时停止，不自动改写历史，结束后返回 `dev`。
