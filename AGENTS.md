### 规则

1. 在编写任何代码前，先描述你的方法并等待批准
2. 如果我给出的需求模糊，请在编写代码前提出澄清问题
3. 完成任何代码编写后，列出边缘案例并建议覆盖它们的测试用例
5. 出现bug时，先编写能重现该bug的测试，再修复直到测试通过
6. 每次我纠正你时，反思你做错了什么，并制定永不再犯的计划
7. 如果需求没有完全理解，请继续向我提问，直到完全清楚需求了才开始写代码！

### 代码规范

- 代码风格、测试用例风格、程序输出风格等必须和当前包中保持一致.
- 修改完代码后, 需要同时检查下相关的文档/注释是否需要更新.
- 代码新增功能、bug修复完后, 需要更新其对应的函数、结构体、接口、类的代码注释和文档、代码注释请使用英文
- module, internal/model 中使用自定义 Request 时, 命名偏向于 XXXReq, 例如 SignupReq，Response 命名偏向于 XXXRsp, 例如 SignupRsp
- 总是按照最佳实践方式来实现代码、代码注释需要符合 golang 规范；新需求代码需要有足够的注释；如果发现现有注释有问题或不符合代码逻辑也需要优化注释。
- 对于不直接走框架默认 CRUD 资源模型的自定义接口，service 的 model 优先使用空模型；不同接口之间禁止复用 REQ、RSP，即使字段完全相同，也必须为当前接口单独定义自己的 REQ、RSP。
- 代码中的结构体、变量、常量、类型别名等定义需要按功能归类摆放；同一功能域的内容必须相邻放在一起，必要时使用空行分隔不同功能组，避免同类定义被无关内容打散。
- 新增测试用例之前，检查下是否有现有测试用例满足要求，如果满足则修改现有测试用例，如果不满足才新增测试用例
- 如果让你 review 代码，你只需要 review 暂存区的代码。



### moudle 开发规范

开发 module 时，每个接口对应的【model/REQ/RSP】、【业务逻辑】必须写在自己对应的单独代码文件中，禁止将多个接口的【model/REQ/RSP】写在同一个 model 代码文件中，禁止将多个不同接口的【业务逻辑】写在同一个 service 代码文件中。三种场景如下：

- 完全不同的业务逻辑和接口：/api/users，/api/groups，那么需要两个 model 文件和两个 service 文件
- 同一资源对象则走框架提供的 curd：POST /api/configs、DELETE /api/configs/:id、 DELETE /api/configs、PUT /api/configs/:id、PATCH /api/configs/:id、GET /api/configs、GET /api/configs/:id，只需要一个 model 文件且 model 文件中没有自定义 REQ 和 RSP，service 文件中只有一个结构体，在结构体上加上不同的 hooks。
- 同一资源对象走自定义业务逻辑：GET /api/iam/sessions、DELETE /api/iam/sessions/:id。还是只需要一个 model 文件和一个 service 文件，但是都有自己的 REQ、RSP service结构体：
  - model 代码文件中的结构体：`SessionsListReq`、`SessionsListRsp`、`SessionsDeleteReq`、`SessionsDeleteRsp`。
  - service 结构体方法：
    `func (s *SessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.SessionsListReq) (rsp *modeliamsession.SessionsListRsp, err error)`、
    `func (s *SessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionsDeleteReq) (rsp *modeliamsession.SessionsDeleteRsp, err error)`

module 包中的接口测试用例规范：

- 测试文件名名要符合子 moudle 名，例如 module/iam/session_test.go 就是专门用来存放 session 相关接口的测试用例，其对应的接口实现放在 internal/{model,service}/session 目录中。
- 测试组织方式要改成一个接口对应一个顶层测试函数，各个顶层测试函数应该尽量避免相互影响。
- 如果同一个接口有多种场景，则在这个接口对应的测试函数里 用 t.Run(...) 做子测试，如果只有一个场景，则不需要额外使用 t.Run(...) 来运行子测试。
- 测试用到的辅助函数应该放在其对应的测试文件中，例如 session 子模块相关的测试辅助函数应该放在 session_test.go 中，account 子模块相关的测试辅助函数应该放在 account_test.go 中。并且测试用例使用到的辅助函数尽量放在顶层测试函数之后。



### Sandbox

sandbox 中切换目录时，必须使用 `builtin cd` 而不是 `cd`

### 开发中

- 修改完 `cmd/gg`、`internal/codegen`、`dsl` 包的代码后，需要及时安装最新版本的 `gg` 命令：

```
go install ./cmd/gg
```

- 优先使用的包: 错误处理使用 `github.com/cockroachdb/errors `而不是 golang 内置的 errors 包.
- 禁止在 gst 项目根目录执行 `cmd/gg` 的任何命令\*\* - 很容易破坏当前项目代码。测试 `cmd/gg` 命令请到 `examples/demo` 项目目录下执行。
- 修改了代码，当前代码如果有测试用例，必须确保测试用例通过
- internal/model 不需要使用 dsl 来定义接口行为
- 禁止操作 git 暂存区、提交等操作，这就是这类命令禁止操作：`git add`, `git restore`, `git commit`，
    如果你发现暂存区、代码提交发生变化这是正常的，因为我在和你并行操作。你只需要关注代码变动即可。



### commit 

如果我让你给出 git commit 建议，你给出的 git commit 建议必须符合如下规则：

- 需要根据代码变更内容给出一个或多个 git commit，当修改的代码内容涉及多个主题时则需要多个 git commit。
- 如果给出多个 git commit，则需要提供每个 commit 对应的代码文件。
- commit 必须符合 conventional commit 规范
- 只查看暂存区的代码变更。
- commit 内容必须是英文。
- 给出的 commit 内容的 title 和 body 需要放在一起，方便我直接复制



### 开发完后

- 必须执行 `make check` 确保代码检查能通过。如果没有修改代码，例如只修改了 Makefile、Markdown 等文件则不需要执行 `make check`



## 其他

### 生成 skill

```bash
repomix \
  --include "**/*.go,go.mod,go.sum,Makefile,*.md,**/*.md,**/*.ini" \
  --ignore "**/*.log,**/.env*,**/*secret*,**/*credential*,**/*key*,**/vendor/**,**/tmp/**,**/.git/**,**/node_modules/**,**/dist/**,**/coverage/**,**/testdata/**,**/testcode/**,examples/**" \
  --skill-generate gst \
  --skill-output ~/.codex/skills/gst \
  --force
```

### README.md

README.md 面向使用 gst 框架的后端开发者，应保持简洁并聚焦实际使用流程；不要写入内部实现、维护者决策或开发者不需要关心的细节，但必须保留会影响正确使用的关键信息、命令顺序和风险提醒。

### 如何使用当前框架

可以结合 `examples/demo` 理解框架用法。这里描述的是后端项目如何使用 gst，不是 gst 框架源码本身的开发流程。

#### 基本流程

1. 在 gst 仓库执行 `make install` 安装 `gg` 命令。
2. 使用 `gg new myproject` 创建后端项目。
3. 在业务项目中修改或新增 `model` 文件，例如 `model/user.go`、`model/config/file.go`。
4. 修改 model 的 DSL 后执行 `gg gen` 生成 `main.go`、`model/model.go`、`service/service.go`、`router/router.go` 等注册代码。
5. 在对应的 `service` 文件中实现业务逻辑和复杂 hook。
6. 如果 model 的 `Design()` 中声明了 `Migrate(true)`，该 model 也是数据库模型；数据库字段变化后使用 `gg migrate --dry-run` 预览迁移，再用 `gg migrate` 按确认执行 schema 迁移。
7. 服务启动后会自动生成 Swagger 文档，访问路径是 `/docs/index.html`。

#### 生成文件和手写文件的边界

- `main.go`、`model/model.go`、`service/service.go`、`router/router.go` 通常由 `gg gen` 生成，主要负责导入包和注册 model、service、router。除非明确要修改生成器，否则不要手写这些文件。
- `model/**/*.go` 是接口和数据模型声明层。这里定义结构体字段、轻量级 model hook、`Design()` DSL、`Migrate(true)`、`Endpoint()`、`Param()`、`Route()`、`Payload()`、`Result()`、`Public()` 等接口行为。
- `service/**/*.go` 是业务实现层。这里实现 `Create`、`Delete`、`Update`、`Patch`、`List`、`Get`、`DeleteMany` 等方法，以及 `CreateBefore`、`ListAfter`、`Filter`、`FilterRaw` 等复杂 hook。
- `module/` 用来注册内置或自定义模块，例如 `iam.Register(...)`。
- `configx/`、`cronjob/`、`middleware/` 分别用于扩展配置、定时任务和中间件，应用入口通过空导入触发它们的 `init()`。

#### model 设计规则

- 普通数据库资源使用 `model.Base`，通常在 `Design()` 中声明 `Migrate(true)`、`Endpoint(...)` 和 `Param(...)`。
- 不落数据库、只表示动作或自定义接口的模型优先使用 `model.Empty`，例如登录、刷新 token、文件加密、批量处理等接口。
- 默认 CRUD 资源优先交给框架处理：在 `Design()` 中启用对应动作即可。如果没有额外业务逻辑，使用 `Service(false)` 或不声明 service。
- 需要自定义业务逻辑时，在对应动作中声明 `Service(true)`，然后在同名 service 子目录中实现对应 phase 的 service 结构体。
- 自定义接口必须为当前接口单独定义 `XXXReq`、`XXXRsp`，即使字段和其他接口完全相同也不要复用。请求和响应类型通过 `Payload[*XXXReq]()`、`Result[*XXXRsp]()` 绑定到 DSL。
- 同一资源的嵌套路由或额外动作使用 `Route(...)` 包裹，例如 `/config/files/encrypt`、`/messages/batch` 这类非默认 CRUD 路由。

#### service 实现规则

- service 结构体通常嵌入 `service.Base[M, REQ, RSP]`，其中 `M` 是 model，`REQ` 是请求类型，`RSP` 是响应类型。
- service 类型按 phase 命名，例如 `Creator`、`Lister`、`Getter`、`Updater`、`Patcher`、`Deleter`、`ManyDeleter`。注册时在生成的 `service/service.go` 中映射到 `consts.PHASE_CREATE`、`consts.PHASE_LIST` 等 phase。
- 查询和写库优先使用 `database.Database[T](ctx.DatabaseContext())`，并按需要组合 `WithQuery`、`WithSelect`、`WithPagination`、`WithOrder`、`WithLimit` 等框架能力。
- 列表过滤优先实现 `Filter(ctx, model)` 或 `FilterRaw(ctx)`；返回数据补充、关联查询、字段填充等逻辑优先放在 `ListAfter`。
- 简单字段校验、默认值、哈希计算等贴近模型生命周期的逻辑可以放在 model hook，例如 `CreateBefore`、`UpdateBefore`；复杂业务编排放在 service。

#### 常见接口模式

- `model/conversation.go`：会话/对话类数据库资源 model，启用 CRUD，并通过 service hook 做当前用户过滤、返回字段补充、关联对象填充等逻辑。
- `model/common/common.go`：通用工具类接口，使用 `model.Empty` 定义非数据库动作，并为当前接口单独定义请求和响应。适合搜索结果去重、文件解析、批量转换等没有独立数据表的动作。
- `model/auth/login.go`：登录跳转类公开接口，使用 `model.Empty` 定义动作模型，在 DSL 中声明 `Public(true)`，service 返回登录地址、token、回调结果等响应。
- `model/config/namespace/file.go`：配置文件类数据库资源，同一个 model 可以同时提供默认资源路由和自定义嵌套路由；文件名、必填字段、文件大小、校验和等轻量逻辑放在 model hook。
- `model/config/namespace/file/encrypt.go`：文件动作类接口，加密、解密、复制、发布、格式化等动作使用空模型加独立 `XXXReq`、`XXXRsp`，业务逻辑放在对应 service。

#### 后端项目使用注意事项

- 在后端项目根目录执行 `gg gen`、`gg migrate` 等命令，不要在 gst 框架仓库根目录对业务项目执行生成命令。
- 修改 model 的 DSL、接口路径、REQ/RSP、service 开关后，需要重新执行 `gg gen`，并检查生成的 router 和 service 注册是否符合预期。
- 修改声明了 `Migrate(true)` 的数据库模型字段后，需要执行 `gg migrate --dry-run` 检查 `generated/migrate/<dbtype>/schema.sql` 和迁移计划，再用 `gg migrate` 按确认处理 schema 变化。
- 不要手写覆盖生成文件；如果生成结果不符合预期，优先修正 model DSL 或框架生成逻辑。
- 后端服务启动后，通过 `/docs/index.html` 检查生成的 Swagger 文档和接口路径。
- 后端项目自身如果已有测试，修改业务逻辑后需要运行对应测试；涉及框架仓库改动时仍按 gst 仓库要求执行 `make check`。
