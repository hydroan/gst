# cluster：多副本部署示例

一个用 `gg` 生成的最小项目，演示同一份代码起多个副本时框架的三种协调能力：定时任务全集群只跑一次、常驻任务只有一个副本在跑、一件事同一时刻只做一次。三种能力都建在主库的租约表 `gst_leases` 上，不需要 Redis 或 etcd。

## 里面有什么

| 文件 | 演示什么 |
| --- | --- |
| `cronjob/cronjob.go` | `tick`：每 10 秒一轮，整个部署只跑一次；`local-tick`：每个副本各跑；`slow`：一轮跑 20 秒，比 15 秒的租约长，靠续期保住 |
| `leader/leader.go` | 常驻任务 `counter`：每秒在事务里把计数加一；接手的副本从库里的计数接着数 |
| `lock/lock.go`、`dao/rebuild.go`、`service/rebuild/` | `POST /api/rebuilds` 在锁 `rebuild` 下跑，同时来第二个请求立刻 409 |
| `model/event.go`、`model/progress.go` | 副本做过的事和计数器的进度，`GET /api/events`、`GET /api/progress` 可以看 |
| `docker-compose.yaml`、`Dockerfile` | 一个 MySQL + 三个副本 |
| `deploy/k8s/` | 三副本的 Deployment、就绪 / 存活探针、停机宽限、PodDisruptionBudget |
| `scripts/run-local.sh`、`scripts/stop-local.sh` | 在本机起 / 停三个副本（端口 8081–8083） |

每个副本写的行都带 `replica`（主机名:端口），框架的日志则带 `instance`（主机名 + 随机串）；两边按主机名对得上。

## 起来

```bash
cd examples/cluster
docker compose up --build
```

三个副本分别在 `localhost:8081`、`8082`、`8083`，日志在 `logs/replica<n>/`（`cronjob.log`、`leader.log`、`lock.log`、`app.log` 等）。三个副本同时启动、都开着 `auto_migrate` 对一个空库建表也没关系：框架用数据库自己的建表锁（MySQL `GET_LOCK`、PostgreSQL advisory lock）让副本轮流准备表，后到的看到表已在就只补差异。本机不走容器的话，准备一个 MySQL（库名 `cluster`，账号见 `config.ini`），然后 `scripts/run-local.sh`。

进 MySQL 看租约表：

```bash
docker compose exec mysql mysql -uroot -pcluster cluster \
  -e 'select name, instance, term, expires_at_ms, slot_ms from gst_leases'
```

## 场景

### 1. 定时任务：全集群只跑一次

```bash
curl -s 'localhost:8081/api/events?kind=cron&name=tick&_size=50' | jq '.data'
curl -s 'localhost:8081/api/events?kind=cron&name=local-tick&_size=50' | jq '.data'
```

`tick` 每 10 秒只有一行，`replica` 每次可能不同——哪个副本先抢到就谁跑；`local-tick` 每 10 秒三行，一个副本一行。`slow` 每 30 秒跑 20 秒，期间租约表里 `cron:slow` 的 `expires_at_ms` 每 5 秒往后挪一次，这就是续期。

### 2. 选主：停掉 leader，别的副本接手

```bash
grep -h '"elected leader"\|"leader stepped down' logs/replica*/leader.log
curl -s localhost:8081/api/progress | jq '.data'
```

只有一个副本有 `elected leader`，计数每秒加一。停掉它：

```bash
docker compose stop replica1   # 换成当前 leader 所在的副本
```

SIGTERM 触发优雅停机，租约被主动放手，几秒内另一个副本 `elected leader`，计数从库里的值接着加，不归零。用 `docker compose kill` 模拟崩溃的话没有放手这一步，要等 15 秒租约到期再加一次竞选，最多 21 秒接手。`docker compose start replica1` 后它回到候选状态。

### 3. 锁：同一件事同一时刻只做一次

两个窗口同时发：

```bash
curl -s -X POST localhost:8081/api/rebuilds -H 'content-type: application/json' -d '{"seconds":5}'
curl -s -X POST localhost:8082/api/rebuilds -H 'content-type: application/json' -d '{"seconds":5}'
```

一个返回 200 并带执行它的 `replica`，另一个立刻 409 "a rebuild is already running"——不排队。5 秒后再发就又能跑。`GET /api/events?kind=lock&_size=50` 每次成功一行。

### 4. 隔离（fencing）：反应慢的旧 leader 写不进去

```bash
docker compose pause replica1     # 换成当前 leader
sleep 20
docker compose unpause replica1
grep -h 'lease lost' logs/replica1/leader.log
```

暂停超过 15 秒，租约在数据库里到期，别的副本当选。旧 leader 恢复后本地截止早就过了，它的 ctx 立刻结束；即便它来得及再发一个事务，事务的第一条语句也会核对租约、发现不是自己的而整个不跑。`leader.log` 里这一任以 `reason: lease lost` 结束，随后它重新竞选。计数没有被两个副本同时加过。

### 5. 优雅停机

```bash
docker compose stop replica2 &
watch -n 0.5 curl -s -o /dev/null -w '%{http_code}\n' localhost:8082/-/readyz
```

收到 SIGTERM 的瞬间 `/-/readyz` 变成 503，负载均衡不再分流；`SERVER_SHUTDOWN_DELAY`（这里 5 秒）过后监听才关闭，正在跑的一轮任务跑完、租约放手，进程才退出。`stop_grace_period` 和 k8s 里的 `terminationGracePeriodSeconds` 都设成 40 秒，盖过这个过程。

## Kubernetes

```bash
# 在仓库根目录构建：go.mod 里的 replace 指向 ../..，构建上下文得是整个仓库
docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
kind load docker-image gst-cluster:dev        # 或推到你的镜像仓库
kubectl apply -f examples/cluster/deploy/k8s/
kubectl get pods -l app=cluster -w
```

`deploy/k8s/mysql.yaml` 里的 MySQL 只是给示例用的，没有持久卷。三个 Pod 一起对空库建表和 compose 一样安全；正式环境的做法仍是先 `gg migrate` 建表、副本一律 `DATABASE_AUTO_MIGRATE=false`，让 schema 变更走评审而不是启动副作用。`kubectl delete pod <leader>` 对应场景 2，`kubectl rollout restart deployment/cluster` 能看到滚动更新期间任务不断：PodDisruptionBudget 保证至少两个副本在，租约让工作在副本之间接力。
