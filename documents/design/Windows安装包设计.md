# Windows安装包设计

## 简述

Windows 安装包使用 Inno Setup 6 生成 `locus-setup-windows-amd64.exe`，以当前用户身份把 standalone `locus-pkg.exe` 和 `locus-scope.exe` 安装到 `%USERPROFILE%\.locus`。Registry server、Node adapter 和 npm platform host 不属于该安装包。

## 职责

本文定义 Windows standalone CLI 的源码输入、发布暂存、最终制品、用户安装目录、PATH 和卸载边界。Package/Registry 语义见[Package设计](Package设计.md)。所有暂存和生成制品必须位于仓库 `temp/`。

## 仓库与发布目录

```text
packaging/
└── windows/
    └── locus.iss

scripts/
├── inno-setup.ps1
├── local-build.ps1
└── release-package.ps1

temp/
├── tools/
│   └── inno-6.7.3/
│       └── ISCC.exe
├── release-stage/
│   └── windows-amd64/
│       ├── bin/
│       │   ├── locus-pkg.exe
│       │   └── locus-scope.exe
│       └── licenses/
│           └── locus-license.txt
└── release/
    ├── windows-amd64/
    │   ├── locus-windows-amd64.zip
    │   ├── locus-setup-windows-amd64.exe
    │   └── SHA256SUMS
    └── npm/
        └── package tarballs
```

`inno-setup.ps1 install` 下载并验证固定版本 installer，以 portable 模式把 Inno Setup 默认安装到项目 `temp/tools/inno-6.7.3/`；不建立全局安装或系统卸载项。`release-package.ps1` 默认通过该管理脚本取得已验证的 `ISCC.exe`，只有显式 `-IsccPath` 或 `ISCC_PATH` 才覆盖它；随后从根 `VERSION` 构建两个 standalone CLI、Windows zip、installer 和 checksums，同时把 `locus-scope-node-host` 交叉构建到 `packaging/npm/` 的五个平台 npm package staging 目录并用 `pnpm pack` 生成 `@sundw/locus-scope` 及 platform tarball。npm tarball 是独立发布面，不进入 Windows installer。

## 用户安装目录

```text
%USERPROFILE%\.locus\
├── bin\
│   ├── locus-pkg.exe
│   └── locus-scope.exe
├── licenses\
│   └── locus-license.txt
└── installer\
    └── unins000.exe
```

- `bin/` 是唯一可选择加入当前用户 `PATH` 的目录。
- installer 只拥有上述程序、license、快捷方式、PATH entry 和卸载器。
- `.locus/cache`、`.locus/packages` 和 `locus.lock` 是项目本地 Pure Locus 状态，不属于用户安装目录。
- npm Registry（包括开发 Verdaccio）由项目或用户独立管理，安装器不分发、不启动、不停止、不卸载 Registry server。
- 卸载删除安装器拥有的程序文件、快捷方式和 PATH entry，不扫描或删除任何项目的 package/cache/lock 数据。

## 组件与开始菜单

安装器只有 `scope` 与 `pkg` 两个可选组件；默认安装两者。开始菜单只提供 CLI 入口所需快捷方式和“卸载 Locus”，不创建 Registry 生命周期入口、计划任务或登录自启动项。

## 验收

- zip 与 installer 只包含两个 standalone CLI 和 Locus license，不包含 Registry server、Node host 或 Registry 配置。
- 安装后两个 CLI 的 `version` 输出等于根 `VERSION`。
- 可选 PATH 修改只涉及 `%USERPROFILE%\.locus\bin`，卸载准确移除自己的 entry。
- 无 Registry server component、binary option、Registry data deletion option、计划任务或第三方 Registry license。
