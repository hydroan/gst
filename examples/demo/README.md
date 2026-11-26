# demo

gst 的入门示例：一个用 `gg new` 生成、按框架推荐写法补齐的小项目，覆盖 DSL 的每个关键字、
每个扩展点（配置、定时任务、常驻组件、中间件、拦截器、模块、provider）和测试的写法。
`examples/bench` 是压测项目，`examples/cluster` 是多副本部署示例，这里只讲单体入门。

## 目录

| 路径 | 内容 |
| --- | --- |
| `model/` | 模型与接口声明（DSL）。根目录和 `ticket/`、`archive/`、`tool/` 是 HTTP 示例；`board/` 是 gRPC 示例，模型声明了 `GRPC()`，同一个动作两条线都提供 |
| `service/` | 业务实现和它的测试，目录镜像 `model/`；只有声明了 `Service()` 的动作才有文件 |
| `pb/` | `gg gen` 从 `model/board/` 推导的 `.proto` 和 Go 代码，提交进仓库 |
| `configx/` `cronjob/` `component/` `middleware/` `interceptor/` `module/` | 扩展点：每一项一个文件，在各自的 `xxx.go` 的 `init()` 里注册 |
| `dao/` | service 之下共用的东西：这里是每进程定时任务刷新、ping 答出的工单计数 |
| `internal/testsupport/` | 测试共用的一小段：注册一个新账号并登录 |
| `config.ini` | 本地运行的配置。测试不靠它，testutil 自己起容器并指定端口 |
| `main.go`、`*.gen.go` | `gg gen` 生成，不手改 |

## 工作流

1. `gg new demo` 生成骨架（本项目已经生成好）。
2. 在 `model/` 里写模型：结构体加 `Design()`。
3. 执行 `gg gen`：生成注册文件、`.proto`、声明了 `Service()` 的 service 文件和它们的测试骨架。
4. 把 service 文件里的方法和 hook 填上，把测试骨架的第一行 `t.Fatal` 删掉、补成真正的用例。
5. `go test ./...`：需要 Docker，testutil 会起 redis、minio 容器（数据库用 sqlite，不起容器）。
6. `gg check`：检查目录、命名、tag、REQ/RSP、protobuf 契约等约定。
7. 数据库字段有变化时 `gg migrate --dry-run` 看计划，再 `gg migrate` 执行；本地开发在 `config.ini` 里开了 `auto_migrate`，启动时会自动建表。
8. `go run .` 启动，HTTP 在 8090、gRPC 在 8091，接口文档在 http://localhost:8090/docs/index.html 。
9. 前端要类型时 `gg gen ts`：按 model 里的声明生成 TypeScript 类型。

改了 DSL、路径、REQ/RSP 或 `Service()` 之后回到第 3 步。

## DSL 关键字在哪个文件

| 关键字 | 文件 |
| --- | --- |
| `Migrate` `Endpoint` `Param` `Create` `Delete` `Update` `Patch` `List` `Get` `Route` `Service` `Payload` `Result` | `model/ticket.go`（工单）：新建和修改声明自己的请求类型，删除、列表、读取用框架默认流程加 hook |
| 关联预加载 `Expands()`（请求带 `_expand=comments`）、乐观锁 `model.Version`、`binding` 校验 tag | `model/ticket.go` |
| 回收站：`WithDeleted` 读软删除的行 | `model/ticket.go` 的 trash 路由，`service/ticket/trash.go` |
| 分析查询 `database.Select`：分组计数 | `service/ticket/summary.go` |
| 连接查询 `Join`：按工单的状态统计回复 | `model/ticket/comment.go` 的 summary 路由，`service/ticket/comment/summary.go` |
| `Upsert`：同一个键再提交就是改值 | `model/preference.go`，`service/preference/set.go` |
| 字段类型 `datatypes.Date` `datatypes.TimeOfDay` `datatypes.JSONSlice` `datatypes.JSONMap` `datatypes.JSON`：日期、时段和存成 JSON 的列；不落库的查询参数交给 `Filter` 钩子，用生成的列引用按日期范围、按「数组里包含某个值」过滤 | `model/meeting.go`（会议），`service/meeting/list.go` |
| 只建表、不出接口（只写 `Migrate()`） | `model/audit.go` |
| 嵌套资源、`CreateMany` `DeleteMany` `UpdateMany` `PatchMany` | `model/ticket/comment.go`（工单下的回复） |
| 自定义路由、`Import` `Export` | `model/archive/document.go` |
| `Exact` `Payload`、provider（MinIO） | `model/archive/document/attachment.go` |
| `Flatten` | `model/tool/label.go`（合并标签） |
| `Public` | `model/ping.go` |
| `SSE` | `model/notice.go` |
| `GRPC`、`pb` tag、gRPC 上的标准动作和自定义动作 | `model/board/note.go` |
| `binding:"readonly"`：客户端不能写、只由发布动作写的字段 | `model/board/note.go` |
| `Stream` `StreamingPayload` `StreamingResult`：三种流，gRPC 上是流式 rpc，HTTP 上是 WebSocket 连接 | `model/board/feed.go` |
| 不写 `GRPC()` 的 `Stream`：只走 WebSocket | `model/tally.go` |

每个文件开头的注释说明了这个模型演示什么、路由长什么样。

## 扩展点

- 配置：`configx/notice.go`、`configx/cleanup.go` 各声明一节，`configx.go` 注册；每个键有对应的环境变量常量。`Cleanup` 带 `Validate()` 方法检查自己的值，保留天数或每批行数小于 1 时启动直接失败。
- 定时任务：`cronjob/purge_audits.go`、`cronjob/report_audits.go` 全部署每个时刻只跑一次；`cronjob/count_tickets.go` 用 `RegisterPerInstance`，每个副本各刷新自己进程里的工单计数（计数放在 `dao/ticket_count.go`）。函数和它的调度、名字放在一起，`cronjob.go` 注册。
- 常驻组件：`component/notice_watch.go` 是订阅 Redis 频道 `demo:notices` 的消费循环，`component/runtime_report.go` 定期记录本进程的运行指标；每个副本各跑一份，直到进程退出。
- 中间件：`middleware/no_store.go` 挂在所有接口上，`middleware/actor.go` 只挂在需要认证的接口上；框架自带的 `RequestSizeLimit`、`Timeout`、`SecurityHeaders` 在 `middleware.go` 里直接挂；iam 模块的会话检查也在那里挂，排在读用户的中间件之前。
- 拦截器：`interceptor/` 是中间件在 gRPC 上的对应物，`served_by.go` 和 `actor.go`，会话检查同样挂在这里。
- 模块：`module/module.go` 注册内置的 iam（登录、注册、会话）和示例模块 helloworld；`module/login_audit.go` 是登录观察者，登录成功、失败和登出各写一条 Audit。
- provider：`service/archive/document/attachment/` 用 MinIO 存文件，`config.ini` 的 `[minio]` 节配置连接。
- 数据库：service 里用 `database.Database[T](ctx)` 链式查询，见 `service/ticket/search.go`、`service/ticket/trash.go`（`WithDeleted`）、`service/preference/set.go`（`Upsert`）和 `cronjob/purge_audits.go`；聚合用 `database.Select`，见 `service/ticket/summary.go`（分组）和 `service/ticket/comment/summary.go`（连接）。

## 本地运行需要什么

`config.ini` 指向本机的 MySQL（test/test）、Redis 和 MinIO（minioadmin/minioadmin，桶 demo）。
只跑测试的话都不需要：testutil 自己起容器。

服务起来后给组件发一条通知，日志里会多一行 `notice received`：

```bash
redis-cli PUBLISH demo:notices hello
```

## 看一眼 WebSocket

`Stream` 动作在 HTTP 上是一条 WebSocket 连接，`GET` 握手打开，之后一帧文本是一条 JSON 消息。
打开 http://localhost:8090/docs/index.html ，在浏览器控制台里粘贴（同源的页面才连得上，别的站点的页面要先写进 `[server]` 节的 `websocket_origins`）：

```js
const ws = new WebSocket("ws://localhost:8090/api/tallies");
ws.onmessage = (event) => console.log(event.data);
ws.onclose = (event) => console.log("closed", event.code);
ws.onopen = () => {
  ws.send(JSON.stringify({ amount: 2 }));
  ws.send(JSON.stringify({ amount: 3 }));
  ws.send("null");
};
```

控制台依次打印 `{"total":2}`、`{"total":5}`、`closed 1000`：最后那帧 `null` 是「我发完了」，
service 的 `Recv` 得到 `io.EOF` 后返回，连接随之正常关闭。Go 里的调用写法见 `service/tally/sum_test.go`，
需要登录的流见 `service/board/feed/chat_test.go`（会话在登录时种下的 Cookie 里，握手时自动带上）。

## 看一眼 gRPC

服务默认开了反射，grpcurl 能直接列出服务；公开的 `WatchFeed` 不需要登录：

```bash
grpcurl -plaintext localhost:8091 list
```

```bash
grpcurl -plaintext -d '{"payload":{"topic":"news"}}' localhost:8091 demo.board.FeedService/WatchFeed
```

需要登录的方法把会话 id 放在 `authorization: Bearer <session id>` 里，会话绑定登录时的 User-Agent，
程序要用什么客户端调用就用什么 User-Agent 登录；写法见 `service/board/note/create_test.go`
（登录时带上 `iamtest.GRPCUserAgent`，调用时用 `grpc.WithBearer` 带上会话）。
