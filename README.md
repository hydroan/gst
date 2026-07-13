[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/hydroan/gst)

# gst

`gst` 是一个面向 Go 后端项目的快速开发框架。它的核心使用方式是：
开发者在业务项目中编写 `model` DSL，随后用 `gg` 生成应用入口、路由注册、
model 注册和 service 注册，再在 `service` 中补充业务逻辑。

> 当前仓库是 gst 框架源码仓库。业务项目应通过 `gg new` 创建；不要在 gst
> 框架仓库根目录对业务项目运行 `gg gen`、`gg migrate` 等生成命令。

前后端接口对接（请求位置、通用查询参数、响应结构）以 [API_CONTRACT.md](API_CONTRACT.md) 为准。

## 快速开始

### 安装 gg

业务项目直接安装发布版：

```bash
go install github.com/hydroan/gst/cmd/gg@latest
```

如果需要基于当前源码验证 `gg` 命令，可以在 gst 源码仓库安装本地版本：

```bash
make install
```

### 创建业务项目

```bash
gg new github.com/example/myapp
cd myapp
cp config.ini.example config.ini
```

`gg new` 会使用 module path 的最后一段创建项目目录，例如上面的目录名是
`myapp`。它会生成基础目录、`main.go`、`config.ini.example`，并执行
`go mod tidy` 和 `git init`。

如果 `go mod tidy` 因网络、代理、本地 Go 缓存权限等原因失败，已生成的项目文件
通常仍保留在项目目录中。进入项目后修复环境并重新执行：

```bash
go mod tidy
git init
```

### 了解生成目录

新项目的常用目录含义如下：

| 路径 | 责任 |
| --- | --- |
| `model/**/*.go` | 声明数据结构、接口 DSL、轻量 model hook |
| `service/**/*.go` | 实现业务逻辑、复杂 hook、查询过滤和返回补充 |
| `module/` | 注册内置或自定义模块，例如 IAM |
| `configx/` | 扩展配置 |
| `cronjob/` | 注册定时任务 |
| `middleware/` | 注册中间件 |
| `router/router.go` | 由 `gg gen` 生成的路由注册文件 |
| `model/model.go` | 由 `gg gen` 生成的模型注册文件 |
| `model/apidoc.go` | 由 `gg gen` 生成的注释与枚举注册文件，让 Swagger 文档在无源码的部署环境仍带字段说明和枚举值 |
| `service/service.go` | 由 `gg gen` 生成的 service 注册文件 |
| `main.go` | 由 `gg gen` 生成的应用入口 |

通常只手写 `model/**/*.go`、`service/**/*.go` 和扩展目录中的业务代码。生成文件
不要手改；如果生成结果不符合预期，先检查 model DSL，再重新执行 `gg gen`。

## 开发主线

日常开发按这个顺序走：

1. 在 `model/**/*.go` 中声明资源模型或动作模型。
2. 每次修改 DSL 后运行 `gg gen`。
3. 在生成的 `service/**` 文件中实现业务逻辑或 hook。
5. 使用 `gg check` 检查项目结构和依赖边界。
6. 删除 model 或关闭 action 后，运行 `gg prune` 或 `gg gen --prune` 清理废弃
   service 文件。

## 模型 DSL

`model` 是业务项目的主要输入。先判断当前接口属于哪一种模式。

### 数据库资源

普通资源使用 `model.Base`。如果这个资源需要建表或迁移，声明 `Migrate()`。

```go
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Conversation is a database-backed resource.
type Conversation struct {
	UserID string `json:"user_id" schema:"user_id"`
	Title  string `json:"title" schema:"title"`

	model.Base
}

func (Conversation) Design() {
	Migrate()
	Endpoint("conversations")
	Param("conv")

	Create(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
	List(func() {
		Service()
	})
	Get(func() {})
}
```

这个模型会生成类似下面的路由：

- `POST /api/conversations`
- `PATCH /api/conversations/:conv`
- `GET /api/conversations`
- `GET /api/conversations/:conv`

`Param("conv")` 控制单资源路由中的参数名。未声明 `Param(...)` 时，单资源路由默认
使用框架默认参数。

需要自增整数主键的资源改用 `model.AutoBase`，字段和默认 hook 与 `model.Base`
一致，区别是 ID 由数据库在插入时分配（框架不会生成）。注意：这类模型通过
`model.Register` 注入 seed 记录时必须显式指定 ID 或依赖唯一索引，否则重复启动会
重复插入；`Base` 字符串 ID 的逗号分隔多 ID 查询写法对整数 ID 不适用。

### 自定义动作

不直接表示数据库表的接口优先使用 `model.Empty`，并为当前接口单独定义自己的
`XXXReq`、`XXXRsp`。即使字段完全一样，也不要复用其他接口的请求和响应结构体。

```go
package common

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Search is a non-database action model.
type Search struct {
	model.Empty
}

// SearchSource is one candidate source returned by a search provider.
type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// SearchDedupReq is the request for deduplicating search sources.
type SearchDedupReq struct {
	Sources []SearchSource `json:"sources"`
}

// SearchDedupRsp is the response returned after deduplication.
type SearchDedupRsp struct {
	Sources []SearchSource `json:"sources"`
}

func (Search) Design() {
	Route("/search-sources/dedup", func() {
		Create(func() {
			Filename("dedup")
			Service()
			Payload[*SearchDedupReq]()
			Result[*SearchDedupRsp]()
		})
	})
}
```

这个接口会生成 `POST /api/search-sources/dedup`，并生成
`service/common/search/dedup.go`。`Filename("dedup")` 用于避免同一个 model 内多
个 `Create` action 都生成 `create.go`。

### 路由和可见性

- `Endpoint("conversations")` 定义默认资源路径。
- `Route("/config/files", func() {...})` 定义额外路径或完全自定义路径。
- `Public()` 表示公开接口，不走认证中间件；默认不写则需要认证。
- `Exact()` 表示当前 action 按声明路径原样注册，不追加默认的 `/:id`、`/batch` 等后缀。
- `Payload[T]()` 定义请求体类型；`Result[T]()` 定义响应体类型。
- `List`、`Get` 是 HTTP GET 接口，没有请求体，禁止声明 `Payload[T]()`；
  只声明 `Result[T]()` 即成为自定义动作，生成的请求类型固定为 `*model.Empty`，
  查询参数通过 `ctx.Query()`、路径参数通过 `ctx.Param()` 读取。
- `Service()` 表示当前 action 需要生成并注册业务 service。
- 只声明 `Create(func(){})`、`List(func(){})` 等 action 就会启用对应接口；
  `Enabled(false)` 主要用于显式关闭已声明 action。

## 业务 Service

生成后的业务逻辑主要写在 `service/**`。service 通常嵌入：

```go
service.Base[M, REQ, RSP]
```

其中 `M` 是模型类型，`REQ` 是请求类型，`RSP` 是响应类型。

业务项目只需要关注 DSL、生成代码里的 `router.Register` / `service.Register`，以及
业务实现里的 `service.Base`。业务代码不需要依赖更底层的框架执行包；手写高级路由时
也应通过 `router.Register` 接入。

需要特别注意默认资源和自定义动作的 service 写法不同：

- 默认资源 CRUD：当 `M`、`REQ`、`RSP` 是同一个类型时，框架会执行默认
  数据库流程，业务侧主要实现 `CreateBefore`、`CreateAfter`、`ListAfter`、
  `Filter`、`FilterRaw` 等 hook。
- 自定义动作：当 `Payload` 或 `Result` 让 `REQ`、`RSP` 不同于 `M` 时，
  框架会调用 service 的 `Create`、`List`、`Delete` 等 action 方法。

默认资源的 hook 示例：

```go
package conversation

import (
	appmodel "github.com/example/myapp/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*appmodel.Conversation, *appmodel.Conversation, *appmodel.Conversation]
}

func (c *Creator) CreateBefore(ctx *types.ServiceContext, conversation *appmodel.Conversation) error {
	if conversation.Title == "" {
		return errors.New("title is required")
	}
	return nil
}
```

自定义动作的 service 示例：

```go
package search

import (
	"github.com/example/myapp/model/common"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Dedup struct {
	service.Base[*common.Search, *common.SearchDedupReq, *common.SearchDedupRsp]
}

func (d *Dedup) Create(ctx *types.ServiceContext, req *common.SearchDedupReq) (*common.SearchDedupRsp, error) {
	seen := make(map[string]struct{}, len(req.Sources))
	rsp := &common.SearchDedupRsp{}

	for _, source := range req.Sources {
		if _, ok := seen[source.URL]; ok {
			continue
		}
		seen[source.URL] = struct{}{}
		rsp.Sources = append(rsp.Sources, source)
	}
	return rsp, nil
}
```

查询和写库优先使用：

```go
database.Database[*appmodel.Conversation](ctx)
```

并按需要组合 `WithQuery`、`WithSelect`、`WithPagination`、`WithOrder`、
`WithLimit` 等选项。一次查询或写入使用一个新的 `database.Database[T](...)`
链式调用，不要在无关操作之间复用同一个 database 句柄。

## 配置和迁移

`config.ini.example` 是新项目的默认配置模板。复制为 `config.ini` 后按环境修改。
默认模板会开启 sqlite，适合本地快速启动。

常用配置命令：

```bash
gg config list
gg config defaults --format ini
gg config defaults server --format yaml
gg config convert config.ini config.yaml
```

模型声明 `Migrate()` 后，字段变化先预览迁移计划：

```bash
gg migrate --dry-run
```

确认无误后执行：

```bash
gg migrate
```

命令会生成 `generated/migrate/<dbtype>/schema.sql`，并在执行前要求确认。
执行前先确认 `config.ini` 指向目标环境，避免把开发中的模型变化迁移到错误数据库。

## 内置模块

业务项目可以在 `module/` 中注册内置模块。下面是注册 IAM 默认用户的形式：

```go
package module

import "github.com/hydroan/gst/module/iam"

func init() {
	iam.Register(iam.Config{
		DefaultUsers: []*iam.User{
			{
				Username: "root",
				Password: "toor",
			},
		},
	})
}
```

应用入口会空导入 `module`，因此 `init()` 会在启动阶段执行。

## 生成和检查命令

| 命令 | 用途 |
| --- | --- |
| `gg gen` | 根据 `model` DSL 生成注册文件和 service action 文件 |
| `gg gen --prune` | 生成后联动清理废弃 service action 文件 |
| `gg module copy <name>` | 将内置模块复制为业务项目本地源码，并提示目标目录中的额外 model/service 文件 |
| `gg check` | 检查业务项目结构、命名、依赖边界和 tag 约束 |
| `gg prune` | 只扫描并清理废弃 service action 文件 |
| `gg routes` | 按 model 层级打印当前生成的接口路径 |
| `gg route-tree` | 按 URL 层级打印当前生成的路由树 |
| `gg migrate` | 生成当前数据库方言的 schema，预览并按确认执行数据库迁移 |
| `gg dev` | 监听 model 变更自动生成代码，并使用 Air 热重载启动业务项目 |

`gg check` 会检查依赖边界、model/service 文件边界、命名规范、`json` tag、
REQ/RSP 命名和业务项目根目录结构；根目录结构检查会跳过 Git ignore 规则忽略的
目录。`gg gen` 生成前也会执行这些检查；检查失败会停止生成。

### 项目级配置 gst.yaml

在业务项目根目录（与 `go.mod` 同级）可放置可选的 `gst.yaml`，这是 `gg` 工具的
构建期工程配置，与运行时 `config.ini` 无关。

当前支持在 `gg gen`（含 `gg module copy` 后的重新生成）中忽略指定路由。
忽略只作用于生成的注册文件：`router/router.go` 不注册路由、
`service/service.go` 不注册 service，也不会为其生成新的 service 文件；
磁盘上已有的 service 文件（例如 `module copy` 拷贝来的）原样保留，
`gg gen --prune` 和 `gg prune` 都不会把它们当作待删除文件，项目文件与
`module copy` 输出保持一致。适合屏蔽 `module copy` 带来的不需要的接口，
或把被框架模块占用的路径让给业务自己的实现：

```yaml
version: 1

gen:
  routes:
    ignore:
      /api/signup: [POST]
      /api/iam/admin/users/:id: [GET, DELETE]
      # 对象形式：from 限定只忽略声明在该目录下的 model，
      # 业务可在自己的 model 目录重新声明同一路由
      /api/iam/admin/users:
        methods: [GET]
        from: model/iam
```

- 每个 path 写一次，值是要忽略的 HTTP method 列表；`/api` 前缀可省略，
  因此路径可直接粘贴 `gg routes` 的输出（其路径不带 `/api` 前缀）。
  参数段（`:id`）按位置匹配，不比较参数名。
- 需要用自己的实现替换框架路由时，用对象形式加 `from`（如 `model/iam`）
  把规则限定到框架模块目录；否则规则会把业务自己声明的同路径 action 一并
  忽略。无 `from` 的规则命中多个 model 目录时会输出 warning 提醒。
- 未匹配到任何路由的条目会在生成时输出 warning，提示配置可能已过期。
- 忽略不影响 model 的 `Migrate` 注册：表结构照常创建，模块内部逻辑
  （如登录查询用户表）不受影响。

## 示例

当前仓库的 `examples/demo` 是推荐阅读的完整业务项目示例：

- [应用入口](./examples/demo/main.go)
- [模块注册](./examples/demo/module/module.go)
- [资源模型：Conversation](./examples/demo/model/conversation.go)
- [嵌套资源模型：Message](./examples/demo/model/conversation/message.go)
- [配置文件资源模型：File](./examples/demo/model/config/file.go)
- [公开动作模型：Login](./examples/demo/model/auth/login.go)
- [自定义动作模型：搜索去重](./examples/demo/model/common/search.go)
- [自定义动作模型：文件加密](./examples/demo/model/config/file/encrypt.go)
- [资源 service hook](./examples/demo/service/conversation/create.go)
- [自定义动作 service](./examples/demo/service/common/search/dedup.go)
- [生成的路由注册](./examples/demo/router/router.go)
- [生成的 service 注册](./examples/demo/service/service.go)

## 常见问题

### 什么时候用 model.Base，什么时候用 model.Empty？

需要数据库表、默认 CRUD、迁移和模型生命周期 hook 时使用 `model.Base`。只表示一个
动作、工具接口、登录跳转、批处理等非数据库接口时使用 `model.Empty`。

### 什么时候用 model.AutoBase？

数据库资源默认用 `model.Base`（UUIDv7 字符串主键）。写入量大、增长快、且不需要
对外暴露不可猜测 ID 的表（例如流水、明细类），可以改用 `model.AutoBase` 获得更窄
的自增整数主键和更小的二级索引。

### 什么时候需要 Service()？

默认 CRUD 没有额外业务逻辑时不需要。需要 hook、过滤、返回补充、复杂查询，或当前
action 是自定义动作时再开启 `Service()`。

### 为什么我写了 service 的 Create 方法但没有被调用？

如果 `M`、`REQ`、`RSP` 是同一个类型，默认资源 CRUD 会执行框架内置流程，
只调用 service hook 和过滤方法。要让 action 主方法被调用，需要用 `Payload[T]()` 或
`Result[T]()` 绑定当前接口专用的 REQ/RSP，让它成为自定义动作。
`List`、`Get` 只能通过 `Result[T]()` 触发自定义动作，请求类型固定为 `*model.Empty`。

### Route 和 Endpoint 有什么区别？

`Endpoint` 是资源默认路径；`Route` 是额外路径或完全自定义路径。同一个 model 可以
同时声明默认资源路由和多个额外 `Route`。如果多个 `Route` 中有相同 phase 的
service，比如多个 `Create`，应使用 `Filename(...)` 避免生成文件冲突。

### 生成文件可以手改吗？

通常不要。`main.go`、`model/model.go`、`model/apidoc.go`、`service/service.go`、
`router/router.go` 由 `gg gen` 维护。手写业务逻辑放在 `model/**/*.go`、
`service/**/*.go` 和扩展目录。

### 如何确认接口路径？

修改 DSL 后运行：

```bash
gg gen
gg routes
```

如果想按 model 文件层级查看 model 和接口关系，可以运行：

```bash
gg routes --model
```

需要排查生成的请求、响应和路径参数绑定时，可以运行：

```bash
gg routes --detail
```

如果只想查看认证或公开路由，可以加上 scope 过滤：

```bash
gg routes --scope auth
gg routes --scope pub
```

也可以启动服务后访问 Swagger 文档：

```text
/docs/index.html
```

### 为什么删除 action 后 service 文件还在？

`gg gen` 默认保留已有 service 文件，避免误删手写业务代码。确认旧文件不再需要后
运行 `gg prune`，或使用 `gg gen --prune`。
