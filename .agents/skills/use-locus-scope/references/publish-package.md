# Publish Locus Scope Package

## 前置校验

确认 `package.json` 包含规范 name、exact version 与 package 内 `locus.entry`。安装项目依赖后运行：

```sh
node <skill-dir>/scripts/validate-package.mjs .
pnpm exec locus-scope validate --json
```

## 打包

在仓库 `temp/` 下指定输出目录，使用 npm 原生命令：

```sh
npm pack --json --ignore-scripts --pack-destination ./temp/npm-pack
```

检查 packed files：必须包含 `package.json`、`locus.entry` 与定义文件；不得包含凭据、`.npmrc`、`.git/` 或 `node_modules/`。记录 npm 返回的 name、version、filename 与 integrity。

## 发布

只发布刚检查过的确切 tarball：

```sh
npm publish ./temp/npm-pack/<name>-<version>.tgz --access public
```

认证只通过 npm userconfig 或环境提供。发布后读取 Registry metadata，确认 exact `name@version` 的 `dist.integrity` 与本地 pack 一致。禁止重新从目录隐式打包后发布。
