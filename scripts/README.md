# Scripts

本目录提供仓库统一的测试、构建、本机部署和本地 Zot Registry 管理入口。所有命令都从仓库根目录执行，生成内容只写入 `temp/`。

## 环境要求

- PowerShell 7，命令名为 `pwsh`。
- Go 1.26。
- 首次安装 Zot 时可访问 GitHub Release；测试场景不会访问外部服务，缺失的 Go module 仍由 Go 按机器级缓存和代理配置获取。

## Zot Registry

`scripts/zot.ps1` 管理固定版本的本地 Zot。服务只监听 `127.0.0.1:18080`，二进制、配置、日志和 Registry 数据保存在 `temp/zot/`。

首次安装：

```powershell
pwsh -File scripts/zot.ps1 install
```

校验二进制版本、SHA-256 和生成的 Zot 配置：

```powershell
pwsh -File scripts/zot.ps1 verify
```

管理后台服务：

```powershell
pwsh -File scripts/zot.ps1 start
pwsh -File scripts/zot.ps1 status
pwsh -File scripts/zot.ps1 stop
```

以前台方式运行，适合直接查看日志：

```powershell
pwsh -File scripts/zot.ps1 serve
```

不传动作时默认执行 `status`。`start` 会等待 `/readyz` 和 `/v2/` 可用后返回；后台日志写入 `temp/zot/logs/`。

## 测试

首次运行测试前先安装 Zot：

```powershell
pwsh -File scripts/zot.ps1 install
```

执行全部测试，包括使用外部 Zot 的 E2E 场景：

```powershell
pwsh -File scripts/test.ps1 all
```

仅执行 `test/e2e`：

```powershell
pwsh -File scripts/test.ps1 e2e
```

不传测试范围时默认执行 `all`。测试脚本会：

1. 复用已经运行的 Zot，或启动已安装的 Zot。
2. 将 `LOCUS_TEST_REGISTRY` 临时设置为 `http://127.0.0.1:18080`，确保外部 Zot 用例不会被跳过。
3. 使用 `-count=1` 执行测试，避免 Go 测试缓存掩盖结果。
4. 仅停止由本次测试启动的 Zot，不影响测试前已经运行的实例。
5. 保留 `temp/e2e-run/` 下的测试现场，供后续复现。

## 构建

编译 `locus-scope` 和 `locus-pkg`：

```powershell
pwsh -File scripts/build.ps1
```

目标平台取自 `go env GOOS` 和 `go env GOARCH`。每次构建会确定性替换对应目录：

```text
temp/build/<goos>-<goarch>/
├── locus-scope[.exe]
└── locus-pkg[.exe]
```

构建使用 `-trimpath`，不会把工作区绝对路径写入可执行文件。

## 本机部署

构建当前目标平台并把两个 CLI 部署到仓库内的本机运行目录：

```powershell
pwsh -File scripts/deploy-local.ps1
```

部署位置：

```text
temp/local/bin/
├── locus-scope[.exe]
└── locus-pkg[.exe]
```

例如在 Windows 上运行部署结果：

```powershell
.\temp\local\bin\locus-scope.exe help
.\temp\local\bin\locus-pkg.exe help
```

该脚本不修改系统 `PATH`，不写入用户目录，也不启动 Zot。

## 清除本机产物

删除构建产物和本机部署：

```powershell
pwsh -File scripts/clean-local.ps1
```

该脚本只删除：

- `temp/build/`
- `temp/local/`

它不会删除 `temp/zot/`、`temp/e2e-run/` 或其他测试现场。Zot 进程通过 `pwsh -File scripts/zot.ps1 stop` 单独停止。
