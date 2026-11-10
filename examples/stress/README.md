# stress

压测项目：回答「框架在真实项目形态下，每类接口的开销与极限是多少」。结论、总表和每个场景的数字都在仓库根的 [STRESS.md](../../STRESS.md)，这里只讲这个项目长什么样、怎么跑。

## 目录

| 位置 | 内容 |
|---|---|
| `model/record.go`、`service/record/` | `Record`：声明了 `GRPC()`，唯一的 `code`，`category + created_at`、`created_at` 两条索引，默认 CRUD 加 `DeleteMany`，`Export`（CSV），`records/upsert` 路由按主键合并 |
| `model/order.go`、`model/order/line.go` | `Order` 带明细 `Line`，`_expand=lines` 预加载；`orders/public` 是同一张表的公开路由，作带登录态与不带的对照 |
| `model/ticket.go` | `Ticket` 嵌 `tenant.Scope`，按租户隔离 |
| `model/ping.go`、`service/ping/` | 公开探活接口 |
| `module/` | iam 与 authz；启动钩子播种 `reader` 账号和它的只读角色，以及租户 t-03 的成员 `member` 和它只读 tickets 的角色 |
| `scripts/` | 场景脚本 `s1_auth.sh` … `s10_overload.sh`、灌数据 `seed.sh`、整条链路 `chain.sh`、profile 报告 `analyze.sh`、构建 `build.sh`；共用的 `lib.sh` 管容器、起停服务、登录、hey 与 k6 的测量 |
| `scripts/k6/` | k6 脚本：`writes.js`（写入各模式）、`grpc.js`（gRPC 三种调用）、`rate.js`（开环恒定速率）、`soak.js`（浸泡的混合负载） |
| `cmd/otlpsink/` | OTLP/HTTP 接收端：收下 `/v1/traces` 的每个请求、数 span，让追踪导出场景量的是导出路径本身 |
| `out/` | 构建产物与每次运行的结果，不进版本库 |

账号：`root` 由 iam 模块按环境变量 `IAM_ROOT_PASSWORD` 在第一次启动时播种；`reader`（不分租户，只读 records 与 orders）和 `member`（租户 t-03 的成员，持 `ticket_reader` 角色，只读 tickets）由 `module/seed.go` 播种，口令是 `[seed] password`（环境变量 `SEED_PASSWORD`）。脚本把两个口令都设成 `stress-root`。

## 准备

- Docker（脚本用 `mysql:8.4` 和 `redis:7-alpine` 容器，端口 3307、6380，数据放 tmpfs）、[hey](https://github.com/rakyll/hey)、[k6](https://k6.io)、Go。
- 构建服务与接收端：

```bash
bash examples/stress/scripts/build.sh
```

- 起容器（一次即可，脚本之间复用）：

```bash
bash -c 'source examples/stress/scripts/lib.sh && infra_up'
```

## 跑一个场景

每个脚本开头的注释说明它压什么、数据怎么灌。`OUT` 指定结果目录，`ROWS` 指定 records 行数：

```bash
OUT=examples/stress/out/s2 ROWS=100000 bash examples/stress/scripts/s2_data.sh
```

结果目录里：`run.log` 每轮一行 `RESULT`（QPS、分位、非 200、服务 CPU 核与每请求微秒）和 `ALLOCS`（每请求分配）；`results.tsv`、`allocs.tsv` 是同样的数字按列排好；`*.txt` 是 hey 或 k6 的原始输出，旁边的 `.idle`、`.ps` 是压测中每秒的整机空闲和每 10 秒最忙的进程；`cpu_*.pb.gz`、`allocs_*.pb.gz` 是 profile，`scripts/analyze.sh <OUT> <name>…` 把它们转成文本报告。

## 跑整条链路

```bash
bash examples/stress/scripts/chain.sh
```

按场景顺序跑完全部，每个场景写进 `out/<场景>`，场景之间按需要清空并重灌 records；约三小时。

## 结果怎么进 STRESS.md

每个场景一节，结果表从 `results.tsv` 抄，三轮写在一格里；总表取三轮的中位数。重跑只替换数字和结论，不加段落；测法改了先改 STRESS.md 的「方法」一节再重跑受影响的场景。规则在 STRESS.md 开头和仓库 CLAUDE.md 的「examples/stress 与 STRESS.md」一节。
