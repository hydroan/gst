# cluster：多副本部署示例

一个用 `gg` 生成的最小项目，演示同一份代码在 Kubernetes 里起多个副本时框架的协调能力：定时任务每个调度时刻全部署只领一次、被打断的一轮换个副本再跑一次、常驻任务同一时刻只有一个副本在跑、一件事同一时刻只做一次、多个副本同时对空库建表。定时任务、选主和锁都建在主库的租约表 `gst_leases` 上，不需要 Redis 或 etcd。

每个场景都给了制造故障和核对结果的命令，可以拿它在真实集群里检验这些能力。

## 里面有什么

| 文件 | 演示什么 |
| --- | --- |
| `cronjob/cronjob.go` | `tick`：每 10 秒一轮，全部署只领一次；`local-tick`：每个副本各跑；`slow`：一轮跑 20 秒，比 15 秒的租约长，靠续期保住 |
| `leader/leader.go` | 常驻任务 `counter`：每秒在事务里给计数器追加下一个数字，并记下是哪一任写的；接手的副本从库里最后一个数字接着数 |
| `lock/lock.go`、`dao/rebuild.go`、`service/rebuild/` | `POST /api/rebuilds` 在锁 `rebuild` 下跑，同时来第二个请求立刻 409 |
| `model/run.go`、`dao/run.go` | 每一轮定时任务、每一次锁下的运行：开始时记一行，跑完时补上结束时间，都写在工作自己的事务里；被打断的没有结束时间。`GET /api/runs` |
| `model/counter_step.go` | 计数器的每个数字，和写它的那一任、那个副本。`GET /api/counter_steps` |
| `Dockerfile` | 镜像：以非 root 用户运行，配置全部来自环境变量；PID 1 是 tini，方便从 Pod 里给进程发信号 |
| `deploy/k8s/` | 纯 YAML 清单：命名空间（强制 restricted 安全标准）、MySQL、三副本 Deployment（探针、资源、只读根文件系统、停机宽限、滚动更新策略）、Service、PodDisruptionBudget |
| `scripts/up.sh`、`scripts/down.sh` | 构建镜像、apply 清单、等就绪；整套删掉 |
| `scripts/collect-logs.sh` | 把每个副本、每一代容器的日志存到本地，供事后核对 |

每个副本写的行都带 `replica`（主机名:端口），框架的日志则带 `instance`（主机名加每次启动的随机串）；主机名就是 Pod 名，两边按它对得上。

## 起来

需要一个当前 `kubectl` 上下文能直接用本机 Docker 镜像的集群（OrbStack、Docker Desktop）：

```bash
examples/cluster/scripts/up.sh
```

改了代码再跑一遍 `up.sh`，它会重新构建镜像，并滚动重启副本换上新镜像。kind、minikube 这类看不到本机镜像的集群，照 `up.sh` 里的命令构建镜像后，先 `kind load docker-image gst-cluster:dev`（或 `minikube image load gst-cluster:dev`），再 `kubectl apply -f examples/cluster/deploy/k8s/`。

所有对象都在命名空间 `gst-cluster` 里。三个副本同时对空库建表：框架用数据库自己的锁（MySQL `GET_LOCK`、PostgreSQL advisory lock）让副本轮流准备表和播种，后到的副本看到表已经在，只补差异。要等多久由排在前面的副本决定，唯一的上限是启动探针的 `failureThreshold × periodSeconds`（这里是 150 秒），播种慢的项目按自己的耗时调它。

清单里的跨节点打散（`topologySpreadConstraints`）和 MySQL 持久卷（`volumeClaimTemplates`）已经写好，但注释掉了：单节点集群用不上，多节点集群按需打开。MySQL 的数据放在 Pod 自己的临时卷里，容器重启后还在，Pod 删掉就没了。

框架把各路日志都写进 stdout，一行一条 JSON，`logger` 字段标明是哪一路（`leader`、`cronjob`、`lock`、`gorm`、`access` 等）。Pod 删掉、容器重启，它的日志也随之消失，可核对一个场景要看参与过的每个副本的日志。所以做场景之前，先在后台启动日志采集，把每个副本、每一代容器的日志各存成 `cluster-logs/` 下的一个文件；正式环境里这件事由日志采集系统完成。

下面的场景会用到日志采集和几个小工具，先在当前终端里启动、定义好：

```bash
NS=gst-cluster
kubectl -n $NS port-forward svc/cluster 8080:8080 >/dev/null &
# 采集日志，全部核对完再 kill $COLLECTOR。
examples/cluster/scripts/collect-logs.sh cluster-logs >/dev/null 2>&1 &
COLLECTOR=$!
# logs [jq 条件]：采集下来的日志，去掉重复采集的行，按时间排好。例如 logs '.logger == "leader"'。
logs() { cat cluster-logs/*.log | jq -R -c "fromjson? | select(${1:-true})" | jq -s -c 'unique | sort_by(.ts) | .[]'; }
# 用应用自己的账号执行 SQL：sql 输出表格，sqlv 只输出值。
sql()  { kubectl -n $NS exec -i mysql-0 -c mysql -- sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -ucluster -t cluster'; }
sqlv() { kubectl -n $NS exec -i mysql-0 -c mysql -- sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -ucluster -N -B cluster'; }
# pods：没在停机的副本 Pod，一行一个。
pods() { kubectl -n $NS get pods -l app.kubernetes.io/name=cluster -o go-template='{{range .items}}{{if not .metadata.deletionTimestamp}}{{.metadata.name}}{{"\n"}}{{end}}{{end}}'; }
# holder NAME [条件]：正持有租约 NAME 的 Pod。instance 去掉最后一段随机串就是 Pod 名。
holder() {
  local i
  i=$(echo "SELECT instance FROM gst_leases WHERE name = '$1' AND expires_at_ms > UNIX_TIMESTAMP() * 1000 + MICROSECOND(NOW(3)) DIV 1000 $2" | sqlv)
  [ -n "$i" ] && echo "${i%-*}"
}
```

## 核对

每个场景做完都可以跑一遍。下面三条查询里，两条应该返回空结果，`missing` 应该是 0：

```bash
sql <<'SQL'
-- 计数器：每一任写的数字连成一段，各段互不交错；有交错，说明两任同时写过。
SELECT a.tenure, a.lo, a.hi, b.tenure, b.lo, b.hi
  FROM (SELECT tenure, MIN(seq) lo, MAX(seq) hi FROM counter_steps GROUP BY tenure) a
  JOIN (SELECT tenure, MIN(seq) lo, MAX(seq) hi FROM counter_steps GROUP BY tenure) b
    ON a.tenure < b.tenure AND a.lo <= b.hi AND b.lo <= a.hi;
-- 计数器：数字连续，没有缺号。
SELECT MAX(seq) - MIN(seq) + 1 - COUNT(*) AS missing FROM counter_steps;
-- 同一个共享定时任务、同一把锁，跑完的运行在时间上互不重叠。
SELECT a.name, a.replica, a.created_at, a.ended_at, b.replica, b.created_at, b.ended_at
  FROM runs a JOIN runs b
    ON a.kind = b.kind AND a.name = b.name AND a.id < b.id AND a.name <> 'local-tick'
   AND a.ended_at IS NOT NULL AND b.ended_at IS NOT NULL
   AND a.created_at < b.ended_at AND b.created_at < a.ended_at;
SQL
```

`seq` 上有唯一索引，两任同时写也可能表现为 leader 日志里的 `Duplicate entry` 错误，这同样说明出了问题。

每个调度时刻只领一次，看采集下来的日志。每一轮结束时 `cronjob` 那一路记一条带 `trace_id` 和调度时刻 `at` 的结果（`finished cronjob`、`cronjob interrupted` 等）；再跑的一轮带 `"rerun":true`，启动补跑的带 `"catch_up":true`。同一个共享任务、同一个 `at`、同样的 `rerun`，只该有一条，下面应该什么都不输出：

```bash
logs '.logger == "cronjob" and .trace_id != null and .name != "local-tick"' | jq -r '[.name, .at, (.rerun // false)] | @tsv' | sort | uniq -d
```

最后数一数记了哪些 WARN 和 ERROR：

```bash
logs '.level == "WARN" or .level == "ERROR"' | jq -r '[.level, .logger, .msg] | @tsv' | sort | uniq -c | sort -rn
```

场景本身会带来的，各场景里都写了：停机打断的 `cronjob interrupted`、超时跳过的 `cronjob skipped instants`、数据库出故障时的 `lease renewal failed`、`sql failed`、`slow sql detected` 等。锁被占时接口返回 409，controller 那一路照例记一条 ERROR `service operation failed`（`a rebuild is already running`）。除此之外的 WARN、ERROR 都值得查清楚。

## 场景

### 1. 定时任务：每个时刻只领一次，被打断的一轮再跑一次

```bash
curl -s 'localhost:8080/api/runs?kind=cron&name=tick&_sort_by=created_at%20desc&_size=10' | jq '.data.items'
curl -s 'localhost:8080/api/runs?kind=cron&name=local-tick&_sort_by=created_at%20desc&_size=10' | jq '.data.items'
```

`tick` 每 10 秒一行，`replica` 每次可能不同，哪个副本先抢到就由谁跑；`local-tick` 每 10 秒三行，每个副本一行。`slow` 每 30 秒跑 20 秒，期间租约表里 `cron:slow` 的 `expires_at_ms` 每 2 秒往后挪一次，这就是续期。

等 `slow` 跑起来，删掉正在跑它的 Pod：

```bash
until P=$(holder cron:slow "AND unfinished_slot_ms = slot_ms"); do sleep 1; done
kubectl -n $NS delete pod "$P"
sleep 40
logs '.logger == "cronjob" and .name == "slow" and .trace_id != null' | jq -c '{at, msg, reason, rerun, instance}' | tail -3
echo "SELECT instance, slot_ms, unfinished_slot_ms, rerun_slot_ms FROM gst_leases WHERE name = 'cron:slow'" | sql
```

被删的 Pod 停机时，这一轮的 ctx 立刻结束，没跑完就交还租约，日志记 `cronjob interrupted`（`reason: shutting down`）。约 15 秒内，另一个副本发现它，为同一个调度时刻（`at` 不变）再跑一轮，日志带 `"rerun":true`。租约表里的时刻都是毫秒：`rerun_slot_ms` 记着再跑过的那个时刻；`slot_ms` 是最近领走的时刻，那一轮正在跑、或者没跑完时 `unfinished_slot_ms` 等于它，跑完归零。

一个时刻只再跑一次。再跑还没开始、下一个时刻就先开始了的话，就放弃再跑，记一条 `cronjob gave up a round cut short`。再跑的这一轮盖过了下一个时刻的话，那个时刻被跳过，记 `cronjob skipped instants`。所以定时任务要写成跑两遍也没问题。

### 2. 进程崩溃：跑到一半被 SIGKILL

```bash
until P=$(holder cron:slow "AND unfinished_slot_ms = slot_ms"); do sleep 1; done
kubectl -n $NS exec "$P" -c cluster -- pkill -KILL -x cluster
sleep 45
kubectl -n $NS get pods -l app.kubernetes.io/name=cluster
logs '.logger == "cronjob" and .name == "slow" and .trace_id != null' | jq -c '{at, msg, reason, rerun, instance}' | tail -3
```

进程没机会交还租约，也不留 `cronjob interrupted`；容器随即重启，`RESTARTS` 加一。租约要等到期（最后一次续期后 15 秒），再由某个副本查到，为同一个时刻再跑一轮，前后不超过约 30 秒。再跑它的也可能正是那个 Pod 重启后的新进程。租约到期之前轮到的时刻谁也领不到，会被跳过，再跑的那一轮结束时记 `cronjob skipped instants`，并把这个时刻记进租约表，之后启动的副本也不会补跑它。

`kubectl delete pod --force --grace-period=0` 模拟不了崩溃：kubelet 照样先给进程发 SIGTERM，进程会正常停机、交还租约。要走崩溃这条路，只能像上面这样直接杀进程。

### 3. 选主：优雅停机与崩溃

```bash
P=$(holder leader:counter)
kubectl -n $NS delete pod "$P"
until N=$(holder leader:counter) && [ "$N" != "$P" ]; do sleep 0.5; done; echo "new leader: $N"
```

只有一个副本记着 `elected leader`，计数器每秒加一。删掉 leader 所在的 Pod，它停机时主动交还租约，最多约 6 秒另一个副本当选，计数器从库里的最后一个数字接着加；Deployment 随后补一个新 Pod，回到候选状态。

把 `delete pod` 换成 `kubectl -n $NS exec "$P" -c cluster -- pkill -KILL -x cluster`，就是崩溃：没有交还这一步，要等租约到期再加一次竞选，最多约 21 秒接手。两种情况做完都跑一遍「核对」，计数器的各任首尾相接、互不交错。

### 4. 隔离：冻住的旧 leader 写不进去

```bash
P=$(holder leader:counter)
kubectl -n $NS exec "$P" -c cluster -- pkill -STOP -x cluster
sleep 20
holder leader:counter      # 已经换成别的 Pod
kubectl -n $NS exec "$P" -c cluster -- pkill -CONT -x cluster
kubectl -n $NS logs "$P" -c cluster | grep '"logger":"leader"' | tail -3
```

冻住超过 15 秒，租约在数据库里到期，别的副本当选。旧 leader 恢复时，本地截止时间早就过了，它的 ctx 立刻结束，这一任记 `leader stepped down`，`reason: lease lost`。就算它抢在发现之前又发出一个事务，事务的第一条语句也会核对租约，发现租约不是自己的，整个事务一条都不执行；这时这一任记成 `leader stepped down with error`，`reason: work returned`，错误里是 `lease lost`。不管是哪种情况，跑「核对」都看不到两任交错写。

### 5. 锁：同一件事同一时刻只做一次

```bash
P1=$(pods | sed -n 1p)
P2=$(pods | sed -n 2p)
rebuild() { kubectl -n $NS exec "$1" -c cluster -- curl -s -w ' HTTP=%{http_code}\n' -X POST localhost:8080/api/rebuilds -H 'content-type: application/json' -d "{\"seconds\":$2}"; }
rebuild "$P1" 5 &
rebuild "$P2" 5
wait $!
```

一个返回 200，并带执行它的 `replica`；另一个立刻返回 409 "a rebuild is already running"，不排队。5 秒后再发，就又能跑了。

持锁的进程崩溃：在 `$P1` 上跑 10 秒的重建，跑到一半 `pkill -KILL` 它。紧接着在 `$P2` 上再发，仍然是 409，因为租约还没到期；约 15 秒后再发就是 200。崩溃的那次运行在 `/api/runs` 里没有 `ended_at`。

持锁的进程被冻住：跑重建时 `pkill -STOP` 冻住它，超过 15 秒后，别的 Pod 就能拿到锁跑完。被冻住的进程恢复后，它那次请求得到 500 "the rebuild did not finish"，那次运行也没有 `ended_at`。请求等过了服务端的写超时（默认 15 秒）的，客户端收不到这个响应，只在服务端的访问日志里看得到。

### 6. 数据库卡住、数据库重启

用全局读锁把所有写入卡住 25 秒：

```bash
kubectl -n $NS exec mysql-0 -c mysql -- sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -e "FLUSH TABLES WITH READ LOCK; SELECT SLEEP(25); UNLOCK TABLES;"'
```

续期语句一条条超时，记 `lease renewal failed`。连续 10 秒续不上，各持有者自己停手：leader 记 `leader stepped down`（`reason: lease lost`），正在跑的定时任务记 `cronjob interrupted`（`reason: lease lost`）。读锁一放开，各项工作按规则恢复：被打断的一轮再跑一次（或者因为下一个时刻已经开始而放弃），新的 leader 当选。整个过程没有一个副本重启。

让 mysqld 停下，容器随即重启：

```bash
kubectl -n $NS exec mysql-0 -c mysql -- sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysqladmin -uroot shutdown'
```

mysqld 一退出，容器跟着重启，这条命令就以 `command terminated with exit code 137` 结束，这是正常的。日志里会有 `Server shutdown in progress`、`invalid connection`、`connection refused` 这些真实的数据库错误。leader 的事务如果在核对租约时断了连接，这一任记 `leader stepped down with error`，然后重新竞选；数据库回来之后，等租约到期就有新 leader。应用副本同样不重启。两个场景做完都跑一遍「核对」。

### 7. 整个部署停机：只补最近一个时刻

```bash
kubectl -n $NS scale deployment/cluster --replicas=0
kubectl -n $NS wait --for=delete pod -l app.kubernetes.io/name=cluster --timeout=150s
sleep 45
kubectl -n $NS scale deployment/cluster --replicas=3
```

停机期间错过的调度时刻没人领。三个副本一起启动，每个共享任务只由一个副本补跑最近一个时刻，日志带 `"catch_up":true`；更早的时刻不补。补跑的 `slow` 同样要跑 20 秒，盖过了下一个时刻的话，那个时刻被跳过，记 `cronjob skipped instants`；跳过的时刻记在租约表里，之后再有副本启动也不会补它。

### 8. 优雅停机：先摘流量再关

```bash
kubectl -n $NS rollout status deployment/cluster
P=$(pods | sed -n 1p)
IP=$(kubectl -n $NS get pod "$P" -o jsonpath='{.status.podIP}')
kubectl -n $NS delete pod "$P" --wait=false
for i in $(seq 1 12); do
  kubectl -n $NS exec "$P" -c cluster -- curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/-/readyz 2>/dev/null || break
  kubectl -n $NS get endpointslices -l kubernetes.io/service-name=cluster -o json \
    | jq -c --arg ip "$IP" '.items[].endpoints[]? | select(.addresses | index($ip)) | .conditions'
  sleep 0.5
done
```

删 Pod 的那一刻，EndpointSlice 里这个地址就被标成 `ready: false`、`terminating: true`：地址还在，但 Service 不再往它上面转发新请求；就绪探针失败后（这里最多 2 秒），`serving` 也变成 `false`。进程收到 SIGTERM，`/-/readyz` 立刻变成 503；`SERVER_SHUTDOWN_DELAY`（这里是 5 秒）过后监听才关闭，给沿途的负载均衡留出发现它下线的时间。正在跑的一轮任务在收到 SIGTERM 时就被取消，返回后交还租约，进程才退出。

框架停机最长是：5 秒排空，加最多 30 秒等 HTTP 连接，加最多 30 秒等在途任务；开了链路追踪和调试端点（pprof、statsviz）的部署，关闭它们再各加最多 5 秒，合计 80 秒。所以 `terminationGracePeriodSeconds` 设成 90 秒，盖过最坏情况。示例里的任务几秒就返回，实际停机要短得多。

### 9. 滚动更新：工作不断

```bash
kubectl -n $NS rollout restart deployment/cluster
kubectl -n $NS rollout status deployment/cluster
```

Deployment 的滚动策略是 `maxSurge: 1`、`maxUnavailable: 0`：新 Pod 就绪之后才停旧 Pod，全程保持三个副本在服务。`tick` 每 10 秒照常一轮；leader 随着旧 Pod 停机在副本之间接力；被停机打断的 `slow` 由别的副本再跑一次。其余副本一直在领时刻，新起的副本没有要补跑的，日志里不该出现 `"catch_up":true`。PodDisruptionBudget 管不到滚动更新，它限制的是节点排空（drain）这类主动驱逐，保证那种时候至少留两个副本。端口转发连着的 Pod 被替换时会断开，重新开一个即可。

## 正式环境

- 这里三个副本都开着 `DATABASE_AUTO_MIGRATE=true`，是为了演示同时建表。正式环境的做法是：先用 `gg migrate` 建表，副本一律设成 `DATABASE_AUTO_MIGRATE=false`，让表结构变更走评审，而不是作为启动的副作用。
- 建表和播种用的启动锁绑定数据库会话，副本要直连主库，或者走会话级连接池；经过按事务复用连接的代理，这把锁会失效。
- 多节点集群打开清单里注释掉的 `topologySpreadConstraints`，数据库换成真正的持久存储；口令这类 Secret 的生成和轮换由部署流程负责。

```bash
examples/cluster/scripts/down.sh
```
