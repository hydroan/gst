# cluster：多副本部署示例

一个用 `gg` 生成的最小项目，演示同一份代码在 Kubernetes 里起多个副本时框架的协调能力：定时任务全集群只跑一次、被打断的一轮换个副本再跑一次、常驻任务只有一个副本在跑、一件事同一时刻只做一次、多个副本同时对空库建表。前三种能力都建在主库的租约表 `gst_leases` 上，不需要 Redis 或 etcd。

## 里面有什么

| 文件 | 演示什么 |
| --- | --- |
| `cronjob/cronjob.go` | `tick`：每 10 秒一轮，整个部署只跑一次；`local-tick`：每个副本各跑；`slow`：一轮跑 20 秒，比 15 秒的租约长，靠续期保住；跑到一半删掉它所在的 Pod，别的副本再跑一次 |
| `leader/leader.go` | 常驻任务 `counter`：每秒在事务里把计数加一；接手的副本从库里的计数接着数 |
| `lock/lock.go`、`dao/rebuild.go`、`service/rebuild/` | `POST /api/rebuilds` 在锁 `rebuild` 下跑，同时来第二个请求立刻 409 |
| `model/event.go`、`model/progress.go` | 副本做过的事和计数器的进度，`GET /api/events`、`GET /api/progress` 可以看 |
| `Dockerfile` | 镜像；PID 1 是 tini，方便从 Pod 里面给进程发信号 |
| `deploy/k8s/` | 一个 MySQL，三副本的 Deployment、就绪 / 存活 / 启动探针、停机宽限、PodDisruptionBudget |
| `scripts/up.sh`、`scripts/down.sh` | 构建镜像、apply 清单、等就绪；整套删掉 |

每个副本写的行都带 `replica`（主机名:端口），框架的日志则带 `instance`（主机名 + 随机串）；主机名就是 Pod 名，两边按它对得上。

## 起来

需要一个当前 `kubectl` 上下文能直接用本机 Docker 镜像的集群（OrbStack、Docker Desktop）：

```bash
examples/cluster/scripts/up.sh
```

kind、minikube 这类看不到本机镜像的集群，先 `kind load docker-image gst-cluster:dev`（或 `minikube image load`）再 `kubectl apply -f examples/cluster/deploy/k8s/`。

三个 Pod 一起对空库建表：框架用数据库自己的锁（MySQL `GET_LOCK`、PostgreSQL advisory lock）让副本轮流准备表和播种，后到的看到表已在就只补差异；等多久由前面的副本决定，启动探针的 `failureThreshold × periodSeconds` 是唯一的上限，播种慢的项目按自己的耗时调它。`deploy/k8s/mysql.yaml` 里的 MySQL 只是给示例用的，没有持久卷。

下面的场景都从本机访问 Service，先开一个端口转发：

```bash
kubectl port-forward svc/cluster 8080:8080 &
```

框架把各路日志都写进 stdout，一行一条，`logger` 字段标明是哪一路（`leader`、`cronjob`、`lock`、`access` 等）；正式部署由采集器收容器输出，示例里用 `kubectl logs` 看：

```bash
for p in $(kubectl get pods -l app=cluster -o name); do
  kubectl logs "$p" -c cluster | grep '"logger":"leader"' | grep -E '"elected leader"|"leader stepped down"' | sed "s#^#$p #"
done
```

进 MySQL 看租约表：

```bash
kubectl exec deploy/mysql -- mysql -uroot -pcluster cluster \
  -e 'select name, instance, term, expires_at_ms, slot_ms, unfinished_slot_ms, rerun_slot_ms from gst_leases'
```

## 场景

### 1. 定时任务：全集群只跑一次，被打断的一轮再跑一次

```bash
curl -s 'localhost:8080/api/events?kind=cron&name=tick&_size=50' | jq '.data'
curl -s 'localhost:8080/api/events?kind=cron&name=local-tick&_size=50' | jq '.data'
```

`tick` 每 10 秒只有一行，`replica` 每次可能不同——哪个副本先抢到就谁跑；`local-tick` 每 10 秒三行，一个副本一行。`slow` 每 30 秒跑 20 秒，期间租约表里 `cron:slow` 的 `expires_at_ms` 每 2 秒往后挪一次，这就是续期。

再看被打断的一轮：等 `slow` 跑起来，删掉正在跑它的 Pod。租约表里 `cron:slow` 还没到期时，`instance` 去掉最后一段随机串就是那个 Pod 的名字：

```bash
until SLOW=$(kubectl exec deploy/mysql -- mysql -uroot -pcluster cluster -N \
  -e "select instance from gst_leases where name = 'cron:slow' and expires_at_ms > unix_timestamp(now(3)) * 1000" 2>/dev/null) && [ -n "$SLOW" ]; do
  sleep 1
done
kubectl delete pod "${SLOW%-*}"
sleep 40
for p in $(kubectl get pods -l app=cluster -o name); do
  kubectl logs "$p" -c cluster | grep '"logger":"cronjob"' | grep '"name":"slow"' | grep -E '"rerun":true|"cronjob gave up a round cut short"' | sed "s#^#$p #"
done
```

被删的 Pod 停机时这一轮的 ctx 结束、没跑完就交还租约；约 15 秒内另一个副本发现它，为同一个调度时刻（`at` 不变）再跑一轮，日志带 `"rerun":true`。跑完后租约表里 `unfinished_slot_ms` 归零，`rerun_slot_ms` 记着这个时刻。一个时刻只再跑一次；再跑之前下一个时刻已经开始的话就放弃，记一条 `cronjob gave up a round cut short`。所以定时任务要写成跑两遍也没问题。

### 2. 选主：删掉 leader 所在的 Pod，别的副本接手

```bash
LEADER=$(for p in $(kubectl get pods -l app=cluster -o jsonpath='{.items[*].metadata.name}'); do
  kubectl logs "$p" -c cluster 2>/dev/null | grep '"logger":"leader"' | grep -E '"elected leader"|"leader stepped down"' | tail -1 | grep -q '"elected leader"' && echo "$p"
done | tail -1)
curl -s localhost:8080/api/progress | jq '.data'
kubectl delete pod "$LEADER"
```

只有一个副本有 `elected leader`，计数每秒加一。`kubectl delete pod` 发 SIGTERM 触发优雅停机，租约被主动放手，几秒内另一个副本 `elected leader`，计数从库里的值接着加，不归零；Deployment 随后补一个新 Pod 回到候选状态。模拟崩溃用 `kubectl delete pod "$LEADER" --grace-period=0 --force`：没有放手这一步，要等 15 秒租约到期再加一次竞选，最多 21 秒接手。

### 3. 锁：同一件事同一时刻只做一次

在两个 Pod 里同时发：

```bash
PODS=($(kubectl get pods -l app=cluster -o jsonpath='{.items[*].metadata.name}'))
kubectl exec "${PODS[0]}" -c cluster -- curl -s -X POST localhost:8080/api/rebuilds -H 'content-type: application/json' -d '{"seconds":5}' &
kubectl exec "${PODS[1]}" -c cluster -- curl -s -X POST localhost:8080/api/rebuilds -H 'content-type: application/json' -d '{"seconds":5}'
wait
```

一个返回 200 并带执行它的 `replica`，另一个立刻 409 "a rebuild is already running"——不排队。5 秒后再发就又能跑。`GET /api/events?kind=lock&_size=50` 每次成功一行。

### 4. 隔离（fencing）：反应慢的旧 leader 写不进去

```bash
kubectl exec "$LEADER" -c cluster -- pkill -STOP cluster    # 冻住 leader 进程
sleep 20
kubectl exec "$LEADER" -c cluster -- pkill -CONT cluster
kubectl logs "$LEADER" -c cluster | grep '"logger":"leader"' | grep 'lease lost'
```

冻住超过 15 秒，租约在数据库里到期，别的副本当选。旧 leader 恢复后本地截止早就过了，它的 ctx 立刻结束；即便它来得及再用 `database.Transaction` 发一个事务（计数就是这么加的），事务的第一条语句也会核对租约、发现不是自己的而整个不跑。它的日志里这一任以 `reason: lease lost` 结束，随后它重新竞选。计数没有被两个副本同时加过。

### 5. 优雅停机：先下线再关

```bash
POD=${PODS[0]}
kubectl delete pod "$POD" &
for i in $(seq 1 20); do kubectl exec "$POD" -c cluster -- curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/-/readyz 2>/dev/null || break; sleep 0.5; done
```

收到 SIGTERM 的瞬间 `/-/readyz` 变成 503，Service 把它摘掉（`kubectl get endpointslices -l kubernetes.io/service-name=cluster` 里少一个地址）；`SERVER_SHUTDOWN_DELAY`（这里 5 秒）过后监听才关闭；正在跑的一轮任务在收到 SIGTERM 时就被取消，返回后交还租约（没跑完的这一轮由别的副本再跑一次，见场景 1），进程才退出。框架停机最长是 5 秒排空 + 最多 30 秒等 HTTP 连接 + 最多 30 秒等在途任务，开了链路追踪和 pprof / statsviz 的部署再各加最多 5 秒关闭它们，合计 80 秒，所以 `terminationGracePeriodSeconds` 设成 90 秒，盖过最坏情况；示例里的任务几秒就返回，实际停机远短于此。

### 6. 滚动更新：工作不断

```bash
kubectl rollout restart deployment/cluster
kubectl rollout status deployment/cluster
curl -s 'localhost:8080/api/events?kind=cron&name=tick&_size=50' | jq '.data | length'
```

滚动期间 `tick` 照常每 10 秒一行，leader 在副本之间接力：PodDisruptionBudget 保证至少两个副本在，租约让工作跟着活着的副本走。端口转发在它连的 Pod 被替换时会断，重新开一个即可。

## 正式环境

这里三个副本都开着 `DATABASE_AUTO_MIGRATE=true`，是为了演示同时建表；正式环境的做法是先 `gg migrate` 建表、副本一律 `DATABASE_AUTO_MIGRATE=false`，让 schema 变更走评审而不是启动副作用。启动锁和建表锁绑定数据库会话，副本要直连主库或走会话级连接池，经按事务复用连接的代理会失效。

```bash
examples/cluster/scripts/down.sh
```
