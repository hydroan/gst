# internal/ggmodule 开发注意事项

`internal/ggmodule` 是 `gg module` 命令的框架侧实现，不属于任何一个具体业务 module。
修改这个包时，必须保持它和真实内置 module 解耦。

## 边界规则

- `internal/ggmodule` 的代码和测试不应依赖真实的 `module/authz`、`module/mfa`、`module/logmgmt`、`module/helloworld`、`module/version` 等业务 module。
- 测试不应引用真实的 `internal/model/<module>` 或 `internal/service/<module>` 业务实现。
- 需要 module、model、service、middleware fixture 时，在测试临时目录中创建 fake framework。
- fake framework 根目录应模拟 `internal/gst/go.mod`，module path 使用 `github.com/hydroan/gst`。
- fixture 名称使用中性命名，例如 `copytest`、`aliased`、`configured`、`modelcopytest`、`servicecopytest`。
- 不要用真实业务名作为“方便的样例”，否则后续业务重构会误伤 `ggmodule` 测试。

## 测试组织

- `add`、`remove`、`catalog` 相关测试只验证 module 命令语义，应使用 fake `module/<name>/register.go`。
- 可添加 module 使用无参 `Register()`。
- 不可添加 module 使用带参数 `Register(...)`。
- 包名和目录名不一致时，使用 fake package alias 测试导入别名逻辑。
- `copy` 相关测试应在临时 fake framework 中同时创建：
  - `module/<name>/module.json`
  - `internal/model/<name>`
  - `internal/service/<name>`
  - 必要时创建 `middleware`
- 不要通过 symlink 或路径指向仓库中的真实 module 作为 copy source。
- 辅助函数应放在对应测试文件或 `test_helpers_test.go`，并保持 fixture 语义中性。
