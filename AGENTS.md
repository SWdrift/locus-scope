# AGENTS.md

- 文档系统见 [`documents\design\README.md`](documents\design\README.md)。

## 安全

- 所有读取、写入、生成文件、子进程工作目录、fixtures 和测试状态都必须保留在本仓库工作区内。
- pnpm、npm 等 Node 包管理器缓存不属于工作区产物，必须沿用机器级全局配置；不得通过脚本、环境变量或配置文件把 store/cache 重定向到仓库内（包括 `temp/.pnpm-store`、`temp/.npm-cache`、`internal/temp/`）。
- 测试不得读取或修改工作区外的用户配置、凭据、服务、网络状态或文件。

## 约定

- 在仓库根目录运行命令。
- 临时文件和构建产物只能写入 `temp/`。
- internal\web\ui 可使用 `pnpm --dir internal\web\ui run format` 格式化。
- 使用 `pnpm`。

## 品味

- 轻量、直接、可读、可验证。
- 优先复用成熟基础设施，用最少的抽象解决真实问题；只为已知的扩展方向保留适度接口，不预支复杂度。
- 强调一致性：优先遵循项目既有的命名、结构、接口和实现模式，不并行引入第二套约定。
- 仅添加和执行必要测试；测试可观察行为、边界和不变量，不测试基础库或无分支的简单函数。
- 小段逻辑默认内联。只有当逻辑形成稳定、可复用、可命名的独立概念时才提取函数；不要为了“看起来干净”制造大量没有独立语义的小函数，适当接受重复编码。
- 编码、修改、评审和重构时遵循 [`.agents\skills\code-smell-guard\SKILL.md`](.agents\skills\code-smell-guard\SKILL.md)，只处理本次变更新增或加重的高置信坏味道，避免机械式重构。

## 测试

- 测试使用的观测数据存储必须显式重定向到测试工作区内的路径。
- 可复用的 end-to-end 声明和模拟设备状态必须存放在 `test/e2e/case/` 下；测试运行时将其具现化到 `temp/e2e-run/` 下。
- 测试结束后，`temp/e2e-run/` 下生成的 fixtures、二进制文件、SQLite 状态和解析结果必须保留，以便手动复现；新的测试运行可以用确定性方式替换该目录。

