# Module Development Guide

本文档记录内置模块开发约定，尤其是会通过 `gg module copy <name>` 复制到业务项目中的模块。

## Design 与内置模块契约必须一致

可复制模块需要同时支持两种使用方式：

- `gg module add <name>` 使用框架内置的 `module.Register` / `module.Use` / `module.NewWrapper` 注册模型、服务和路由。
- `gg module copy <name>` 先复制 `internal/model/<name>` 的 DSL `Design()`，再通过 `gg gen` 生成业务项目本地的 router、service shell，最后合并框架侧 service 业务代码。

因此，模块的 DSL `Design()` 必须完整表达内置模块的对外契约，不能只为了生成 service 文件写一个近似定义。

必须保持一致的内容：

- 路由路径必须一致。`Design()` 中的 `Route(...)` 要和内置 module 的 `Route()` 或 `module.NewWrapper(...)` 路径一致，包括单复数、连字符、下划线和是否带业务前缀。
- action 集合必须一致。内置 module 注册了哪些 phase，`Design()` 就应该声明哪些 action；没有真实语义的 action 不要为了默认 CRUD 随手声明。
- public/auth 语义必须一致。内置 module 的 `Pub()` 或 `module.NewWrapper(..., pub, ...)` 是 public 时，`Design()` 对应 action 必须写 `Public(true)`；需要登录的 action 不写 `Public(true)`。
- request/response 契约必须一致。自定义请求、响应类型要通过 `Payload[T]()`、`Result[T]()` 写进 `Design()`，避免 copy 后生成默认模型签名。
- service 文件目标必须一致。存在自定义 service 代码的 action 必须写 `Service(true)`；如果多个 action 共用一个 service 文件，所有相关 action 都要写相同的 `Filename(...)`。

修改模块时要同时检查四处：

- `internal/model/<name>` 的 `Design()`。
- `module/<name>` 的注册代码和 wrapper。
- `internal/service/<name>` 中真实存在的 service 文件和 helper 文件。
- `module/<name>/module.json` 中的 copy manifest。

目标是保证同一个模块通过 `gg module add` 使用，和通过 `gg module copy` 后再 `gg gen` 使用，得到相同的路由、鉴权和 service 行为。

## Copy manifest

如果模块复制需要跳过框架源文件、复制中间件或输出复制后的接入提示，在模块目录放置 `module.json`：

```json
{
  "copy": {
    "excludeSourceFiles": [
      "internal/model/authz/button.go"
    ],
    "middleware": [
      {
        "sourceFile": "middleware/authz.go",
        "scope": "auth",
        "handler": "Authz"
      }
    ],
    "postNotes": [
      "Create a project-owned adapter outside service/mfa."
    ]
  }
}
```

- `excludeSourceFiles` 使用 framework root 相对路径，表示 `gg module copy` 不复制这些源文件，也不让它们参与 model/action 规划。
- `middleware[].sourceFile` 只能指向 framework `middleware/*.go` 源文件，目标固定复制到项目 `middleware/` 下的同名文件。
- `middleware[].scope` 只能是 `global` 或 `auth`，分别对应 `middleware.Register(...)` 和 `middleware.RegisterAuth(...)`。
- `middleware[].handler` 是 `sourceFile` 中返回 gin handler 的零参函数名，例如 `Authz`。
- `postNotes` 只在复制成功后输出，用于提示项目侧必须补齐的 adapter、配置或初始化步骤。

## Service 文件边界

### 公用函数不要放在具体 action service 文件中

开发可复制模块时，多个 service action 共享的函数、类型、常量不要放在某一个具体 action 的 service 文件里，这类代码应放到独立 helper 文件中。

原因是 `gg module copy` 会根据 action service 文件的依赖关系复制 helper 文件。如果一个未被当前 DSL `Service(true)` 声明的 action service 文件里放了公用函数，它可能会被当成 helper 复制到业务项目；随后 `gg gen --prune` 又会因为当前模型没有对应 service target，把这个文件识别成应清理的 service 文件。

简单规则：

- action service 文件只放该 action 自己的 service struct、action 方法、hook 方法和强绑定该 action 的私有逻辑。
- 多个 action 共享的逻辑必须放到非 action service 文件中。
- 复制模块前要确认 `gg module copy <name>` 产出的 helper files 不包含未被 DSL action 声明的 action service 文件。
