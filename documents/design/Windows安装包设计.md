# Windows安装包设计

## 简述

Windows 安装包使用 Inno Setup 6 生成单个 `locus-setup-windows-amd64.exe`，以当前用户身份将可选组件安装到 `%USERPROFILE%\.locus`。本文确定安装包相关的源码目录、发布暂存目录、最终制品目录和用户安装目录。

## 职责

本文负责 Windows 安装包的目录布局，以及 `locus-pkg`、`locus-scope`、Zot 和 Zot 生命周期管理脚本在这些目录中的归属。不负责 Package artifact 格式、Scope 协议、具体 Inno Setup 实现、代码签名凭据或发布平台配置。

- Package 和用户级 OCI cache 的语义以 [Package设计](Package设计.md) 为准。
- 所有暂存和生成制品必须位于仓库 `temp/` 下。

## 仓库目录

采用以下结构：

```text
installer/
└── windows/
    ├── locus.iss
    └── assets/
        └── zot-license.txt

scripts/
├── build.ps1
├── package-release.ps1
└── internal/
    ├── locus-paths.ps1
    ├── locus-zot-user.ps1
    └── zot-release.psd1
```

职责如下：

- `installer/windows/locus.iss`：Inno Setup 主安装脚本。
- `installer/windows/assets/zot-license.txt`：随 Zot 分发的第三方许可证。
- `scripts/build.ps1`：构建 `locus-pkg.exe` 和 `locus-scope.exe`。
- `scripts/package-release.ps1`：组装压缩包、独立 Zot 和 Windows 安装包。
- `scripts/internal/locus-zot-user.ps1`：安装后供开始菜单快捷方式调用的 Zot `start`、`stop` 和 `status` 生命周期管理脚本，并维护当前用户的登录自启动计划任务；它不属于公共 CLI，也不加入 `PATH`。
- `scripts/internal/zot-release.psd1`：集中记录 Zot 的固定版本、发布资产、下载地址和 SHA-256，供开发部署与制品构建共同使用。

## 发布目录

发布过程在 `temp/` 下生成暂存内容和最终制品，`release-stage` 是安装器和压缩包的唯一组装输入；`release` 只保存可以直接发布的制品及其校验和：

```text
temp/
├── release-stage/
│   └── windows-amd64/
│       ├── bin/
│       │   ├── locus-pkg.exe
│       │   └── locus-scope.exe
│       ├── zot/
│       │   └── zot.exe
│       ├── libexec/
│       │   └── locus-zot-user.ps1
│       └── licenses/
│           ├── locus-license.txt
│           └── zot-license.txt
└── release/
    └── windows-amd64/
        ├── locus-windows-amd64.zip
        ├── zot-windows-amd64.exe
        ├── locus-setup-windows-amd64.exe
        └── SHA256SUMS
```

## 用户安装目录

安装根目录固定为 `%USERPROFILE%\.locus`：

```text
%USERPROFILE%\.locus\
├── bin\
│   ├── locus-pkg.exe
│   └── locus-scope.exe
├── libexec\
│   └── locus-zot-user.ps1
├── zot\
│   ├── bin\
│   │   └── zot.exe
│   ├── config.json
│   ├── zot.pid
│   ├── logs\
│   └── registry\
├── licenses\
│   ├── locus-license.txt
│   └── zot-license.txt
├── oci\
└── installer\
    └── unins000.exe
```

目录所有权和卸载边界：

- `bin/` 保存用户选择安装的两个公共 CLI；只有该目录可以选择加入当前用户 `PATH`。
- `libexec/` 保存安装器内部辅助程序，不构成公共命令接口。
- `zot/bin/zot.exe` 和 Zot 管理入口属于 Zot 安装组件。
- `zot/config.json`、`zot.pid`、`zot/logs/` 和 `zot/registry/` 是 Zot 配置或运行状态。
- `licenses/` 保存 Locus 和已安装第三方组件的分发许可证。
- `oci/` 是 `locus-pkg` 的用户级 OCI cache，不属于安装器静态文件。
- `installer/` 保存卸载器，避免安装根目录散落安装器内部文件。

卸载时停止 Zot 并删除安装器拥有的程序文件。Zot 仓库数据默认保留；卸载界面以默认不勾选的“删除 Zot 仓库数据”选项允许用户显式删除 `zot/registry/`。`oci/` 始终作为用户数据保留，除非未来另行设计明确的数据删除入口。

## 开始菜单入口

安装 Zot 组件后创建以下开始菜单入口：

```text
Locus/
├── 启动 Zot
├── 停止 Zot
├── 查看 Zot 状态
├── 打开 Zot 日志目录
└── 卸载 Locus
```

前三个入口调用 `libexec/locus-zot-user.ps1`。安装器可提供默认不勾选的“登录 Windows 后自动启动 Zot”选项；该选项使用当前用户计划任务，不安装需要管理员权限的 Windows Service。用户手动停止 Zot 后，本次登录期间不自动重启。

本地 Zot 安装并启动后监听 `127.0.0.1:18080`。