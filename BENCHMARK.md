# 压力测试

框架性能基线与压测方法。全部压测接口由 [examples/bench](./examples/bench) 项目提供，「启动被测服务」的命令一律在 `examples/bench` 目录下执行；压测工具使用 [hey](https://github.com/rakyll/hey)。

## 压测目标地址

压测命令通过 `BENCH_BASE_URL` 指向被测服务。值含协议和端口、结尾不带 `/`。URL 在命令里必须用双引号，单引号不展开变量。

```bash
export BENCH_BASE_URL=http://127.0.0.1:8080
```

```fish
set -x BENCH_BASE_URL http://127.0.0.1:8080
```

## 注意事项

- 协议：warmup `-z 10s` 丢弃 → cooldown → 3 轮 `-z 30s`，轮间 cooldown。cooldown 无 DB 10s、带 DB 15s
- 3 轮全部填表、不取平均；QPS 逐轮下降即降频，加长 cooldown 重测
- 只认同场 A/B：同一会话内交替跑「改前 3 轮 → 改后 3 轮 → 改前 3 轮」，不与历史表格比
- 减少分配一类的改动以 `go test -bench -benchmem` 的 `allocs/op` 为判据，QPS 仅作辅助
- hey 的 `[200] N` 是结果缓存上限（100 万），不是请求数；吞吐看 `Requests/sec`
- 本地一律用 `127.0.0.1`，不用 `localhost`：macOS 走 IPv6 回环，QPS 低约 40%（实测 13.1w vs 8w）
- 表内基线均为本地口径（`127.0.0.1` + pprof 开）；远程环境（CDN、共享数据库、日志与 OTel 全开）的数字不可比，不要填进来



## NO DB

### no otel

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true OTEL_ENABLED=false LOGGER_HTTP_BODY_ENABLED=false go run .


hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/ping" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/ping"              # 压测
```

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 131548 | 0.3ms | 0.6ms | 1.1ms | 0     |
| 50   | 131695 | 0.3ms | 0.5ms | 1.1ms | 0     |
| 50   | 129887 | 0.4ms | 0.6ms | 1.2ms | 0     |

### with otel

```bash
# 启动被测服务（仅本地）
# 采样率为0
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=true OTEL_TRACES_SAMPLER=parentbased_traceidratio OTEL_TRACES_SAMPLER_ARG=0 go run .
# 采样率为100%
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=true OTEL_TRACES_SAMPLER=parentbased_traceidratio OTEL_TRACES_SAMPLER_ARG=1 go run .

hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/ping" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/ping"              # 压测
```

#### 采样率为 0

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 130585 | 0.3ms | 0.6ms | 1.1ms | 0     |
| 50   | 131536 | 0.3ms | 0.6ms | 1.1ms | 0     |
| 50   | 130220 | 0.3ms | 0.6ms | 1.2ms | 0     |

#### 采样率为 1

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 110695 | 0.4ms | 0.7ms | 1.7ms | 0     |
| 50   | 111266 | 0.4ms | 0.7ms | 1.8ms | 0     |
| 50   | 111219 | 0.4ms | 0.8ms | 1.8ms | 0     |

### 裸 gin 对照

量化框架完整中间件链相对 gin 底座的净开销：用同版本 gin 的 `gin.New()` 零中间件程序返回与 ping 相同形状的 JSON envelope，同参数、同场交替对比。响应必须用结构体而不是 `gin.H`：框架 envelope 是结构体，map 序列化要排序键，用 `gin.H` 会让对照侧凭空多一笔序列化开销（实测 map 版比结构体版低约 6%）。

对照程序（独立 module：`go mod init ginbare && go get github.com/gin-gonic/gin@<与框架同版本>`）：

```go
package main

import "github.com/gin-gonic/gin"

type pingData struct {
	Msg string `json:"msg"`
}

type envelope struct {
	Code    int      `json:"code"`
	Data    pingData `json:"data"`
	Msg     string   `json:"msg"`
	TraceID string   `json:"trace_id"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/bench/ping", func(c *gin.Context) {
		c.JSON(200, envelope{
			Code:    0,
			Data:    pingData{Msg: "pong"},
			Msg:     "success",
			TraceID: "0000000000000000000000",
		})
	})
	_ = r.Run("127.0.0.1:8082")
}
```

```bash
hey -z 10s -c 50 "http://127.0.0.1:8082/api/bench/ping" > /dev/null  # 热身
hey -z 30s -c 50 "http://127.0.0.1:8082/api/bench/ping"              # 压测
```

对照必须同场交替跑（裸 gin 3 轮 → 框架 ping 3 轮 → 各回验 1 轮确认无漂移），不跨场比较。下表为同场口径（与上方 no otel 基线同环境，数字可互相印证），gin v1.12.0：

#### 裸 gin（gin.New() 零中间件）

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 125657 | 0.4ms | 0.6ms | 1.2ms | 0     |
| 50   | 125443 | 0.4ms | 0.6ms | 1.2ms | 0     |
| 50   | 125082 | 0.4ms | 0.6ms | 1.2ms | 0     |

#### 框架 ping（完整中间件链，同场）

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 131091 | 0.3ms | 0.6ms | 1.2ms | 0     |
| 50   | 131534 | 0.3ms | 0.6ms | 1.2ms | 0     |
| 50   | 131248 | 0.3ms | 0.6ms | 1.1ms | 0     |

同场结论：框架带完整中间件链（tracing、access log 落盘、body logger、CORS、recovery、trace_id 生成）的 ping 吞吐反而比裸 gin 高约 4.7%——**中间件链净开销为零**，框架响应写出路径的效率收益覆盖了全部中间件成本。



## With DB

带 DB 压测统一使用本地 MySQL 容器：与生产同引擎（InnoDB），连接池语义真实（框架默认 `max_open_conns=100`）。sqlite 内存模式被框架钳制为单连接（shared-cache 表锁所致，WAL 不支持内存库），QPS 反映的是单连接串行化瓶颈而非框架吞吐，不再用于压测。

```bash
# 一次性启动本地压测 MySQL（tmpfs 数据目录 + 关 binlog，压低引擎噪声）
docker run -d --name gst-bench-mysql \
  -e MYSQL_ROOT_PASSWORD=bench -e MYSQL_DATABASE=bench \
  -p 127.0.0.1:3307:3306 --tmpfs /var/lib/mysql mysql:8 --skip-log-bin

# 清空压测数据（跑过 create 后先清表再测 list/get）
docker exec gst-bench-mysql mysql -uroot -pbench bench -e "TRUNCATE TABLE benches;"
```

### list

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# Q 覆盖 WithQuery 两个查询参数的全部 URL 能力：13 个操作符（eq/ne/gt/gte/lt/lte/in/notin/like/notlike/startswith/endswith/isnull）、
# 模型等值条件四种形态（field1 string、field2 int、field3 指针、field5 bool 显式零值经 PresentFields 保留）、
# created_at[gte] 时间范围与 updated_at 裸键时间等值。条件间语义一致，命中集非空。
export Q='field1=hello&field1%5Bne%5D=abc&field2=5&field2%5Bgt%5D=0&field2%5Bgte%5D=1&field2%5Blt%5D=100&field2%5Blte%5D=99&field3=world&field3%5Blike%5D=wor&field3%5Bnotlike%5D=xyz&field3%5Bstartswith%5D=wo&field3%5Bendswith%5D=ld&field3%5Bisnull%5D=false&field4%5Beq%5D=2&field4%5Bin%5D=1,2,3&field4%5Bnotin%5D=8,9&field5=false&created_at%5Bgte%5D=2020-01-01T00:00:00Z&updated_at=2020-01-01T00:00:00Z'

hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/list?$Q" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/list?$Q"              # 压测
```

list 必须空表压测：create 压测灌进去的行会全部命中上面的查询条件，每个请求的 count+select 两条 SQL 都变成全表扫描（MySQL 口径实测 38 万行时单请求 1.2s、两条各 ~580ms；sqlite 旧口径 100 万行时 QPS 从 1.3w 跌到 8.6）。跑过 create 后先用上面的 TRUNCATE 命令清表再测 list。

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 16656 | 2.1ms | 3.0ms | 4.3ms | 0     |
| 50   | 16209 | 2.1ms | 3.1ms | 4.4ms | 0     |
| 50   | 16505 | 2.0ms | 2.9ms | 4.2ms | 0     |

#### dry run

不适用：list 走标准框架 List 动作，控制器没有 dry run 分支。`dry_run=true` 仅因模型带 `DryRun` 字段而被接受（否则 400 unsupported query parameter），SQL 仍真实执行，与 no dry run 同一条路径。剥离 DB I/O 看框架开销用 list2 或 pprof。



### list2

list2 是自定义 List 动作，完整复刻标准 List 的查询解析与条件构建（模型条件、Filters、排序、游标、显式零值、分页、SELECT/COUNT 生成），但全程 dry run、不产生数据库 I/O，作为 list 的「无 DB」对照口径压测框架查询能力。表内容不影响结果，无需空表。

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# Q 与 list 小节相同
hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/list2?$Q" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/list2?$Q"              # 压测
```

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 67050 | 0.5ms | 1.6ms | 3.4ms | 0     |
| 50   | 67558 | 0.5ms | 1.5ms | 3.2ms | 0     |
| 50   | 67766 | 0.5ms | 1.5ms | 3.3ms | 0     |



### get

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# no dry run
hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/get" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/get"              # 压测

# with dry run
hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/get?dry_run=true" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/get?dry_run=true"              # 压测
```

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 34344 | 1.0ms | 1.6ms | 2.3ms | 0     |
| 50   | 35892 | 1.0ms | 1.6ms | 2.3ms | 0     |
| 50   | 35129 | 1.1ms | 1.6ms | 2.4ms | 0     |

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 120019 | 0.4ms | 0.7ms | 1.2ms | 0     |
| 50   | 118866 | 0.4ms | 0.7ms | 1.2ms | 0     |
| 50   | 120021 | 0.4ms | 0.7ms | 1.2ms | 0     |

### create

**每轮压测结束后记得清空数据库**

```bash
docker exec gst-bench-mysql mysql -uroot -pbench bench -e "TRUNCATE TABLE benches;"
```



```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .
# no dry run
# 热身
hey -z 10s -c 50 -m POST -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/create" > /dev/null

# 压测
hey -z 30s -c 50 -m POST -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/create"


# dry run
# 热身
hey -z 10s -c 50 -m POST -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/create?dry_run=true" > /dev/null

# 压测
hey -z 30s -c 50 -m POST -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/create?dry_run=true"
```

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 26866 | 1.5ms | 2.5ms | 4.6ms | 0     |
| 50   | 27234 | 1.5ms | 2.5ms | 4.7ms | 0     |
| 50   | 26870 | 1.6ms | 2.6ms | 5.0ms | 0     |

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 112657 | 0.4ms | 0.8ms | 1.7ms | 0     |
| 50   | 114227 | 0.4ms | 0.7ms | 1.7ms | 0     |
| 50   | 113548 | 0.4ms | 0.7ms | 1.7ms | 0     |



### update

update 压测框架标准 Update 写路径（PUT + JSON body）。接口对**不存在的主键**执行一条真实 UPDATE：完整走参数绑定、钩子判定、事务分流和 DB 往返，但命中 0 行、不写任何数据；框架返回的 ErrRecordNotFound 由接口吞掉视为成功。URL 里的 `:id` 只是路由占位，service 内固定使用不存在的主键。因此无需播种、无需清表，表内容不影响口径。

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# no dry run
hey -z 10s -c 50 -m PUT -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/update/bench-missing" > /dev/null  # 热身
hey -z 30s -c 50 -m PUT -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/update/bench-missing"              # 压测

# dry run
hey -z 10s -c 50 -m PUT -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/update/bench-missing?dry_run=true" > /dev/null  # 热身
hey -z 30s -c 50 -m PUT -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/update/bench-missing?dry_run=true"              # 压测
```

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 35723 | 1.2ms | 1.9ms | 3.4ms | 0     |
| 50   | 36857 | 1.2ms | 1.8ms | 3.0ms | 0     |
| 50   | 35789 | 1.2ms | 1.9ms | 3.0ms | 0     |

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 106754 | 0.4ms | 0.8ms | 2.0ms | 0     |
| 50   | 105648 | 0.4ms | 0.8ms | 1.9ms | 0     |
| 50   | 105413 | 0.4ms | 0.8ms | 1.9ms | 0     |



### delete

delete 压测框架标准 Delete 写路径（DELETE，无 body）。接口对**不存在的主键**执行一条真实 DELETE 语句（`Bench.Purge()` 为 true，模型删除策略是物理删除）：完整走钩子判定、事务分流和 DB 往返，但命中 0 行、不改任何数据（Delete 命中 0 行不报错）。URL 里的 `:id` 只是路由占位，service 内固定使用不存在的主键。因此无需播种、无需清表，表内容不影响口径。

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# no dry run
hey -z 10s -c 50 -m DELETE "$BENCH_BASE_URL/api/bench/delete/bench-missing" > /dev/null  # 热身
hey -z 30s -c 50 -m DELETE "$BENCH_BASE_URL/api/bench/delete/bench-missing"              # 压测

# dry run
hey -z 10s -c 50 -m DELETE "$BENCH_BASE_URL/api/bench/delete/bench-missing?dry_run=true" > /dev/null  # 热身
hey -z 30s -c 50 -m DELETE "$BENCH_BASE_URL/api/bench/delete/bench-missing?dry_run=true"              # 压测
```

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 38936 | 1.1ms | 1.7ms | 2.8ms | 0     |
| 50   | 37938 | 1.2ms | 1.8ms | 2.8ms | 0     |
| 50   | 38630 | 1.1ms | 1.7ms | 2.7ms | 0     |

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 118692 | 0.4ms | 0.7ms | 1.5ms | 0     |
| 50   | 117844 | 0.4ms | 0.7ms | 1.6ms | 0     |
| 50   | 117916 | 0.4ms | 0.7ms | 1.5ms | 0     |



### updatebyid

updatebyid 压测 database.UpdateByID 写路径（PATCH + JSON body）：无钩子、无事务包装，是框架最轻的写路径，可与 update 对比出钩子判定与事务分流的开销。接口对**不存在的主键**执行一条真实 UPDATE，命中 0 行、不写任何数据（UpdateByID 对不存在的记录返回 nil）。URL 里的 `:id` 只是路由占位，service 内固定使用不存在的主键。因此无需播种、无需清表，表内容不影响口径。

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true LOGGER_HTTP_BODY_ENABLED=false OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=bench MYSQL_USERNAME=root MYSQL_PASSWORD=bench go run .

# no dry run
hey -z 10s -c 50 -m PATCH -H 'Content-Type: application/json' \
  -d '{"field1":"hello"}' \
  "$BENCH_BASE_URL/api/bench/updatebyid/bench-missing" > /dev/null  # 热身
hey -z 30s -c 50 -m PATCH -H 'Content-Type: application/json' \
  -d '{"field1":"hello"}' \
  "$BENCH_BASE_URL/api/bench/updatebyid/bench-missing"              # 压测

# dry run
hey -z 10s -c 50 -m PATCH -H 'Content-Type: application/json' \
  -d '{"field1":"hello"}' \
  "$BENCH_BASE_URL/api/bench/updatebyid/bench-missing?dry_run=true" > /dev/null  # 热身
hey -z 30s -c 50 -m PATCH -H 'Content-Type: application/json' \
  -d '{"field1":"hello"}' \
  "$BENCH_BASE_URL/api/bench/updatebyid/bench-missing?dry_run=true"              # 压测
```

#### no dry run

| -c   | QPS   | p50   | p90   | p99   | 非200 |
| ---- | ----- | ----- | ----- | ----- | ----- |
| 50   | 37187 | 1.2ms | 1.8ms | 3.0ms | 0     |
| 50   | 37707 | 1.2ms | 1.8ms | 2.9ms | 0     |
| 50   | 37943 | 1.2ms | 1.8ms | 2.8ms | 0     |

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 |
| ---- | ------ | ----- | ----- | ----- | ----- |
| 50   | 113269 | 0.4ms | 0.8ms | 1.6ms | 0     |
| 50   | 112875 | 0.4ms | 0.8ms | 1.7ms | 0     |
| 50   | 111534 | 0.4ms | 0.8ms | 1.7ms | 0     |



## pprof 定量实证

QPS 只回答「多快」，pprof 占比回答「慢在谁」——占比结构跨机器稳定，比 QPS 更能说明框架自身是否是瓶颈。两个口径互补：ping（纯框架路径，无 DB 稀释，是框架自身成本的放大镜）与 update/delete/updatebyid（带 DB 的真实写路径）。写路径三个接口各采 30s CPU profile 与 15s allocs profile，构成完全同构，以 update 为代表。

```bash
# 负载中采样（服务需开 DEBUG_PPROF_ENABLED=true DEBUG_PPROF_PORT=10002）
# ping 口径
hey -z 50s -c 50 "$BENCH_BASE_URL/api/bench/ping" > /dev/null &
curl -s -o cpu_ping.pb.gz "http://127.0.0.1:10002/debug/pprof/profile?seconds=30"
# update 口径
hey -z 50s -c 50 -m PUT -H 'Content-Type: application/json' \
  -d '{"field1":"hello","field2":1,"field3":"world","field4":2}' \
  "$BENCH_BASE_URL/api/bench/update/bench-missing" > /dev/null &
curl -s -o cpu.pb.gz "http://127.0.0.1:10002/debug/pprof/profile?seconds=30"
curl -s -o allocs.pb.gz "http://127.0.0.1:10002/debug/pprof/allocs?seconds=15"

# 分析
go tool pprof -top -cum <被测二进制> cpu.pb.gz
go tool pprof -top -hide="^(syscall|runtime|internal/poll)\." <被测二进制> cpu.pb.gz
go tool pprof -top -sample_index=alloc_space <被测二进制> allocs.pb.gz
```

### CPU 构成（ping，纯框架路径，30s 采样）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| 网络写 `net.(*netFD).Write` | ~49% | HTTP 响应发送 |
| 网络读 `net.(*netFD).Read` | ~25% | HTTP 请求接收 |
| gin 入口到响应写完的整条应用层链路 | cum ~2.1% | tracing 2.08% → access log 1.98% → body logger 1.00% → recovery 0.99% → CORS 0.99% → route params 0.95% → controller 0.90% → 响应序列化 0.43%，逐层递减 |
| 其余 | ~24% | Go runtime（GC、调度）与 HTTP 库缓冲 |

零 DB 的路径上，**gin + 框架全部应用层逻辑合计只占 ~2% CPU**，其余全是网络收发与 runtime——框架自身路径没有任何隐藏热点，这也是裸 gin 对照里中间件链测不出净开销的原因。

### CPU 构成（update，带 DB 路径，30s 采样，约 395% CPU）

| 构成 | flat 占比 | 说明 |
| --- | --- | --- |
| 网络写 `net.(*netFD).Write` | ~31% | HTTP 响应发送 + MySQL 请求发送 |
| MySQL driver 连接探活 `connCheck` | ~20% | 每次从连接池复用连接前的非阻塞读探活 |
| 网络读 `net.(*netFD).Read` | ~17% | HTTP 请求接收 + MySQL 响应接收 |
| gorm SQL 日志 `GormLogger.Trace` | cum ~1.2% | SQL 日志构造与落盘 |
| 框架自有函数 | 每项 <0.5%，合计 <5% | 最大几项：access log 中间件 ~0.6%、SQL caller 定位 ~0.4%、响应序列化 ~0.5% |

约 70% 的 CPU 花在网络 syscall 上；中间件链从 gin 入口到 service 的逐层损耗合计不足 2%。

### 分配构成（update，alloc_space）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| gorm 语句构建 | ~37% | `Statement.clone`、`SelectAndOmitColumns`、`AddClause`、`Session` 等，gorm 内部机制 |
| 框架自有 | ~13% | 请求上下文、日志元数据、SQL 日志构造，分散无单点 |
| zap 日志缓冲 | ~7% | bufferpool 扩容 |

### 结论

- 纯框架路径（ping）上应用层合计仅 ~2% CPU；带 DB 路径（update）上框架自有 CPU 份额 <5%、分配份额 ~13%，均无超过 5% 的单点热点：吞吐由网络往返数与数据库决定，**框架不是瓶颈**。
- 唯一超过 5% 的可动候选是 MySQL driver 的 `connCheck`（~20% CPU）：DSN 加 `checkConnLiveness=false` 可整个省掉，但被服务端或 LB 闲置回收的陈旧连接会把失败直接暴露给业务首请求，属可靠性换吞吐，默认不动；且本压测口径语句极轻（命中 0 行），真实业务下该占比会被稀释。
- 分配大头在 gorm 语句构建，属 ORM 内部机制；绕开等于手写 SQL，是重构不是优化，不做。

