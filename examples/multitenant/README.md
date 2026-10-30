# multitenant：多租户示例

一个用 `gg` 生成的最小项目，演示框架的租户能力怎么用：模型嵌入 `tenant.Scope` 就按租户隔离行；登录时选租户；authz 模块在租户内授权；内置的 system root 跨全部租户；定时任务用 `tenant.In` 逐个租户干活；租户也可以从可信网关的请求头来；gRPC 上同样生效；最后是工具链对租户模型的检查和产物。每个场景都给了命令和它答什么，可以照着敲核对。这里只讲租户：其余能力看 `examples/demo`，多副本看 `examples/cluster`。

## 里面有什么

| 文件 | 演示什么 |
| --- | --- |
| `model/tenant.go` | 租户登记表 `tenants`：全局表，一行一个租户，id 就是登录时选的、行上盖的租户名。它不嵌 `tenant.Scope`，谁都能读；写只有 system root 能做，由模型钩子守住。`/api/tenants` 的 Create、Patch、Delete、List、Get |
| `model/ticket.go`、`service/ticket/create.go` | 租户资源 `tickets`：嵌入 `tenant.Scope` 就是全部声明，建行时盖上调用者的租户，之后每条语句都只到这个租户；`code` 在租户内唯一，索引写 `TenantID` 加 `Code`；声明了 `GRPC()`，六个标准动作同时是 `/api/tickets` 和 `TicketService` 的 rpc。只有 Create 有 service 文件：钩子用 `tenant.From` 拿到行将要盖上的租户，算出跨租户唯一的 `key` |
| `model/digest.go`、`cronjob/digest.go` | 定时任务 `digest_tickets` 每分钟给每个租户写一行 `digests`：任务在请求之外、没有授权来定租户，默认在 `default` 租户里，用 `tenant.In` 轮流进每个租户数票、写行，行就盖上那个租户 |
| `cronjob/reconcile.go` | 定时任务 `reconcile_policies` 每五分钟跨全部租户核对 authz 的策略和角色、菜单、绑定是否一致，结果写日志 |
| `model/membership.go`、`service/membership/list.go` | `GET /api/memberships`：当前用户是哪些租户的成员（`rbac.TenantsForSubject`）、这次请求在哪个租户里 |
| `module/module.go`、`module/seed.go` | 注册内置的 iam（登录、会话）和 authz（角色、绑定、菜单、授权判定）；启动钩子播种两个租户、五个账号、三个菜单和每个租户的角色、绑定，见下表 |
| `middleware/`、`interceptor/` | HTTP 和 gRPC 各挂三段认证链，顺序一样：iam 的会话检查（写下会话里的租户）、项目自己的网关头覆写、authz 的授权（在那个租户里判定，并把它交给行级隔离） |
| `configx/gateway.go`、`configx/seed.go` | `[gateway] tenant_header`：可信网关放租户的请求头名，空就不覆写；`[seed] password`：播种账号的口令 |
| `pb/` | `gg gen` 从 `model/ticket.go` 推导的 `.proto` 和 Go 代码，提交进仓库 |
| `internal/testsupport/` | 测试共用：用播种账号登进某个租户、拨 gRPC 连接 |

启动时播种的数据：

| | |
| --- | --- |
| 租户 | `alpha`（Alpha）、`beta`（Beta） |
| 账号 | `root`（内置的 system root）、`alice`、`bob`、`carol`、`dave`，口令都是 `[seed] password`，默认 `12345678` |
| 菜单（全局目录） | `tickets`：读票（GET `/api/tickets`、`/api/tickets/{id}`）；`tickets_manage`：写票（POST、PUT、PATCH、DELETE）；`digests`：读汇总 |
| 角色 | alpha：`agent`（tickets、digests）、`manager`（三个都有）；beta：`agent`（tickets、digests） |
| 绑定 | alice 是 alpha 的内置管理员 `admin`；bob 是 alpha 的 agent；dave 是 alpha 的 manager，也是 beta 的 agent；carol 是 beta 的 admin |
| 所有登录用户 | 不论在哪个租户、有没有角色，都能 GET `/api/tenants`、`/api/memberships`、`/api/authz/menus` |

## 租户从哪来

请求里的租户只有一个来源：请求上下文里的租户键 `requestctx.KeyTenantID`。登录时 `tenant_id` 选了哪个租户，会话就记住它，iam 的会话检查在每个请求上把它写进去；项目可以在会话检查之后、授权之前改写它，本例用可信网关的请求头；authz 的授权在这个租户里判定，放行后把同一个租户交给行级隔离，这次请求读写的租户资源行全部只在这个租户里。所以数据能到的范围不会比授权放行的范围大。登录时没选租户就是默认租户 `default`，单租户项目什么都不用配。

root 持有系统角色，它的授权不在任何一个租户里判定，于是它的请求跨全部租户。请求之外的代码（定时任务、启动钩子）没有授权来定租户，上下文默认在 `default` 租户里，要去别处自己说：`tenant.In` 进某一个租户，`tenant.Across` 跨全部。

## 起来

需要本机 MySQL（账号 test/test，库 `multitenant` 要先建好并授权给 test）和 Redis（会话存在这里）。`config.ini` 已指向它们，`auto_migrate` 开着，启动时自动建表并播种。只跑测试的话都不需要。

```bash
mysql -uroot -p -e "CREATE DATABASE multitenant; GRANT ALL ON multitenant.* TO 'test'@'%'"
```

```bash
cd examples/multitenant && go run .
```

HTTP 在 8092、gRPC 在 8093，接口文档在 http://localhost:8092/docs/index.html 。日志一行一条 JSON 写到 stdout，`logger` 字段标明哪一路（`access`、`authz`、`cronjob`、`app` 等）。

下面的场景用几个小工具，先在当前终端里定义好：

```bash
B=http://localhost:8092
# 会话绑定登录时的 User-Agent，框架只比对它的前两个名字；grpcurl 自报 grpcurl/<版本> grpc-go/<版本>，
# 所以登录和 curl 都用这个 UA，同一个会话 HTTP、gRPC 都能用。
UA='grpcurl/1.0 grpc-go/1.0'
# login USER [TENANT]：登进 TENANT（不写就是不选租户），打印会话 id。
login() { curl -s -c - -A "$UA" -X POST $B/api/login -H 'content-type: application/json' -d "{\"username\":\"$1\",\"password\":\"12345678\",\"tenant_id\":\"${2:-}\"}" | awk '$6 == "session_id" {print $7}'; }
# as SESSION METHOD PATH [BODY]：带会话调 HTTP 接口。
as() { local s=$1 m=$2 p=$3; shift 3; curl -s -A "$UA" -b "session_id=$s" -X "$m" -H 'content-type: application/json' ${1:+-d "$1"} "$B$p"; echo; }
# rpc SESSION 参数...：带会话调 gRPC，参数照 grpcurl 的写。
rpc() { local s=$1; shift; grpcurl -plaintext -H "authorization: Bearer $s" "$@"; }
```

## 场景

### 一、登录时选租户

登录请求的 `tenant_id` 选这次会话在哪个租户里：

```bash
# alice 不是 beta 的成员，登不进去。
curl -s -A "$UA" -X POST $B/api/login -H 'content-type: application/json' -d '{"username":"alice","password":"12345678","tenant_id":"beta"}'; echo
# 自己是哪些租户的成员、这次在哪个租户里：bob 没选租户，就在默认租户里；dave 选了 alpha。
BOB0=$(login bob)
as "$BOB0" GET /api/memberships | jq -c '.data'
DAVE=$(login dave alpha)
as "$DAVE" GET /api/memberships | jq -c '.data'
# 租户登记表谁都能读，客户端据此让用户选租户。
as "$BOB0" GET /api/tenants | jq -c '[.data.items[] | {id, name}]'
# root 不是任何租户的成员，但哪个租户都能登。
login root beta | cut -c1-8
```

alice 登 beta 答 403 `{"data":null,"msg":"user is not a member of tenant",...}`；bob 答 `{"tenants":["alpha"],"current":"default"}`，dave 答 `{"tenants":["alpha","beta"],"current":"alpha"}`；登记表列出 alpha、beta；root 登 beta 拿到会话。

### 二、行级隔离

`Ticket` 嵌入了 `tenant.Scope`，建行时盖上请求所在的租户，之后的每条语句都带着它：

```bash
ALICE=$(login alice alpha); CAROL=$(login carol beta)
# 建行盖章：alice 在 alpha 里建票，请求体里写的 tenant_id=beta 被盖掉。
T1=$(as "$ALICE" POST /api/tickets '{"code":"T-1","title":"printer jammed","tenant_id":"beta"}')
echo "$T1" | jq -c '.data | {tenant_id, key, status}'; T1_ID=$(echo "$T1" | jq -r '.data.id')
# 别的租户看不见它：列表没有，按 id 取、删都是 404。
as "$CAROL" GET /api/tickets | jq -c '.data.total'
as "$CAROL" GET "/api/tickets/$T1_ID" | jq -c '.msg'
as "$CAROL" DELETE "/api/tickets/$T1_ID" | jq -c '.msg'
# code 在租户内唯一：beta 里也能有 T-1，alpha 里再建一个就是 409。
as "$CAROL" POST /api/tickets '{"code":"T-1","title":"badge reader"}' | jq -c '.data | {tenant_id, key}'
as "$ALICE" POST /api/tickets '{"code":"T-1","title":"again"}' | jq -c '.msg'
# 挪不走：整条更新带别的租户答 400，这一列只在建行时写一次。
as "$ALICE" PUT "/api/tickets/$T1_ID" '{"code":"T-1","title":"moved","status":"open","tenant_id":"beta"}' | jq -c '.msg'
# 过滤参数写别的租户也到不了：租户条件始终 AND 在前面。
as "$ALICE" GET '/api/tickets?tenant_id=beta' | jq -c '.data.total'
```

alice 的票答 `{"tenant_id":"alpha","key":"alpha/T-1","status":"open"}`；carol 的列表 `0`，取和删都答 `The requested resource was not found.`；carol 建出 `{"tenant_id":"beta","key":"beta/T-1"}`，alice 再建答 `The resource already exists.`；挪租户答 `The request contains invalid parameters.`；带 `tenant_id=beta` 过滤答 `0`。

### 三、租户内的权限

authz 的角色、绑定都是租户里的行，授权在请求所在的租户里判定：

```bash
BOB=$(login bob alpha)
# bob 的 agent 角色只选了读票的菜单：读到，建不了；dave 的 manager 角色建得了。
as "$BOB" GET /api/tickets | jq -c '.data.total'
as "$BOB" POST /api/tickets '{"code":"T-3"}' | jq -c '.msg'
as "$DAVE" POST /api/tickets '{"code":"T-3","title":"vpn"}' | jq -c '.data | {key}'
# 菜单按角色裁：bob 看到自己角色选的，alice 是管理员，看到全部。
as "$BOB" GET /api/authz/menus | jq -c '[.data.items[].id]'
as "$ALICE" GET /api/authz/menus | jq -c '[.data.items[].id]'
```

bob 读到 `1` 条、建答 `permission denied`，dave 建出 `{"key":"alpha/T-3"}`；bob 的菜单 `["digests","tickets"]`，alice 的 `["digests","tickets","tickets_manage"]`。

租户管理员自己管本租户的角色和绑定，角色、绑定表本身也是租户资源：

```bash
# carol 看到的只有 beta 的角色，alice 看到的只有 alpha 的。
as "$CAROL" GET /api/authz/roles | jq -c '[.data.items[] | {tenant_id, name}]'
as "$ALICE" GET /api/authz/roles | jq -c '[.data.items[] | {tenant_id, name}]'
# alice 建角色 viewer（只选 tickets 菜单），把 bob 从 agent 换成 viewer。
# 用户列表走 iam 的管理接口，租户管理员只看到本租户的成员。
VIEWER=$(as "$ALICE" POST /api/authz/roles '{"name":"viewer","menu_ids":["tickets"]}' | jq -r '.data.id')
BOB_ID=$(as "$ALICE" GET '/api/iam/admin/users?username=bob' | jq -r '.data.items[0].id')
AGENT_BINDING=$(as "$ALICE" GET "/api/authz/role-bindings?subject_id=$BOB_ID" | jq -r '.data.items[0].id')
as "$ALICE" DELETE "/api/authz/role-bindings/$AGENT_BINDING" | jq -c '.msg'
as "$BOB" GET /api/digests | jq -c '.msg'
as "$ALICE" POST /api/authz/role-bindings "{\"subject_id\":\"$BOB_ID\",\"role_id\":\"$VIEWER\"}" | jq -c '.data | {tenant_id, role_id}'
as "$BOB" GET /api/authz/menus | jq -c '[.data.items[].id]'
as "$BOB" GET /api/tickets | jq -c '.data.total'
as "$BOB" GET /api/digests | jq -c '.msg'
```

carol 答 `[{"tenant_id":"beta","name":"agent"}]`，alice 答 alpha 的 agent 和 manager；删掉 agent 绑定后 bob 读汇总答 `permission denied`；新绑定答 `{"tenant_id":"alpha","role_id":"<viewer 的 id>"}`，绑定行盖的是 alpha；之后 bob 的菜单只剩 `["tickets"]`，读票 `2` 条，读汇总仍是 `permission denied`。写在同一个副本上的变更当场生效；多副本下其余副本最多一分钟后跟上，见 `examples/cluster` 的「权限变更在副本间传播」。

菜单是全局目录，改它影响每个租户，只有 root 能改：

```bash
ROOT=$(login root alpha)
as "$ALICE" PATCH /api/authz/menus/tickets_manage '{"label":"Manage"}' | jq -c '.msg'
# root 把 tickets_manage 菜单里的 PATCH 去掉，dave 的 manager 角色随之失去 PATCH；放回去又有了。
T3_ID=$(as "$DAVE" GET '/api/tickets?code=T-3' | jq -r '.data.items[0].id')
as "$ROOT" PATCH /api/authz/menus/tickets_manage '{"routes":[{"path":"/api/tickets","methods":["POST"]},{"path":"/api/tickets/{id}","methods":["PUT","DELETE"]}]}' | jq -c '.msg'
as "$DAVE" PATCH "/api/tickets/$T3_ID" '{"title":"vpn down"}' | jq -c '.msg'
as "$ROOT" PATCH /api/authz/menus/tickets_manage '{"routes":[{"path":"/api/tickets","methods":["POST"]},{"path":"/api/tickets/{id}","methods":["PUT","PATCH","DELETE"]}]}' | jq -c '.msg'
as "$DAVE" PATCH "/api/tickets/$T3_ID" '{"title":"vpn down"}' | jq -c '.data.title'
```

alice 改菜单答 `only system administrators may modify menus`；去掉 PATCH 后 dave 改票答 `permission denied`，放回去后答 `"vpn down"`。

### 四、system root 跨租户

```bash
# root 登在 alpha，但授权不在任何一个租户里判定，请求跨全部租户；要看一个租户就带过滤参数。
as "$ROOT" GET /api/tickets | jq -c '[.data.items[].key]'
as "$ROOT" GET '/api/tickets?tenant_id=beta' | jq -c '[.data.items[].key]'
# 建行时在请求体里点名租户；没点名就落在它登录的那个租户。
as "$ROOT" POST /api/tickets '{"code":"T-9","title":"filed for beta","tenant_id":"beta"}' | jq -c '.data | {tenant_id, key}'
as "$ROOT" POST /api/tickets '{"code":"T-10","title":"filed where root acts"}' | jq -c '.data | {tenant_id, key}'
```

列表答 `["alpha/T-1","beta/T-1","alpha/T-3"]`，带过滤答 `["beta/T-1"]`；点名的票答 `{"tenant_id":"beta","key":"beta/T-9"}`，没点名的答 `{"tenant_id":"alpha","key":"alpha/T-10"}`。

新租户上线：登记表只有 root 能写，第一个管理员由 root 经绑定指派，绑定的请求体点名租户：

```bash
as "$ALICE" POST /api/tenants '{"id":"gamma","name":"Gamma"}' | jq -c '.msg'
as "$ROOT" POST /api/tenants '{"id":"gamma","name":"Gamma"}' | jq -c '.data | {id, name}'
ERIN_ID=$(as "$ROOT" POST /api/iam/admin/users '{"username":"erin","password":"12345678","must_change_password":false}' | jq -r '.data.user.id')
as "$ROOT" POST /api/authz/role-bindings "{\"subject_id\":\"$ERIN_ID\",\"role_id\":\"admin\",\"tenant_id\":\"gamma\"}" | jq -c '.data | {tenant_id, role_id}'
ERIN=$(login erin gamma)
as "$ERIN" POST /api/tickets '{"code":"G-1","title":"first in gamma"}' | jq -c '.data | {key}'
as "$ERIN" GET /api/authz/menus | jq -c '[.data.items[].id]'
```

alice 建租户答 `only the system root may change the tenants`；root 建出 `{"id":"gamma","name":"Gamma"}`，绑定答 `{"tenant_id":"gamma","role_id":"admin"}`；erin 登进 gamma 建出 `{"key":"gamma/G-1"}`，作为管理员看到全部三个菜单。

### 五、租户从网关头来

有的部署里租户不是登录时选的，而是前面的网关按域名或路径认出来、放在请求头里。项目自己的中间件和拦截器（`middleware/gateway_tenant.go`、`interceptor/gateway_tenant.go`）在会话检查之后、授权之前读这个头，改写 `requestctx.KeyTenantID`。头名由 `[gateway] tenant_header` 配，带上它重启：

```bash
GATEWAY_TENANT_HEADER=X-Tenant-ID go run .
```

```bash
DAVE=$(login dave alpha); BOB=$(login bob alpha)
# dave 登在 alpha，不带头就在 alpha；带 X-Tenant-ID: beta 这次请求就在 beta 里，看到的是 beta 的票。
as "$DAVE" GET /api/memberships | jq -c '.data.current'
curl -s -A "$UA" -b "session_id=$DAVE" -H 'X-Tenant-ID: beta' $B/api/memberships | jq -c '.data.current'
curl -s -A "$UA" -b "session_id=$DAVE" -H 'X-Tenant-ID: beta' $B/api/tickets | jq -c '[.data.items[].key]'
# gRPC 上是同名的元数据键。
rpc "$DAVE" -H 'x-tenant-id: beta' -d '{}' localhost:8093 multitenant.TicketService/ListTicket | jq -c '[.items[].key]'
# 头只决定在哪个租户里判定，判定本身照旧：bob 在 beta 没有角色，带头也进不去。
curl -s -A "$UA" -b "session_id=$BOB" -H 'X-Tenant-ID: beta' $B/api/tickets | jq -c '.msg'
```

dave 不带头答 `"alpha"`，带头答 `"beta"`，票答 `["beta/T-1","beta/T-9"]`，gRPC 一样；bob 带头答 `permission denied`。访问日志和 authz 的判定日志里，这几次请求的 `tenant_id` 字段记的都是 `beta`。

这个头的值被原样信任：授权在它点名的租户里判定，行也只到那个租户。所以它只能在网关会剥掉客户端自带的同名头时才配；不配就留空，会话里的租户决定一切。

### 六、gRPC 上一样

声明了 `GRPC()` 的 `Ticket` 在 gRPC 上走同一套会话、授权和隔离：

```bash
ALICE=$(login alice alpha); CAROL=$(login carol beta); ROOT=$(login root alpha)
grpcurl -plaintext localhost:8093 list | grep -v '^grpc\.'
# 消息里的 tenant_id 只出不进：建行时它不被读，行盖的仍是请求所在的租户。
rpc "$ALICE" -d '{"ticket":{"code":"T-2","title":"over grpc","tenant_id":"beta"}}' localhost:8093 multitenant.TicketService/CreateTicket | jq -c '.ticket | {tenantId, key}'
# carol 列到的只有 beta 的票，取 alice 的票是 NotFound。
rpc "$CAROL" -d '{}' localhost:8093 multitenant.TicketService/ListTicket | jq -c '[.items[].key]'
T2_ID=$(as "$ALICE" GET '/api/tickets?code=T-2' | jq -r '.data.items[0].id')
rpc "$CAROL" -d "{\"id\":\"$T2_ID\"}" localhost:8093 multitenant.TicketService/GetTicket
# root 经 gRPC 建行点不了租户（HTTP 的请求体可以），行落在它登录的租户。
rpc "$ROOT" -d '{"ticket":{"code":"T-11","title":"root over grpc","tenant_id":"beta"}}' localhost:8093 multitenant.TicketService/CreateTicket | jq -c '.ticket | {tenantId, key}'
# 没有会话：Unauthenticated。
grpcurl -plaintext -d '{}' localhost:8093 multitenant.TicketService/ListTicket
```

列出 `multitenant.TicketService`；alice 的票答 `{"tenantId":"alpha","key":"alpha/T-2"}`；carol 列到 `["beta/T-1","beta/T-9"]`，取答 `Code: NotFound`；root 的票答 `{"tenantId":"alpha","key":"alpha/T-11"}`；没会话答 `Code: Unauthenticated`、`Message: no session`。

### 七、请求之外

定时任务和启动钩子在请求之外跑，没有授权来定租户，上下文默认在 `default` 租户里，要进哪个租户代码自己说：

```bash
# digest_tickets 每分钟一轮，用 tenant.In 轮流进每个租户数票、写一行汇总；行盖的是任务进的那个租户，
# 各租户只读到自己的。等过一个整分钟再看。
as "$ALICE" GET '/api/digests?_sort_by=at%20desc&_size=1' | jq -c '[.data.items[] | {tenant_id, open, closed, at}]'
as "$CAROL" GET '/api/digests?_sort_by=at%20desc&_size=1' | jq -c '[.data.items[] | {tenant_id, open, closed, at}]'
```

alice 读到 `[{"tenant_id":"alpha","open":5,"closed":0,"at":"...:00.0..Z"}]`，carol 读到 beta 的 `open` 为 `2`：各是本租户到那一分钟为止的票数。

`reconcile_policies` 每五分钟跨全部租户核对一次 authz 的策略，用的是模块给的 `authz.ReconcilePolicies`，结果在日志里（`logger` 为 `app`）：

```text
{"level":"INFO","ts":"2026-10-29T14:55:00.020074Z","logger":"app","caller":"cronjob/reconcile.go:27","msg":"authorization rules in sync","instance":"...","checked":21,"skipped":5}
```

启动钩子 `module/seed.go` 也是这样写的：登记表、账号、菜单是全局的，照原样写；每个租户的角色和绑定在 `tenant.In` 进的那个租户里写，行就盖上它。

### 八、工具链

`gg check` 拦下租户模型的两种错误写法。把 `model/ticket.go` 的索引改成只有 `Code`：

```text
  ✘ Tenant unique indexes (1)
  → model/ticket.go: model Ticket: unique index (Code) leaves out TenantID; a model embedding tenant.Scope is unique within a tenant, so every unique index names TenantID among its fields
```

给 `tenant.Scope` 加 `gorm:"embeddedPrefix:t_"`：

```text
  ✘ Framework columns (1)
  → model/ticket.go: model Ticket: framework field TenantID maps to column "t_tenant_id" instead of "tenant_id"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them
```

启动期同样会报错退出。迁移产物里租户列不可空、唯一索引带着它，authz 模块自己的角色表、绑定表也是同样的形状：

```bash
gg migrate --dry-run
grep -E 'tenant_id|uniq_tickets' generated/migrate/mysql/schema.sql
```

```text
  `tenant_id` varchar(191) NOT NULL DEFAULT 'default',
  `tenant_id` varchar(191) NOT NULL DEFAULT 'default',
CREATE UNIQUE INDEX `uniq_tickets_tenant_id_code` ON `tickets` (`tenant_id`, `code`);
  `tenant_id` varchar(191) NOT NULL DEFAULT 'default',
CREATE UNIQUE INDEX `uniq_roles_tenant_id_name` ON `roles` (`tenant_id`, `name`);
  `tenant_id` varchar(191) NOT NULL DEFAULT 'default',
CREATE UNIQUE INDEX `uniq_role_bindings_tenant_id_subject_id_role_id` ON `role_bindings` (`tenant_id`, `subject_id`, `role_id`);
```

`gg gen` 给 `.proto` 的消息带上 `tenant_id`（6 号，1 到 10 留给框架），生成的转换只往外写它：

```text
  // The tenant the record belongs to, stamped from the caller when the record is created.
  string tenant_id = 6;
```

`gg gen ts` 生成的 TypeScript 类型里它是普通字段，`Digest` 和 `Ticket` 各一处：

```bash
gg gen ts
grep 'tenant_id?' generated/typescript/model.ts
```

```text
  tenant_id?: string;
  tenant_id?: string;
```

## 测试

```bash
go test ./...
```

需要 Docker：testutil 起 Redis 容器，数据库用 sqlite，不起容器。测试只钉各组的核心断言：`service/ticket/create_test.go` 钉住建行盖章、别的租户看不见、code 在租户内唯一、挪不走、角色没有写菜单就读得了写不了、root 点名和不点名租户、gRPC 上的盖章；`service/membership/list_test.go` 钉住「我的租户」和当前租户。其余场景靠上面的命令核对。

## 正式环境

- `[gateway] tenant_header` 只在网关会剥掉客户端自带的同名头时才配，它的值被原样信任。
- 播种口令经 `SEED_PASSWORD` 注入；root 的口令只在第一次启动时写进库，之后改口令走 iam 的接口，换配置不会改掉已经播种的账号。
- 关掉 `auto_migrate`，表结构变更走 `gg migrate`。
