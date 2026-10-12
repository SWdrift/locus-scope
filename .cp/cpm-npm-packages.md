# npm Package Memory

## 稳定决策

- npm/pnpm 是唯一 package manager，负责解析、lock、integrity、缓存、安装、pack、publish 与认证。
- Node adapter 从真实安装环境产生 importer-relative resolved Environment；Go 不重新验证 npm package-name、SemVer 或 lock semantics。
- Environment 包含 rootScope、rootDependencies 和以 `npm:<name>@<exact-version>` 为 key 的 PackageDescriptor。
- Go packageenv 只验证 edge target、entry containment/readability、Scope Import 与 identity-to-source 唯一性。
- `@sundw/locus-scope` 暴露唯一 executable `locus-scope`，通过 optional platform package 携带私有 Go host。
- Node host 使用单次 versioned JSON request/response，不引入 RPC 或第二套命令实现。
- 发布和消费均使用 npm/pnpm 原生命令；项目不维护自有 lock/store/cache/materialization。
