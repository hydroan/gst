# gst 租约协议

gst · internal/lease

一张表，四条通用语句，外加定时任务专用的四条。契约：**共享同一个主库的所有进程之间，任一时刻至多一个健康的持有者**。到期由数据库时钟判定；持有者比数据库早 5 秒停手；上下文一取消，它开的事务自动回滚。定时任务的一轮跑完才算数，被打断的由先查到它的副本再跑一次。

- **15s**：租约时长（数据库时钟）
- **2s**：续期间隔；失败按同样节奏重试，单次最多等 5 秒
- **10s**：持有者本地截止（从最后一次成功续期发起算）
- **5s +**：竞选间隔，加最多 1 秒随机抖动
- **≤21s**：持有者崩溃后别人接手（15 s 到期 + 下一次竞选）
- **15s +**：各副本查找被打断的定时任务的间隔，加最多 1 秒抖动

## 表 `gst_leases`

每个名字一行，原地更新，永不增长。名字有三种前缀：`cron:`、`leader:`、`lock:`。

| 列 | 类型 | 含义 |
| --- | --- | --- |
| `name` | varchar(191) 唯一 | 协调的名字 |
| `holder` | varchar(32) | 每次抢占新生成的随机令牌，不复用；"是不是我"只认它 |
| `instance` | varchar(191) | 持有它的进程（主机名 + 启动随机串），只用于排查 |
| `term` | bigint unsigned | 任期号，每换一次持有者 +1；写进本表和 cron 每轮、leader 每任的日志。不提供公开取值入口——与 client-go 一致，它也不把 fencing token 交给回调；库内靠事务校验，工作不停靠失败退出 |
| `expires_at_ms` | bigint | 到期时刻，UTC 毫秒，由数据库时钟写入与比较 |
| `slot_ms` | bigint | 只有 cron 用：最近一次领走的调度时刻；一轮跑完时，它跑的途中到了的时刻也记进来（记最近的那个）。只增不减 |
| `unfinished_slot_ms` | bigint | 只有 cron 用：领走的时刻还没跑完时（在跑，或被打断）等于 `slot_ms`，跑完清零。记的是"没跑完"而不是"最后跑完的时刻"：默认值 0 就是没有欠着的。只有它等于 `slot_ms` 才算最近领走的那一轮没跑完，其他值一律按已结清处理 |
| `rerun_slot_ms` | bigint | 只有 cron 用：最近一次被再领一次的时刻；一个时刻最多再领一次 |

## 四条通用语句

全部按唯一键 `name` 定位单行；行锁只在语句自身执行期间持有，没有任何锁跨越业务工作。改到几行就是答案。

### 抢占（claim）

- 谁执行：每个竞选者 · `TryRun` 的调用方
- 何时：每 5 秒竞选一次 / 调用时

```sql
UPDATE gst_leases
   SET holder = :holder, instance = :instance,
       term = term + 1,
       expires_at_ms = :now + 15000
 WHERE name = :name
   AND expires_at_ms <= :now
```

- **1 行**：抢到；从这一刻起每 2 秒续期
- **0 行**：有人持有——正常跳过，不是错误
- **无此名**：改用 `INSERT`，写成"名字已在就什么都不插"（MySQL `INSERT IGNORE` / `ON CONFLICT DO NOTHING`）；改到 0 行 = 别人先插进去了，不是错误、不进错误日志

### 续期（renew）

- 谁执行：持有者
- 何时：每 2 秒，从上一次尝试发起算；一轮 cron 跑超过 2 秒才会有

```sql
UPDATE gst_leases
   SET expires_at_ms = :now + 15000
 WHERE name = :name
   AND holder = :holder
   AND expires_at_ms > :now
```

- **1 行**：仍然持有；本地截止顺延到"本次发起 + 10 秒"
- **0 行**：已失去（过期了，不管有没有被人抢走，或已放手）：取消手上的活
- **报错**：连不上库或超时：每 2 秒重试一次、单次最多等 5 秒，一直试到本地截止；到点仍未成功，按失去处理

### 校验（verify）

- 谁执行：框架，替业务做
- 何时：租约下每个 `database.Transaction` / `TransactionOn` 的第一条语句；开在别的库实例上的事务改到主库上核对，那边没有这张表

```sql
SELECT term
  FROM gst_leases
 WHERE name = :name
   AND holder = :holder
   AND expires_at_ms > :now
```

- **1 行**：放行，任期号进上下文
- **0 行**：已被人抢走、已放手或已过期：返回 `lease.ErrLost`，一条业务语句都不跑

### 放手（release）

- 谁执行：持有者
- 何时：lock、leader 的工作返回 · 停机；cron 只在一轮被打断时用它，跑完的一轮用下面的「登记跑完」

```sql
UPDATE gst_leases
   SET expires_at_ms = 0
 WHERE name = :name
   AND holder = :holder
```

- **1 行**：名字立刻可被抢
- **0 行 / 报错**：只记警告，工作已完成；最迟 15 秒后自然过期

`:now` 是数据库服务端的当前时刻，毫秒整数，不用进程时钟：MySQL `UNIX_TIMESTAMP() * 1000 + MICROSECOND(NOW(3)) DIV 1000`（不经过会话时区）· PostgreSQL `FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT` · SQLite `CAST(unixepoch('subsec') * 1000 AS INTEGER)`。ClickHouse 当主库时这四条能力在入口直接报错。

## 定时任务专用的四条

cron 的名字多记三列，领取和收尾换成下面四条。读和写分两步，是因为领取一个时刻时必须知道自己放弃了什么；写的时候比对读到的任期号——每次领取都会让任期号变，几个副本读到同一行，只有一个写得进去，和单条语句一样排他。

### 领取时刻（claim an instant）

- 谁执行：到点的每个副本 · 启动补跑的副本
- 何时：调度时刻到了 / 启动时补最近一个没人领过、也没被跑完的一轮盖过的时刻（先单独读一次 `slot_ms`，分清是没人领过还是任务从没跑过）

```sql
SELECT term, slot_ms, unfinished_slot_ms,
       rerun_slot_ms, expires_at_ms <= :now
  FROM gst_leases WHERE name = :name
-- 租约到期、且 :slot 晚于 slot_ms 才写：
UPDATE gst_leases
   SET holder = :holder, instance = :instance,
       term = term + 1,
       expires_at_ms = :now + 15000,
       slot_ms = :slot, unfinished_slot_ms = :slot
 WHERE name = :name
   AND term = :term
   AND expires_at_ms <= :now
```

- **1 行**：领到。读到的 `unfinished_slot_ms` 不为 0、且等于读到的 `slot_ms`，说明前一个时刻被打断、还没补上（或补跑也被打断），就此放弃，记 WARN `cronjob gave up a round cut short`
- **0 行**：读和写之间别的副本先领了（任期号变了）——正常跳过
- **不写**：读到有人持有，或 `slot_ms` 不早于 :slot——正常跳过
- **无此名**：改用 `INSERT`，`slot_ms`、`unfinished_slot_ms` 都写成 :slot

### 登记跑完（finish）

- 谁执行：跑这一轮的副本
- 何时：fn 返回 nil、返回自己的错误，或 panic

```sql
UPDATE gst_leases
   SET expires_at_ms = 0,
       unfinished_slot_ms = 0,
       slot_ms = CASE WHEN slot_ms < :passed
                       AND :passed <= :now
                      THEN :passed ELSE slot_ms END
 WHERE name = :name
   AND holder = :holder
-- 0 行（跑的途中丢了租约）时只登记跑完：
UPDATE gst_leases
   SET unfinished_slot_ms = 0
 WHERE name = :name
   AND unfinished_slot_ms = :slot
```

- `:passed`：这一轮结束时，调度表上最近一个已经到了的时刻，按跑这一轮的副本的时钟算。它晚于这一轮的时刻，说明这一轮盖过了它和它之前的几个时刻：这些时刻没有被任何副本领走过——领走过的话，这一轮就领不到；这一轮持有名字期间，别人也领不到。记进 `slot_ms` 之后，它们就此作废，之后启动的副本也不会补跑。数据库时钟还没到的不记，免得时钟偏快的副本把别人还要领的时刻作废
- **1 行**：这个时刻结清，名字立刻可被领
- **第二条 0 行**：更晚的时刻已经领走，不动它。丢了租约的这一轮不记它盖过的时刻：名字可能已经被别人领过
- **报错**：这一轮留在没跑完，租约到期后按被打断再跑一次

### 查找被打断的（find）

- 谁执行：每个副本
- 何时：约每 15 秒一次，加最多 1 秒抖动；一条语句覆盖本副本所有共享任务

```sql
SELECT name, term, slot_ms
  FROM gst_leases
 WHERE name IN (:names)
   AND unfinished_slot_ms = slot_ms
   AND rerun_slot_ms < slot_ms
   AND expires_at_ms <= :now
```

- **每一行**：一个被打断、还没再跑过、租约已放手或已到期的时刻，交给这个任务的循环去再领一次

### 再领一次（claim again）

- 谁执行：查到它的副本上，这个任务的循环
- 何时：查到之后，等下一个时刻的间隙里

```sql
UPDATE gst_leases
   SET holder = :holder, instance = :instance,
       term = term + 1,
       expires_at_ms = :now + 15000,
       rerun_slot_ms = slot_ms
 WHERE name = :name
   AND term = :term
   AND unfinished_slot_ms = slot_ms
   AND rerun_slot_ms < slot_ms
   AND expires_at_ms <= :now
```

- **1 行**：领到，为同一个时刻（`at` 不变）再跑一轮，日志带 `"rerun":true`；跑完照常登记，再被打断就只放手，这个时刻从此放弃
- **0 行**：别的副本先再领了、原来那轮迟到跑完了、或更晚的时刻已开始——跳过

## 三种情形的时间线

横轴是秒，0 秒是副本 A 最后一次成功续期发起的时刻；数据库认定的到期在 15 秒，A 自己的截止在 10 秒。

<!-- 这张图画的是下面三种情形的文字，改文字时同步改图。 -->

```vega-lite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "description": "三种情形下租约的时间线：横轴是秒，0 秒是副本 A 最后一次成功续期发起的时刻",
  "width": 680,
  "height": {"step": 80},
  "padding": {"left": 5, "top": 24, "right": 10, "bottom": 5},
  "config": {"view": {"stroke": null}, "axis": {"labelFontSize": 12}},
  "encoding": {"y": {"field": "lane", "type": "ordinal", "axis": null}},
  "layer": [
    {
      "data": {"values": [
        {"lane": 1, "start": 0, "end": 25, "kind": "hold"},
        {"lane": 2, "start": 0, "end": 2, "kind": "hold"},
        {"lane": 2, "start": 2, "end": 15, "kind": "dead", "label": "没有心跳，租约在数据库里等到期"},
        {"lane": 2, "start": 15, "end": 25, "kind": "take", "label": "15–20s 之间 B 抢到，从头跑 fn"},
        {"lane": 3, "start": 0, "end": 1, "kind": "hold"},
        {"lane": 3, "start": 1, "end": 10, "kind": "dead", "label": "每 2 秒重试续期都失败 · 库里仍是 A"},
        {"lane": 3, "start": 15, "end": 25, "kind": "take", "label": "B 抢到；A 醒来后校验得 0 行"}
      ]},
      "layer": [
        {
          "mark": {"type": "bar", "size": 26, "strokeWidth": 1.5},
          "encoding": {
            "x": {
              "field": "start", "type": "quantitative", "title": null,
              "scale": {"domain": [0, 25], "nice": false},
              "axis": {"values": [0, 5, 10, 15, 20, 25], "labelExpr": "datum.value + 's'", "grid": true}
            },
            "x2": {"field": "end"},
            "color": {
              "field": "kind", "type": "nominal", "legend": null,
              "scale": {"domain": ["hold", "dead", "take"], "range": ["#d7efe9", "#f3d9dd", "#f4e6c8"]}
            },
            "stroke": {
              "condition": [
                {"test": "datum.kind === 'hold'", "value": "#0f7b6c"},
                {"test": "datum.kind === 'take'", "value": "#b7791f"}
              ],
              "value": null
            }
          }
        },
        {
          "transform": [{"filter": "datum.label"}],
          "mark": {"type": "text", "align": "left", "dx": 8, "fontSize": 12},
          "encoding": {
            "x": {"field": "start", "type": "quantitative"},
            "text": {"field": "label"},
            "color": {"condition": {"test": "datum.kind === 'dead'", "value": "#b23a48"}, "value": "#4a5563"}
          }
        }
      ]
    },
    {
      "data": {"values": [
        {"lane": 1, "t": 0}, {"lane": 1, "t": 2}, {"lane": 1, "t": 4}, {"lane": 1, "t": 6},
        {"lane": 1, "t": 8}, {"lane": 1, "t": 10}, {"lane": 1, "t": 12}, {"lane": 1, "t": 14},
        {"lane": 1, "t": 16}, {"lane": 1, "t": 18}, {"lane": 1, "t": 20}, {"lane": 1, "t": 22},
        {"lane": 1, "t": 24}, {"lane": 2, "t": 0}, {"lane": 3, "t": 0}
      ]},
      "mark": {"type": "point", "filled": true, "size": 50, "color": "#0f7b6c", "opacity": 1},
      "encoding": {"x": {"field": "t", "type": "quantitative"}}
    },
    {
      "data": {"values": [
        {"lane": 1, "t": 0, "note": "B 每次竞选都改到 0 行 · 换人只看心跳，不看时长", "tone": "ink"},
        {"lane": 3, "t": 10, "note": "10s：A 取消 ctx → 事务回滚、Commit 被拒", "tone": "lost"}
      ]},
      "mark": {"type": "text", "align": "left", "dx": 6, "dy": 25, "fontSize": 12},
      "encoding": {
        "x": {"field": "t", "type": "quantitative"},
        "text": {"field": "note"},
        "color": {"condition": {"test": "datum.tone === 'lost'", "value": "#b23a48"}, "value": "#4a5563"}
      }
    },
    {
      "data": {"values": [
        {"lane": 1, "title": "健康地干 10 分钟", "sub": "每 2 秒续期，到期不断后移"},
        {"lane": 2, "title": "进程在 2 秒时死亡", "sub": "开着的事务被数据库回滚"},
        {"lane": 3, "title": "1 秒起卡住或断网", "sub": "续期发不出去或没回音"}
      ]},
      "layer": [
        {
          "mark": {"type": "text", "align": "left", "dy": -7, "fontSize": 13, "fontWeight": "bold", "color": "#1b2430"},
          "encoding": {"x": {"value": -180}, "text": {"field": "title"}}
        },
        {
          "mark": {"type": "text", "align": "left", "dy": 10, "fontSize": 11.5, "color": "#4a5563"},
          "encoding": {"x": {"value": -180}, "text": {"field": "sub"}}
        }
      ]
    },
    {
      "data": {"values": [{"t": 10, "label": "A 本地截止 10s"}]},
      "layer": [
        {
          "mark": {"type": "rule", "strokeWidth": 2, "color": "#b23a48"},
          "encoding": {"x": {"field": "t", "type": "quantitative"}, "y": null}
        },
        {
          "mark": {"type": "text", "align": "left", "dx": 5, "dy": -10, "fontSize": 12, "fontWeight": "bold", "color": "#b23a48"},
          "encoding": {"x": {"field": "t", "type": "quantitative"}, "y": {"value": 0}, "text": {"field": "label"}}
        }
      ]
    },
    {
      "data": {"values": [{"t": 15, "label": "数据库到期 15s"}]},
      "layer": [
        {
          "mark": {"type": "rule", "strokeWidth": 2, "strokeDash": [4, 3], "color": "#b7791f"},
          "encoding": {"x": {"field": "t", "type": "quantitative"}, "y": null}
        },
        {
          "mark": {"type": "text", "align": "left", "dx": 5, "dy": -10, "fontSize": 12, "fontWeight": "bold", "color": "#b7791f"},
          "encoding": {"x": {"field": "t", "type": "quantitative"}, "y": {"value": 0}, "text": {"field": "label"}}
        }
      ]
    }
  ]
}
```

**情形 1：健康地干 10 分钟**

- 每 2 秒续期，到期不断后移。
- B 每次竞选都改到 0 行 · 换人只看心跳，不看时长。

**情形 2：进程在 2 秒时死亡**

- 0–2s：A 持有。
- 2s：进程死亡，开着的事务被数据库回滚。
- 2–15s：没有心跳，租约在数据库里等到期。
- 15–20s 之间 B 抢到，从头跑 fn。

**情形 3：1 秒起卡住或断网**

- 0–1s：A 持有。
- 1s 起：卡住或断网，续期发不出去或没回音。
- 1–10s：每 2 秒重试续期都失败 · 库里仍是 A。
- 10s：A 取消 ctx → 事务回滚、Commit 被拒。
- 15s 以后：B 抢到；A 醒来后校验得 0 行。

情形 3 里 10 秒到 15 秒这 5 秒余量就是正确性所在：A 先停手，B 才可能上任。这是 client-go 的 RenewDeadline 与 LeaseDuration 的比例；比 k8s 多出来的一层是，A 的副作用在同一个库里，标准库替它回滚。

## 谁用哪几条

| 消费者 | 一次完整过程 | 备注 |
| --- | --- | --- |
| cron | 「领取时刻」「跑这一轮」「续期 ×n」「登记跑完 / 被打断只放手」 | slot 只增不减，一个调度时刻全集群只领一次；上一轮还持有租约时下一个时刻被拒，上一轮跑完时把它记下，之后谁也不补；丢租约后这一轮 5 秒不返回，进程立即退出 |
| cron 补跑 | 「约 15 秒查找一次」「再领一次」「跑这一轮」「续期 ×n」「登记跑完 / 放手」 | 同一个时刻最多再跑一次；再跑也被打断就放弃；再跑之前下一个时刻已经开始也放弃，记 WARN；跑完了但登记跑完的语句失败的一轮也会这样再跑；所以共享任务必须幂等 |
| leader | 「竞选 = 抢占」「当选后每 2 秒续期」「失去 / 停机 → 取消 ctx」「放手」 | fn 在新副本上从头跑，进度要落库、重跑要能接着来；丢租约后 5 秒还不返回，进程立即退出 |
| lock.TryRun | 「抢占」「fn」「续期 ×n」「放手」 | 只试一次不等待，被占着就返回 `ErrHeld`；先拿锁、事务开在 fn 里——调用时已在任一库实例的事务里（含模型钩子），直接返回 `ErrInTransaction`，一条语句都不发，因为锁会先于外层事务提交放掉；跑到一半失去，即使 fn 成功也返回 `ErrLost`；丢租约后 5 秒不返回同样进程立即退出 |
| 租约下的 `database.Transaction` | 「校验」「业务语句…」「COMMIT」 | 由 `database.Transaction` / `TransactionOn` 自动加，业务代码里没有 `if isLeader` |

## 保证与边界

`cronjob.Register`、`leader.Register` 和 `lock` 建在租约上，下面两份清单说的是它们的共同行为；启动期串行化对所有项目生效。

### 框架保证

- 共享同一个主库的进程之间，同一个名字任一时刻至多一个健康的持有者。
  - 实现：到期只看数据库时钟；任期号随每次换人递增，写进每轮、每任的日志，不对项目公开（对齐 client-go）。
- 租约 15 秒到期。持有者每 2 秒续一次，续失败或变慢时按同样节奏重试，单次最多等 5 秒；连续 10 秒没续上才算丢失，持有者这时自己停手，比数据库允许别人接手早 5 秒，所以数据库抖几秒不丢租约。
  - 实现：续期节奏与 client-go 相同，间隔从上一次尝试发起时算，单次最多等本地截止的一半。
- 停机或让位时续期不停：fn 真正返回之前名字仍是自己的，别的副本接不了手；返回后立刻放手，别的副本不用等租约到期。
  - 实现：续期协程与工作的 ctx 解耦，工作返回、放手时才停。
- fn 收到的 ctx 在租约丢失时结束，定时任务和选主在停机时也结束，锁还跟着调用方的 ctx 结束。用它开的事务跟着回滚，提交前被打断的事务返回的是 ctx 的结束，定时任务据此把这一轮算作被打断。
  - 实现：Go 标准库 `database/sql` 在 ctx 取消后回滚，`Commit` 直接返回错误；标准库这时可能报「事务已结束」，框架统一换成 ctx 自己的结束。
- 在 fn 的 ctx 上用 `database.Transaction` / `TransactionOn` 开的事务，第一条语句核对租约，不是自己的就一条业务语句都不跑。
  - 实现：由事务入口自动加，是事务开头一条不加锁的 SELECT；在其他库实例上开的事务，到主库另一条连接上核对。
- 丢租约后 fn 5 秒还不返回（停机收尾途中发现租约丢了也算），进程立即以失败退出：跳过停机排空，在途请求直接断开，不等其他组件，剩下的收尾最多 10 秒，由编排器重启。
  - 实现：只有 `lease.Run` 一份实现，cron / leader / lock 共用；与 controller-runtime 丢主时不做优雅停机一致。
- 定时任务一轮跑完才算数：fn 返回 nil、返回自己的错误或 panic 算跑完，不再跑；停机或丢租约时 fn 把 ctx 的结束（`ctx.Err()` 或 `context.Cause(ctx)`）原样返回，或者进程崩溃、失败退出没能返回，算被打断，由先发现它的副本为同一个时刻再跑一次：停机打断的约 15 秒内，崩溃的约 30 秒内；跑完了但记录跑完的语句失败的，和崩溃一样处理。
  - 实现：每个副本约 15 秒查一次，一条语句覆盖本副本全部任务；只有最近领走的时刻会被再跑，`unfinished_slot_ms` 是 0 或不等于 `slot_ms` 的都按已结清。
- 被打断的一轮、一任不算失败：fn 返回的只是 ctx 自己的结束（包装过也算）时，定时任务记 WARN `cronjob interrupted`，选主记正常卸任 `leader stepped down`，都带原因；fn 自己的失败要用 `errors.Join` 和结束一起返回才按失败记 ERROR，用 `errors.CombineErrors` 附带或用 `%v` 拼进消息的失败看不见，按被打断处理。
  - 实现：判定单点在 `lifecycle.Interrupted`，err 的每个叶子都得是 ctx 的结束，叶子只认同一个错误对象或它的 Is 方法；span 不设 Error 状态，只加 interrupted 事件。
- 启动期建表（`auto_migrate` 打开时）和 `router.OnRoutesReady` 钩子里的播种，在整个部署里一次只有一个副本在做，所以播种照常写"先查后建"；钩子收到启动期 ctx，停机信号会取消它、钩子跑完它就结束，播种把它一路传给 dao；排队的副本等多久由前面的副本决定，没有框架定的上限，每 30 秒记一条 WARN，停机信号能打断等待，卡住的持锁副本由编排器的启动探针重启放锁。
  - 实现：MySQL / PostgreSQL 服务端 advisory lock，锁名按库区分；SQLite 上建表靠「表已存在就再试一次」。
- 启动顺序是建表、播种，然后是定时任务、选主、锁和常驻组件，最后打开监听；停机时它们先于 provider 停下。
- 不用就零成本：没 import 这些包的项目没有租约表、没有后台协程、没有 SQL、没有配置。

### 框架管不到的

- 已经发出去的外部调用（HTTP、消息）拦不住，调用本身要幂等。
- 不在 `database.Transaction` 里的 Create / Update / Delete（包括它们自己开的事务）不核对租约，只靠 ctx 结束停下；和 Kubernetes 的 leader election 一样，不做逐写核对。
  - 实现：指 `database.Database[M](ctx)` 链上的写方法。
- 事务的租约核对只在开头做一次：核对通过后，进程如果长时间停顿（被冻结、长时间 GC）到租约已被别人接手才恢复，这个事务仍可能提交。事务要短，关键写入要自带条件（唯一约束、按状态更新）。
  - 实现：平时的兜底是本地截止先取消 ctx；只有停顿跨过本地截止与数据库到期之间的余量，才会在丢租约后提交。
- 业务自己用 `context.Background()` 另起 ctx 写库，框架运行期看不见；gg check 的「Detached context」规则在 service、dao、cronjob、leader、lock、component、router 目录里拦这种写法，包括赋给变量再传、包一层 `WithTimeout` 再传。
- 定时任务每个时刻至多跑两轮，不是"恰好一次"：被打断的一轮只再跑一次，再跑也被打断就放弃；再跑之前下一个时刻已经开始也放弃，记 WARN `cronjob gave up a round cut short`。
- 所有副本同时停机期间错过的调度，启动时只补最近一个，而且只补一天以内的。一轮跑超时，期间到点的时刻谁也领不到，直接跳过、不堆叠；这一轮跑完时把跳过的时刻记下，之后启动的副本也不补。
  - 实现：登记跑完的语句顺带把 `slot_ms` 推到这一轮结束时最近一个已到的时刻，只推数据库时钟已经到了的；被打断的一轮不推，它盖过的时刻按「只补最近一个」处理。
- SQLite 只有一条连接，fn 里的事务会挡住续期：单个事务 8 秒内一定安全；事务一个接一个、中间不停顿时，每个要短于 5 秒；超过 8 秒就可能、超过 10 秒必定按租约丢失处理。
  - 实现：续期要等任务自己的事务让出连接，5 秒是单次续期最多等的时长。
- 内存 SQLite 每个进程一套库，协调范围只有本进程，多副本部署不能用。
- 名字加上 `cron:`、`leader:`、`lock:` 前缀后不能超过 191 字节，超长启动期报错；ClickHouse 当主库时这三种能力启动即报错。
  - 实现：领取时兜底的 INSERT 用 INSERT IGNORE / ON CONFLICT DO NOTHING，超长值会被截断插入，所以名字长度必须在进表前校验。
- 建表锁和播种锁绑定数据库会话：副本必须直连主库或走会话级连接池，经 ProxySQL / PgBouncer 这类按语句或事务复用连接的代理，锁会失效；持锁连接一断，锁即释放。
- 租约表的结构有变化时，和项目自己的表一样先 `gg migrate` 再发布：没开 `auto_migrate` 的环境启动只检查表在不在，缺列要到领租约时才报错，定时任务一轮都领不到，选主和锁第一次用一个新名字时也领不到。
