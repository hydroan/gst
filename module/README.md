# 模块开发指南

本文档记录内置模块开发约定，尤其是会通过 `gg module copy <name>` 复制到业务项目中的模块。

`module/<name>` 表示一套可复用的通用业务能力，不是单个 model、service 或 middleware 文件的简单集合。一个模块可以由 model、service、middleware、cronjob、provider/adapter、复制清单和接入说明共同组成；这些内容作为整体实现同一个业务域的对外契约。



## 核心规则

- 同一个模块通过 `gg module add <name>` 使用，和通过 `gg module copy <name>` 后再 `gg gen` 使用，必须得到相同的数据库表、路由、中间件、和 service 行为。
- 修改模块时必须同时检查 model、service、middleware、路由、module 注册、复制清单、测试和文档，避免 add 路径正常但 copy 路径缺文件、缺 adapter、缺 middleware。



## Design 契约

可复制模块需要同时支持两种使用方式：

- `gg module add <name>` 使用框架内置的 `module.Register` / `module.Use` / `module.NewWrapper` 注册模型、服务和路由。
- `gg module copy <name>` 先复制 `internal/model/<name>` 的 DSL `Design()`，再通过 `gg gen` 生成业务项目本地的 router、service shell，最后合并框架侧 service 业务代码。

因此，模块的 DSL `Design()` 必须完整表达内置模块的对外契约，不能只为了生成 service 文件写一个近似定义。

必须保持一致的内容：

- 路由路径必须一致。`Design()` 中的 `Route(...)` 要和内置 module 的 `Route()` 或 `module.NewWrapper(...)` 路径一致，包括单复数、连字符、下划线和是否带业务前缀。
- action 集合必须一致。内置 module 注册了哪些 phase，`Design()` 就应该声明哪些 action；没有真实语义的 action 不要为了默认 CRUD 随手声明。
- exact 路由语义必须一致。内置 module 使用 `module.Exact(...)` 原样注册路径时，`Design()` 对应 action 必须写 `Exact()`，避免 `gg gen` 追加默认 CRUD 后缀。
- public/auth 语义必须一致。内置 module 的 `Pub()` 或 `module.NewWrapper(..., pub, ...)` 是 public 时，`Design()` 对应 action 必须写 `Public()`；需要登录的 action 不写 `Public()`。
- request/response 契约必须一致。自定义请求、响应类型要通过 `Payload[T]()`、`Result[T]()` 写进 `Design()`，避免 copy 后生成默认模型签名。
- service 文件目标必须一致。存在自定义 service 代码的 action 必须写 `Service(true)`；如果多个 action 共用一个 service 文件，所有相关 action 都要写相同的 `Filename(...)`。
- middleware 注册必须一致。内置 module 如果调用 `middleware.Register(...)` 或 `middleware.RegisterAuth(...)`，对应 middleware 源文件、作用域和 handler 必须写进 copy manifest，避免 `gg module copy` 后少挂全局或鉴权中间件。



## Service 文件边界

**公用辅助代码不要放进具体 action service 文件**

开发可复制模块时，多个 service action 共享的函数、类型、常量不要放在某一个具体 action 的 service 文件里，这类代码应放到独立 helper 文件中。

原因是 `gg module copy` 会根据 action service 文件的依赖关系复制 helper 文件。如果一个未被当前 DSL `Service(true)` 声明的 action service 文件里放了公用函数，它可能会被当成 helper 复制到业务项目；随后 `gg gen --prune` 又会因为当前模型没有对应 service target，把这个文件识别成应清理的 service 文件。

简单规则：

- action service 文件只放该 action 自身的 service struct、action 方法、hook 方法和强绑定该 action 的私有逻辑。
- 多个 action 共享的逻辑必须放到非 action service 文件中。
- provider/adapter 接口可以放在 service helper 文件中，但具体宿主 adapter 不要放在 `internal/service/<name>`。
- 复制模块前要确认 `gg module copy <name>` 产出的辅助文件列表不包含未被 DSL action 声明的 action service 文件。



## Provider 和 Adapter 边界

当模块需要宿主项目提供某种能力时，不要让 `internal/service/<name>` 直接 import 具体业务 model 或基础设施实现。模块 service 只依赖自己定义的最小接口，注册层或项目侧 adapter 负责把宿主实现转换成模块需要的抽象对象。

以 `module/mfa` 为例：

- `internal/service/mfa` 定义 `AccountAuthenticator` 和 `AuthenticatedAccount`，MFA 业务只关心稳定账号 ID 和可选用户名。
- `internal/service/mfa` 的 TOTP check、unbind 等流程通过 `currentAccountAuthenticator()` 完成账号认证，不 import `internal/model/iam/user`。
- `module/mfa/iam_account_authenticator.go` 是框架内置 IAM adapter，负责查询 IAM user、校验密码和状态，再转换成 `servicemfa.AuthenticatedAccount`。
- `module/mfa/register.go` 安装内置 adapter；业务项目通过 `gg module copy mfa` 后，应在项目自己的接入文件里调用 `servicemfa.SetAccountAuthenticator(...)`。
- `module/mfa/module.json` 的 `postNotes` 必须提示 copy 后需要补齐项目自有 adapter。

设计新的 provider/adapter 边界时遵守这些规则：

- service 接口只暴露模块真实需要的数据，不透传宿主项目的完整领域模型或基础设施对象。
- adapter 命名应该表达被适配的宿主能力，service 接口命名应该表达模块需要的能力；不要在通用文档中规定固定命名模板。
- service 层必须提供安全默认实现。没有安装 adapter 时应返回明确配置错误，不应 panic，也不应悄悄放行安全流程。
- adapter 负责调用宿主项目能力并做错误归一；service 负责模块自身状态和业务规则。
- 如果 copy 后项目必须实现 adapter，需要在 `module.json` 的 `postNotes` 中写清楚接入入口和必要步骤，不规定具体文件名。



## 复制清单

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
      "在项目接入层安装模块所需的 adapter。"
    ]
  }
}
```

- `excludeSourceFiles` 使用 framework root 相对路径，表示 `gg module copy` 不复制这些源文件，也不让它们参与 model/action 规划。
- `middleware[].sourceFile` 只能指向 framework `middleware/*.go` 源文件，目标固定复制到项目 `middleware/` 下的同名文件。
- `middleware[].scope` 只能是 `global` 或 `auth`，分别对应 `middleware.Register(...)` 和 `middleware.RegisterAuth(...)`。
- `middleware[].handler` 是 `sourceFile` 中返回 gin handler 的零参函数名，例如 `Authz`。
- `postNotes` 只在复制成功后输出，用于提示项目侧必须补齐的 adapter、配置或初始化步骤。
