### 设计哲学

gst 是强约定框架（Apple 风格），不是自由框架（Windows 风格）：面向业务开发者只暴露最小 API 面，把最佳实践内置成默认行为，而不是提供旋钮让使用者自行选择。新增或评审框架能力时按以下原则判断：

- 窄接口：能力入口只声明意图（做什么），实现策略（怎么做）全部由框架内部决定。设计新能力时先砍旋钮；被砍能力的真实需求出现后由框架统一演进，不预先给使用者留自由度。
- 约定进代码：命名规则、校验规则必须是单点实现、多处消费（运行时 bootstrap 与 gg 工具链共用同一份逻辑），禁止靠文档约定加人工遵守，也禁止在多个消费点各自实现一份。
- 最佳实践用代码达成，不靠使用方自觉。优先级从高到低：1) 框架代码直接内建，通过 API 设计让正确做法成为唯一顺手的做法，错误做法无从写出；2) 静态检查强制——gst 自身靠 make check 挂载的检查器，业务项目靠 gg check 规则；gg check 只检查业务项目，不检查框架自身。注释、文档、AGENTS.md 不承担约束职责，只解释已被前两层强制的机制；凡是要求开发者"记得做某事"的约定，必须落进前两层，落不进的就是设计缺陷。
- 可选能力走接口断言：少数模型才需要的能力用可选接口表达，实现即启用；不要膨胀必选接口。
- 可选即兜底，必须即报错：框架允许的每种使用形态（如接口默认实现所支持的省略）都是一等契约，框架内所有消费点必须兜底全部合法形态，不得要求使用方补配置来迁就框架实现；确实必须使用方提供的，在 bootstrap 或 gg 阶段直接报错强制，不留「文档约定 + 项目自觉」的中间态。判定契约以默认实现和存量用法为准，新增校验不得收窄既有契约。
- 公开包是 alias 转发层：实现下沉 internal 包，公开包只做转发，细节见「internal 包引用方向」。
- fail fast：配置或声明错误在 bootstrap 或 gg 命令阶段直接报错退出，禁止静默忽略、静默去重、静默改写。
- 能力缺失显式报错：某个后端或方言不支持框架的一项能力时，该能力的入口直接返回错误，禁止静默 no-op、静默降级或悄悄换实现模拟。唯一例外是该能力在此后端语义上自动成立、no-op 与真实执行不可区分的场景，此时允许 no-op 并必须在能力入口的文档写明。
- 主路优先：同一件事只保留一条官方路径，扩展能力只补主路够不到的盲区，不做与主路重叠的平行第二入口。
- 框架不悄悄决策：有风险的变更显式暴露给人评审（dry-run、报错提示），框架不自动执行变更决策。

### 开发

### 协作流程

1. 在编写任何代码前，先描述你的方法并等待批准
2. 如果我给出的需求模糊，请在编写代码前提出澄清问题
3. 完成任何代码编写后，列出边缘案例并建议覆盖它们的测试用例
5. 出现bug时，先编写能重现该bug的测试，再修复直到测试通过
6. 每次我纠正你时，反思你做错了什么，并制定永不再犯的计划
7. 如果需求没有完全理解，请继续向我提问，直到完全清楚需求了才开始写代码！



### 命名、注释和测试、优化

1. 极其重要：功能新增、重构、bug修复后：需要相应调整其对应的函数名、结构体名、接口名、测试用例名、文件名、文档、注释。
2. 代码中的结构体、接口、函数、变量、常量、类型别名等定义需要按功能归类摆放；同一功能域的内容必须相邻放在一起，必要时使用空行分隔不同功能组，避免同类定义被无关内容打散。
3. 代码风格、测试用例风格、程序输出风格等必须和当前包中保持一致。
4. 代码注释使用英文，注释必须符合 golang 规范、习惯。开发过程中发现现有注释有问题、过时或不符合代码逻辑的，即使与当前任务无关，也必须顺手修复。
5. 新增测试用例之前，检查下是否有现有测试用例满足要求，如果满足则修改现有测试用例，如果不满足才新增测试用例
6. 如果让你 review 代码，你只需要 review 暂存区的代码。
7. 优先使用的包: 错误处理使用 `github.com/cockroachdb/errors `而不是 golang 内置的 errors 包.
8. 禁止在测试用例、fixture、示例和注释中使用具体业务背景命名（如 cmdb/host、订单），必须使用中性命名（如 Sample、Record、Item）；开发过程中发现存量违规的必须顺手改正。
9. 我说对某些包做「代码质量优化」时，按下面执行；只说「优化 xxx 包」、没说清是质量优化还是功能改动的，先问我。
   1. 目标：可观测行为完全不变——执行路径、输入输出、错误信息、日志文案、导出 API 语义都与改前一致——只让代码更好读、更好找。
   2. 要做：文件内摆放与分组、包内跨文件搬运、文件拆分与合并、文件与符号重命名（含导出符号，需同步改掉全部引用点）、注释修正。测试代码一并处理：`_test.go` 文件名、测试函数名、子测试名、测试辅助函数、testdata。
   3. 不要做：增删功能、改分支/参数/返回值/并发/错误处理、增删测试用例或改断言、动依赖；发现 bug 也不顺手修，单独列清单给我。
   4. 收尾：`make check` 通过，且被优化包的测试在不改断言的前提下全绿，作为「没改逻辑」的证据。




### 时间基的唯一权威：UTC

框架内所有"时刻"（instant）一律以 UTC 产生、存储和序列化；换算成本地时区只属于展示层（前端）。服务端只做瞬间运算（比较、排序、范围过滤），禁止出现任何时区偏移换算——出现就是双重偏移或漏偏移的温床。

- 产生：会落库或进响应的时间值必须以 UTC 产生。框架管理的时间戳（Create/Upsert 强制写入、Update 刷新 updated_at、软删 deleted_at，后两者经各方言 `gorm.Config` 的 NowFunc）统一出自 `dbruntime.NowUTC`：UTC 且截断到毫秒，对齐存储精度，保证写入时返回的值与后续回读值完全一致。
- 存储：各方言连接统一 UTC 口径（MySQL DSN `loc=UTC` 等），同一瞬间在所有方言落同一 UTC 墙钟；理由见 mysql 包 buildDSN 的注释。
- 入口：URL 时间过滤只接受 RFC3339（自带时区，任意偏移都无损）；无时区字符串和 Unix 时间戳直接报错，不猜测。
- 日志：业务打点 ts 统一 UTC RFC3339Nano。
- 例外是"日历"语义（不是瞬间）：cron 表达式按进程本地墙钟解释（时区配置化是独立待办）；聚合时间分桶按存储的 UTC 墙钟切，按运营时区切日需在查询侧显式换算。

### 数据库列名的唯一权威

数据库列名一律由 `internal/modelschema` 解析，它底层调用 gorm 自己的 schema 解析，因此 `column` tag、`-`/`-:all`/`-:migration` 三种忽略标记、嵌入结构体展开、gorm 的 commonInitialisms 规则全部与运行时一致。

- 禁止在任何地方重新实现「gorm tag 解析 + snake case」推导列名，包括 controller、database 和生成器。
- URL 参数名与数据库列名是两件事：URL 参数名由 `query` tag → `json` tag → 字段名推导（前后端契约），列名只来自 gorm。`Column.QueryName` 与 `Column.DBName` 分别承载它们，不要混用。
- `json:"-"` 的字段仍是数据库列，但不可被客户端过滤（`Column.Filterable` 为 false），软删除时间戳属于这一类。

### internal 包引用方向

公开包 `model`、`service`、`types` 是面向业务项目的 alias 转发层，分别转发 `internal/modelregistry`、`internal/serviceregistry`、`internal/sse`。internal 包（含测试）需要这些能力时必须直接引用对应的 internal 包，禁止反向 import 公开包，避免 internal → 公开 → internal 的依赖绕行和潜在 import 环。如果所需符号只存在于公开包（如曾经的 `service.Error`），应把实现下沉到 internal 包、公开包改为 alias 转发，而不是让 internal 反向引用。

例外，以下内容必须保持公开包 import：

- module 源码 `internal/model/<module>`、`internal/service/<module>`：会被 `gg copy` 复制进业务项目，copy 只改写 module 子树自身的 internal import 前缀，公开包 import 原样保留；若引用其他 internal 包，复制后无法编译。module 的 `_test.go` 不参与 copy，可以引用 internal 包。
- `internal/codegen`、`internal/ggmodule` 中面向生成代码的 import 路径常量、模板和 testdata：属于生成到业务项目里的用户代码，必须写公开路径。



### 结束收尾

1. 修改完 `dsl`、`cmd/gg`、`internal/codegen`、 `internal/ggmodule` 包的代码后，需要及时安装最新版本的 `gg` 工具
3. 开发完后：必须执行 `make check` 确保代码检查能通过。如果没有修改代码，例如只修改了 Makefile、Markdown 等和代码无关的文件则不需要执行 `make check`



### 测试用例原则

- 测试应优先断言目标行为存在，例如新 module、新 import、新 route、新生成文件。
- 除非业务契约明确要求拒绝旧行为，否则不要写“旧路径不存在”这类负向断言。
- 路径或命名重构后，应通过扫残留确认旧符号被清理，而不是把旧符号不存在写进业务测试。



### Git 

如果我让你给出 git commit 建议，你给出的 git commit 建议必须符合如下规则：

- 只查看暂存区代码变更，不要根据未暂存内容猜测。
- 如果暂存区为空，直接说明没有可用变更，让用户先暂存。
- 根据变更内容给出一个或多个 commit，拆分粒度必须是整个文件，禁止按同一文件内的 hunk 拆分。
- 如果给出多个 commit，需要列出每个 commit 对应的文件。
- commit 必须符合 conventional commit 规范。
- commit 内容必须是英文。
- title 和 body 放在一起，方便用户直接复制。
- 禁止添加 `Co-Authored-By` 等联合开发者字样，项目由用户独立开发。

禁止操作 git 暂存区、提交等操作，这就是这类命令禁止操作：`git add`, `git restore`, `git commit`，
如果你发现暂存区、代码提交发生变化这是正常的，因为我在和你并行操作。你只需要关注代码变动即可。



### moudle 开发规范

具体开发规范在 `module/README.md` 文件中

开发 module 时，每个接口对应的【model/REQ/RSP】、【业务逻辑】必须写在自己对应的单独代码文件中，禁止将多个接口的【model/REQ/RSP】写在同一个 model 代码文件中，禁止将多个不同接口的【业务逻辑】写在同一个 service 代码文件中。三种场景如下：

- 完全不同的业务逻辑和接口：/api/users，/api/groups，那么需要两个 model 文件和两个 service 文件
- 同一资源对象则走框架提供的 curd：POST /api/configs、DELETE /api/configs/:id、 DELETE /api/configs、PUT /api/configs/:id、PATCH /api/configs/:id、GET /api/configs、GET /api/configs/:id，只需要一个 model 文件且 model 文件中没有自定义 REQ 和 RSP，service 文件中只有一个结构体，在结构体上加上不同的 hooks。
- 同一资源对象走自定义业务逻辑：GET /api/iam/sessions、DELETE /api/iam/sessions/:id。还是只需要一个 model 文件和一个 service 文件，但都有自己的 REQ、RSP service 结构体。注意 List、Get 是 HTTP GET 接口，禁止声明 `Payload[T]()`，只声明 `Result[T]()`，请求类型固定为 `*model.Empty`：
  - model 代码文件中的结构体：`SessionListRsp`、`SessionDeleteReq`、`SessionDeleteRsp`。
  - service 结构体方法：
    `func (s *SessionListService) List(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamsession.SessionListRsp, err error)`、
    `func (s *SessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.SessionDeleteReq) (rsp *modeliamsession.SessionDeleteRsp, err error)`

module 包中的接口测试用例规范：

- 测试文件名名要符合子 moudle 名，例如 module/iam/session_test.go 就是专门用来存放 session 相关接口的测试用例，其对应的接口实现放在 internal/{model,service}/session 目录中。
- 测试组织方式要改成一个接口对应一个顶层测试函数，各个顶层测试函数应该尽量避免相互影响。
- 如果同一个接口有多种场景，则在这个接口对应的测试函数里 用 t.Run(...) 做子测试，如果只有一个场景，则不需要额外使用 t.Run(...) 来运行子测试。
- 测试用到的辅助函数应该放在其对应的测试文件中，例如 session 子模块相关的测试辅助函数应该放在 session_test.go 中，account 子模块相关的测试辅助函数应该放在 account_test.go 中。并且测试用例使用到的辅助函数尽量放在顶层测试函数之后。



### README.md

README.md 面向使用 gst 框架的后端开发者，应保持简洁并聚焦实际使用流程；不要写入内部实现、维护者决策或开发者不需要关心的细节，但必须保留会影响正确使用的关键信息、命令顺序和风险提醒。





### 如何使用当前框架

可以结合 `examples/demo` 理解框架用法。这里描述的是后端项目如何使用 gst，不是 gst 框架源码本身的开发流程。

#### 基本流程

1. 在 gst 仓库执行 `make install` 安装 `gg` 命令。
2. 使用 `gg new myproject` 创建后端项目。
3. 在业务项目中修改或新增 `model` 文件，例如 `model/user.go`、`model/config/file.go`。
4. 修改 model 的 DSL 后执行 `gg gen` 生成 `main.go`、`model/model.gen.go`、`model/apidoc.gen.go`、`service/service.gen.go`、`router/router.gen.go` 等注册代码。
5. 在对应的 `service` 文件中实现业务逻辑和复杂 hook。
6. 如果 model 的 `Design()` 中声明了 `Migrate()`，该 model 也是数据库模型；数据库字段变化后使用 `gg migrate --dry-run` 预览迁移，再用 `gg migrate` 按确认执行 schema 迁移。
7. 服务启动后会自动生成 Swagger 文档，访问路径是 `/docs/index.html`。

#### 生成文件和手写文件的边界

- `main.go` 和所有 `.gen.go` 文件（`model/model.gen.go`、`model/apidoc.gen.go`、`service/service.gen.go`、`router/router.gen.go` 等）由 `gg gen` 生成，主要负责导入包和注册 model、service、router 以及 Swagger 文档使用的注释。除非明确要修改生成器，否则不要手写这些文件。
- `model/**/*.go` 是接口和数据模型声明层。这里定义结构体字段、轻量级 model hook、`Design()` DSL、`Migrate()`、`Endpoint()`、`Param()`、`Route()`、`Payload()`、`Result()`、`Public()` 等接口行为。
- `service/**/*.go` 是业务实现层。这里实现 `Create`、`Delete`、`Update`、`Patch`、`List`、`Get`、`DeleteMany` 等方法，以及 `CreateBefore`、`ListAfter`、`Filter`、`FilterRaw` 等复杂 hook。
- `module/` 用来注册内置或自定义模块，例如 `iam.Register(...)`。
- `configx/`、`cronjob/`、`middleware/` 分别用于扩展配置、定时任务和中间件，应用入口通过空导入触发它们的 `init()`。

#### model 设计规则

- 普通数据库资源使用 `model.Base`，通常在 `Design()` 中声明 `Migrate()`、`Endpoint(...)` 和 `Param(...)`。
- 不落数据库、只表示动作或自定义接口的模型优先使用 `model.Empty`，例如登录、刷新 token、文件加密、批量处理等接口。
- 默认 CRUD 资源优先交给框架处理：在 `Design()` 中启用对应动作即可。如果没有额外业务逻辑，不声明 `Service()`。
- 需要自定义业务逻辑时，在对应动作中声明 `Service()`，然后在同名 service 子目录中实现对应 phase 的 service 结构体。
- 自定义接口必须为当前接口单独定义 `XXXReq`、`XXXRsp`，即使字段和其他接口完全相同也不要复用。请求和响应类型通过 `Payload[*XXXReq]()`、`Result[*XXXRsp]()` 绑定到 DSL。例外：List、Get 是 HTTP GET 接口，禁止声明 `Payload`，只定义并声明 `Result[*XXXRsp]()`，请求类型固定生成为 `*model.Empty`。
- 同一资源的嵌套路由或额外动作使用 `Route(...)` 包裹，例如 `/config/files/encrypt`、`/items/batch` 这类非默认 CRUD 路由。

#### service 实现规则

- service 结构体通常嵌入 `service.Base[M, REQ, RSP]`，其中 `M` 是 model，`REQ` 是请求类型，`RSP` 是响应类型。
- service 类型按 phase 命名，例如 `Creator`、`Lister`、`Getter`、`Updater`、`Patcher`、`Deleter`、`ManyDeleter`。注册时在生成的 `service/service.gen.go` 中映射到 `consts.PHASE_CREATE`、`consts.PHASE_LIST` 等 phase。
- 业务代码只使用 `service.Base` 和生成代码里的 `service.Register`；service 查找、registry map、实例注入和 logger 注入等状态由框架内部维护，不作为业务项目 API 使用。
- 查询和写库优先使用 `database.Database[T](ctx)`，并按需要组合 `WithQuery`、`WithSelect`、`WithPagination`、`WithOrder`、`WithLimit` 等框架能力。
- 列表过滤优先实现 `Filter(ctx, model)` 或 `FilterRaw(ctx)`；返回数据补充、关联查询、字段填充等逻辑优先放在 `ListAfter`。
- 简单字段校验、默认值、哈希计算等贴近模型生命周期的逻辑可以放在 model hook，例如 `CreateBefore`、`UpdateBefore`；复杂业务编排放在 service。

#### 常见接口模式

- `model/record.go`：普通数据库资源 model，启用 CRUD，并通过 service hook 做当前用户过滤、返回字段补充、关联对象填充等逻辑。
- `model/common/common.go`：通用工具类接口，使用 `model.Empty` 定义非数据库动作，并为当前接口单独定义请求和响应。适合搜索结果去重、文件解析、批量转换等没有独立数据表的动作。
- `model/auth/login.go`：登录跳转类公开接口，使用 `model.Empty` 定义动作模型，在 DSL 中声明 `Public()`，service 返回登录地址、token、回调结果等响应。
- `model/config/namespace/file.go`：配置文件类数据库资源，同一个 model 可以同时提供默认资源路由和自定义嵌套路由；文件名、必填字段、文件大小、校验和等轻量逻辑放在 model hook。
- `model/config/namespace/file/encrypt.go`：文件动作类接口，加密、解密、复制、发布、格式化等动作使用空模型加独立 `XXXReq`、`XXXRsp`，业务逻辑放在对应 service。

#### 后端项目使用注意事项

- 修改 model 的 DSL、接口路径、REQ/RSP、service 开关后，需要重新执行 `gg gen`，并检查生成的 router 和 service 注册是否符合预期。
- 修改声明了 `Migrate()` 的数据库模型字段后，需要执行 `gg migrate --dry-run` 检查 `generated/migrate/<dbtype>/schema.sql` 和迁移计划，再用 `gg migrate` 按确认处理 schema 变化。
- 改索引名必须"先 `gg migrate` 后发布"，先发布会触发 gorm 启动期对单列唯一索引的静默 DROP + CREATE 重建；migrate 计划前的疑似改名提示应优先按指引改用 `RENAME INDEX` 处理。
- 不要手写覆盖生成文件；如果生成结果不符合预期，优先修正 model DSL 或框架生成逻辑。
- 后端服务启动后，通过 `/docs/index.html` 检查生成的 Swagger 文档和接口路径。
- 后端项目自身如果已有测试，修改业务逻辑后需要运行对应测试；涉及框架仓库改动时仍按 gst 仓库要求执行 `make check`。
