# Scope Management Memory

## 契约决策

- 两套公开 CLI 共享 `internal/scopecli` 的 command/application surface；Node adapter 只负责发现 npm package environment、定位平台 Go host 与转发参数/stdin。加载完成后的 help、参数解析、JSON shaping、错误输出和退出码不得在 JavaScript 侧复制。
- 文档权责固定为：`ERRORS.md` 定义 application error 模型、分类和传播规则，`CLI.md` 引用该模型并定义指令、参数、输出与退出状态。CLI 的命令表直接承载各指令行为；共享引用、筛选和写入规则只定义一次，不再另设重复的命令解释区。
- Entity 与 Relation 均为 root-open object。Entity 以 `(scope,id)` 定位；Relation 以 resolved `(from,type,to)` 定位并要求 Workspace 内唯一。查询结果使用稳定 envelope，owner 元数据以 `@` 前缀隔离。
- Filter 只遍历 object/map；`=`、`!=` 使用 JSON 语义相等，字符串操作符只接受 string，字段缺失时 predicate 永不匹配。Graph 的 `--via` 复用同一 Filter 实现。
- provenance 在 decode/load 阶段形成，至少包含声明 Scope identity 与 Scope-relative definition path；line/index 只描述当前快照，不参与 identity。mutation 必须依 provenance 修改原声明，不得按对象值反查文件。
- mutation 只允许 root Scope，dependency Scope 只读。写入采用原 codec 的规范化整文件重写，先在内存 clone 上修改和验证，再原子替换并 reload；验证失败必须恢复原字节。
- Graph Adapter 在 import/projection 完成后构建 directed multigraph，隔离 Gonum 类型。最短路径和同 hop 平行边均以稳定排序决胜，公共 JSON 不泄露 Gonum。
- `diff` 将 `scope:`、`group:`、`path:` 两侧解析为归一化语义子图，只比较 Entity object 与 Relation object；单 definition 文件使用隔离 synthetic Scope，禁止解析文件外 endpoint。
- Node host protocol v2 在 request 中显式携带用户 stdin；Node adapter 只在命令含 stdin operand `-` 时消费 stdin，避免无条件阻塞。

## 实现坑点

- Relation declaration 只接受 `{from,type,to,...}` 对象；loader、mutation 和 writer 均不得保留 tuple 兼容分支，避免形成长期双格式协议。
- Group 内 Entity 的公开 id 必须保持规范全 id；落盘时才转换为 declaration-local id。否则查询 identity、Relation endpoint 与 mutation 定位会分叉。
- `path:` diff 可能完全脱离当前 root，因此共享 runner 必须支持命令级延迟加载；不能要求所有命令预先拥有当前 Workspace。
- 公共 CLI 参数错误退出 `2`，加载、校验、查询、权限、冲突和事务错误退出 `1`。分类必须来自 typed application error，不得由 transport 匹配错误文本。
