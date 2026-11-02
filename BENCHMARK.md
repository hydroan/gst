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

- 协议：warmup `-z 10s` 丢弃 → cooldown → 3 轮 `-z 30s`，轮间 cooldown。cooldown 无 DB 至少 10s、带 DB 至少 15s，并且等整机空闲率回到压测前水平（减 4 个百分点以内）、连续两次采样都满足再开下一轮，上限 300s；本页数字 cooldown 实际 17–82s，最长的几次在 create 之后，MySQL 在后台清理刚灌进去的 90 万行
- 3 轮全部填表、不取平均；QPS 逐轮下降超过 1% 即降频，加长 cooldown 重测（1% 以内是轮间噪声）
- 每轮校对：hey 的非 200 为 0；服务端各日志流零 ERROR、零 WARN；create 每轮落库行数等于 hey 的 `[200]` 数
- 跑时另开一个每 10 秒记一次各进程 CPU 的监视（`ps -Aro %cpu=,comm= | head -8`），跑完核对杀毒扫描、Spotlight 索引、照片分析这类后台任务没有落进任何一轮；落进了就加长冷却重测那组
- 「服务 CPU 核」与「µs/请求」：每轮前后读被测进程的累计 CPU 时间（`ps -o cputime`），差值除以 hey 的 Total 秒数是核数，差值除以请求数是每请求微秒（等于核数 ÷ QPS）。QPS 会被同机其它进程挤占，这一列几乎不受影响，是跨机器最可比的数字
- 「每请求分配」：3 轮之后另起 20 秒同样的负载，取其中 15 秒的 allocs profile（`/debug/pprof/allocs?seconds=15`），分配总字节与对象数除以窗口内的请求数（该段 QPS × 15）
- 压测中整机被压满（空闲 0–10%）：被测服务占 3.3–7.7 核，其余被 hey 与系统占满，所以同机其它进程会直接从 QPS 里扣掉，QPS 只在同一台空闲的机器上、同一场里可比；要跨机器比看 µs/请求与每请求分配
- 只认同场 A/B：同一会话内交替跑「改前 3 轮 → 改后 3 轮 → 改前 3 轮」，不与历史表格比
- 减少分配一类的改动以 `go test -bench -benchmem` 的 `allocs/op` 为判据，QPS 仅作辅助
- hey 的 `[200] N` 是结果缓存上限（100 万），不是请求数；吞吐看 `Requests/sec`
- 本地一律用 `127.0.0.1`，不用 `localhost`：macOS 走 IPv6 回环，QPS 低约 40%（实测 13.1w vs 8w）
- 表内基线均为本地口径（`127.0.0.1` + pprof 开）；远程环境（CDN、共享数据库、日志与 OTel 全开）的数字不可比，不要填进来
- 本页数字测于 2026-11-01（电脑重启后的空闲机器）：Apple M4 Pro（14 核）、macOS 27.2、Go 1.27.1、gin v1.12.0、hey 0.1.5，MySQL 8.4 容器跑在 OrbStack 里



## NO DB

### no otel

```bash
# 启动被测服务（仅本地）
DEBUG_PPROF_ENABLED=true OTEL_ENABLED=false LOGGER_HTTP_BODY_ENABLED=false go run .


hey -z 10s -c 50 "$BENCH_BASE_URL/api/bench/ping" > /dev/null  # 热身
hey -z 30s -c 50 "$BENCH_BASE_URL/api/bench/ping"              # 压测
```

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 93003 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.93 | 53.0 |
| 50   | 93389 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.96 | 53.1 |
| 50   | 93293 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.96 | 53.2 |

每请求分配 6.6 KB、66 个对象。

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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 93311 | 0.5ms | 0.6ms | 1.0ms | 0     | 5.05 | 54.1 |
| 50   | 93343 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.92 | 52.7 |
| 50   | 94020 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.98 | 53.0 |

每请求分配 7.2 KB、78 个对象。

#### 采样率为 1

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 84207 | 0.5ms | 0.8ms | 1.6ms | 0     | 5.46 | 64.9 |
| 50   | 84149 | 0.5ms | 0.8ms | 1.6ms | 0     | 5.44 | 64.6 |
| 50   | 84851 | 0.5ms | 0.8ms | 1.6ms | 0     | 5.47 | 64.4 |

每请求分配 19.0 KB、156 个对象。

## 框架相对 gin 的开销

压测的绝对值随机器、系统版本和后台负载变化，框架相对底座的差距才是跨环境可比的数字。这一章用同版本 gin 的 `gin.New()` 零中间件程序返回与 ping 相同形状的 JSON envelope，同参数、同场交替与框架 ping 对比，量框架完整中间件链（tracing、access log 落盘、body logger、CORS、recovery、trace_id 生成）加控制器与信封编码相对 gin 底座的净开销。对照程序用结构体响应，与框架 envelope 的编码方式一致（internal/response 用结构体经 encoding/json 编码）。零 DB 的路径只量 HTTP 层；带 DB 的路径里框架在 ORM 之上那一层还没有对照程序。

对照程序（独立 module：`go mod init ginbare && go get github.com/gin-gonic/gin@<与框架同版本>`；pprof 挂在 8083，供分配采样）：

```go
package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/gin-gonic/gin"
)

type pingData struct {
	Msg string `json:"msg"`
}

type envelope struct {
	Data    pingData `json:"data"`
	Msg     string   `json:"msg"`
	TraceID string   `json:"trace_id"`
}

func main() {
	go func() { _ = http.ListenAndServe("127.0.0.1:8083", nil) }()
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/bench/ping", func(c *gin.Context) {
		c.JSON(200, envelope{
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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 93612 | 0.5ms | 0.6ms | 1.1ms | 0     | 4.51 | 48.2 |
| 50   | 93771 | 0.5ms | 0.6ms | 1.1ms | 0     | 4.52 | 48.3 |
| 50   | 93555 | 0.5ms | 0.6ms | 1.1ms | 0     | 4.52 | 48.4 |

每请求分配 2.4 KB、22 个对象。

#### 框架 ping（完整中间件链，同场）

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 93650 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.90 | 52.3 |
| 50   | 93180 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.95 | 53.1 |
| 50   | 93391 | 0.5ms | 0.6ms | 1.0ms | 0     | 4.94 | 52.9 |

每请求分配 6.5 KB、63 个对象。

#### 相对值（同场三轮均值）

| 指标（同场三轮均值） | 裸 gin | 框架 ping | 框架 / gin |
| --- | --- | --- | --- |
| QPS | 93646 | 93407 | 1.00 |
| 每请求服务 CPU（µs） | 48.3 | 52.8 | 1.09 |
| 每请求分配（KB） | 2.4 | 6.5 | 2.71 |
| 每请求分配（对象数） | 22 | 63 | 2.86 |
| p99（ms，三轮最大） | 1.1 | 1.0 | 0.91 |

同场结论：吞吐上测不出中间件链的开销，差距在每请求 CPU 与分配上，见相对值表；pprof 把其中 gin 入口到响应写完的应用层链路计为服务进程 CPU 的 ~2.5%，其余是访问日志落盘的写系统调用与 GC 的差异。

> **脚注：GC 触发频率。** Go 按「堆翻倍」触发 GC（GOGC=100），裸 gin 程序活堆只有 ~1MB，框架进程常驻堆大（日志缓冲、连接池、组件注册表），两者的 GC 频率相差数倍，但对吞吐的影响在 1–2% 内。一轮 30s 内的 GC 次数：裸 gin 5096 次（QPS 93343），框架 1387 次（QPS 93042），裸 gin 设 `GOGC=1000` 后 189 次（QPS 94674，比默认 +1.4%）。GC 次数用 `GODEBUG=gctrace=1` 启动被测程序统计。


## With DB

带 DB 压测统一使用本地 MySQL 容器：与生产同引擎（InnoDB），连接池语义真实（框架默认 `max_open_conns=100`）。sqlite 内存模式被框架钳制为单连接（shared-cache 表锁所致，WAL 不支持内存库），QPS 反映的是单连接串行化瓶颈而非框架吞吐，不用于压测。

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

list 必须空表压测：create 压测灌进去的行会全部命中上面的查询条件，每个请求的 count+select 两条 SQL 都变成全表扫描（MySQL 口径实测 38 万行时单请求 1.2s、两条各 ~580ms）。跑过 create 后先用上面的 TRUNCATE 命令清表再测 list。

#### no dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 18253 | 2.1ms | 3.1ms | 4.6ms | 0     | 4.12 | 225.7 |
| 50   | 18630 | 2.0ms | 2.8ms | 4.0ms | 0     | 4.34 | 233.2 |
| 50   | 18763 | 2.0ms | 2.9ms | 4.0ms | 0     | 4.32 | 230.1 |

每请求分配 61.0 KB、816 个对象。

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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 73917 | 0.5ms | 1.2ms | 2.6ms | 0     | 7.64 | 103.4 |
| 50   | 74097 | 0.5ms | 1.2ms | 2.5ms | 0     | 7.68 | 103.6 |
| 50   | 73934 | 0.5ms | 1.2ms | 2.6ms | 0     | 7.62 | 103.0 |

每请求分配 49.2 KB、717 个对象。



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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 37969 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.17 | 109.9 |
| 50   | 38125 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.27 | 112.0 |
| 50   | 37728 | 1.2ms | 1.7ms | 2.3ms | 0     | 4.27 | 113.2 |

每请求分配 18.3 KB、194 个对象。

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 91723 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.65 | 61.6 |
| 50   | 91703 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.62 | 61.2 |
| 50   | 91162 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.65 | 62.0 |

每请求分配 16.8 KB、160 个对象。

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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 30091 | 1.4ms | 2.2ms | 3.7ms | 0     | 3.52 | 116.8 |
| 50   | 31008 | 1.4ms | 2.2ms | 3.5ms | 0     | 3.50 | 112.9 |
| 50   | 30840 | 1.4ms | 2.2ms | 3.5ms | 0     | 3.57 | 115.9 |

每请求分配 25.6 KB、214 个对象。

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 90240 | 0.5ms | 0.8ms | 1.4ms | 0     | 6.04 | 66.9 |
| 50   | 90413 | 0.5ms | 0.7ms | 1.3ms | 0     | 6.03 | 66.7 |
| 50   | 90190 | 0.5ms | 0.7ms | 1.3ms | 0     | 6.03 | 66.8 |

每请求分配 20.5 KB、175 个对象。



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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 36243 | 1.2ms | 1.8ms | 2.6ms | 0     | 4.54 | 125.2 |
| 50   | 35816 | 1.2ms | 1.8ms | 2.6ms | 0     | 4.54 | 126.8 |
| 50   | 36055 | 1.2ms | 1.8ms | 2.6ms | 0     | 4.53 | 125.7 |

每请求分配 29.6 KB、242 个对象。

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 88936 | 0.5ms | 0.8ms | 1.5ms | 0     | 6.27 | 70.5 |
| 50   | 89118 | 0.5ms | 0.8ms | 1.4ms | 0     | 6.26 | 70.2 |
| 50   | 89297 | 0.5ms | 0.8ms | 1.5ms | 0     | 6.26 | 70.1 |

每请求分配 25.2 KB、203 个对象。



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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 37520 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.12 | 109.8 |
| 50   | 38656 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.17 | 108.0 |
| 50   | 38162 | 1.2ms | 1.7ms | 2.5ms | 0     | 4.09 | 107.1 |

每请求分配 21.0 KB、171 个对象。

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 92345 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.75 | 62.3 |
| 50   | 92420 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.75 | 62.3 |
| 50   | 92245 | 0.5ms | 0.7ms | 1.2ms | 0     | 5.77 | 62.5 |

每请求分配 18.2 KB、150 个对象。



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

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 37827 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.27 | 112.9 |
| 50   | 37716 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.24 | 112.3 |
| 50   | 37914 | 1.2ms | 1.7ms | 2.4ms | 0     | 4.25 | 112.0 |

每请求分配 21.7 KB、199 个对象。

#### dry run

| -c   | QPS    | p50   | p90   | p99   | 非200 | 服务 CPU 核 | µs/请求 |
| ---- | ------ | ----- | ----- | ----- | ----- | ----------- | ------- |
| 50   | 90705 | 0.5ms | 0.7ms | 1.3ms | 0     | 5.99 | 66.1 |
| 50   | 90578 | 0.5ms | 0.7ms | 1.3ms | 0     | 6.00 | 66.2 |
| 50   | 90939 | 0.5ms | 0.7ms | 1.3ms | 0     | 6.00 | 66.0 |

每请求分配 20.2 KB、179 个对象。



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

### CPU 构成（ping，纯框架路径，30s 采样，约 471% CPU）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| 网络写 `net.(*netFD).Write` | ~38% | HTTP 响应发送 |
| 网络读 `net.(*netFD).Read` | ~24% | HTTP 请求接收 |
| gin 入口到响应写完的整条应用层链路 | cum ~2.5% | tracing 2.4% → access log 2.2% → recovery 1.1% → controller 1.0%，逐层递减 |
| 其余 | ~36% | Go runtime：调度器等活（`findRunnable` ~32%，服务端在等客户端）、GC ~1% |

零 DB 的路径上，**gin + 框架全部应用层逻辑合计只占 ~2.5% CPU**，其余全是网络收发与 runtime——框架自身路径没有任何隐藏热点，这也是裸 gin 对照里吞吐测不出中间件链开销的原因。

### CPU 构成（update，带 DB 路径，30s 采样，约 429% CPU）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| 网络写 `net.(*netFD).Write` | ~30% | HTTP 响应发送 + MySQL 请求发送 |
| 网络读 `net.(*netFD).Read` | ~20% | HTTP 请求接收 + MySQL 响应接收 |
| MySQL driver 连接探活 `connCheck` | ~17% | 每次从连接池复用连接前的非阻塞读探活；list 每请求两条 SQL，探活占到 ~27% |
| GC（`gcDrain` + 后台标记） | ~4% | 每请求约 29.5KB 分配的代价 |
| gorm SQL 日志 `GormLogger.Trace` | cum ~1.0% | SQL 日志构造与落盘 |
| 框架自有函数 | 每项 <0.5%，合计 <5% | 分散在访问日志、请求上下文、响应编码 |

约 70% 的 CPU 花在网络 syscall 上；中间件链从 gin 入口到 service 的逐层损耗合计不足 2%。

### 分配构成（update，alloc_space，每请求约 29.5KB）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| gorm 语句构建 | ~45% | `Statement.clone` 14%、`SelectAndOmitColumns` 16%、`AddClause` 5%、`Session` 3%、`ConvertToAssignments` 3%、`clause.Set` 4%，gorm 内部机制 |
| 框架自有 | ~15% | 请求级 logger 的字段 5%、SQL 日志条目 3%、访问日志条目 3%、请求元数据与路由参数表 4% |
| HTTP 请求读取 | ~8% | 头部解析、请求体读取、`Request.WithContext` |

### 分配构成（list2，alloc_space，每请求约 49KB）

| 构成 | 占比 | 说明 |
| --- | --- | --- |
| gorm 语句构建 | ~35% | `AddClause` 11.6%、`clone` 10.3%、`AddVar` 4.3%、`Quote` 2.5%、`Session` 1.9%、`Where.MergeClause` 1.9%、`BuildCondition` 1.8%，两条语句（SELECT 与 COUNT）各一份 |
| 框架查询构建 | ~15% | 模型等值条件 `structConditions` 4.5%、过滤器渲染（`scalarFilter`、`patternFilter`、`quoteIdent`）~4%、URL 过滤解析 `urlquery.Filters` 2.7%、`PresentFields` 1.9%、操作日志字段 1.3% |
| 查询串解析 `url.ParseQuery` | ~5.7% | 严格解析一次，19 个键 |
| 请求级 logger 的字段 | ~4.3% | 控制器为 service 派生的 logger，加每次数据库操作结束时派生的那个 |

### 结论

- 纯框架路径（ping）上应用层合计仅 ~2.5% CPU；带 DB 路径（update）上框架自有 CPU 份额 <5%、分配份额 ~15%，均无超过 5% 的单点热点：吞吐由网络往返数与数据库决定，**框架不是瓶颈**。
- 唯一超过 5% 的可动候选是 MySQL driver 的 `connCheck`（update ~17%、list ~27% CPU）：DSN 加 `checkConnLiveness=false` 可整个省掉，但被服务端或 LB 闲置回收的陈旧连接会把失败直接暴露给业务首请求，属可靠性换吞吐，默认不动；且本压测口径语句极轻（命中 0 行），真实业务下该占比会被稀释。
- 分配大头在 gorm 语句构建，属 ORM 内部机制；绕开等于手写 SQL，是重构不是优化，不做。框架自己的查询构建每请求约 7KB（list2），分散在条件渲染与过滤解析上，没有单点。

