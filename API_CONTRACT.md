# API 对接契约

本文档是前后端共同遵守的对接契约：后端按此实现默认资源接口，前端按此调用，
出现分歧以本文为准。下面以 `record` 资源为例，对应资源路径为 `/api/records`。

本文只描述默认资源接口。自定义接口可能有自己的路径、请求结构和响应结构，应以
对应接口文档或 Swagger 为准。请求体统一使用 JSON：

```http
Content-Type: application/json
```

字段名以接口返回、Swagger 或接口文档中的 JSON 字段名为准，例如 `id`、`name`、
`status`，不要使用未出现在接口契约中的内部字段名。

请求数据必须放在本文指定的位置。放错位置不属于接口契约的一部分，前端不要依赖后端
从 body、query 或 URL 之间兜底读取数据。

时间字段一律用 RFC3339（带时区偏移）。日期字段按 UTC 日历日解释：请求里的时刻先换算成 UTC
再取日期，`2026-01-02T00:00:00+08:00` 存为 2026-01-01，响应里回 `2026-01-01T00:00:00Z`，
HTTP 与 gRPC 一致。

## 接口总览

| 操作 | 方法和路径 | 请求数据硬性要求 |
| --- | --- | --- |
| 创建一个 record | `POST /api/records` | 必须把一个 record 对象放在 body |
| 删除一个 record | `DELETE /api/records/:id` | 必须把 `id` 放在 URL；body 不承载语义 |
| 全量更新一个 record | `PUT /api/records/:id` | 必须把 `id` 放在 URL，完整更新内容放在 body |
| 部分更新一个 record | `PATCH /api/records/:id` | 必须把 `id` 放在 URL，只把要修改的字段放在 body |
| 查询 record 列表 | `GET /api/records?name=r1&status=enabled` | 查询条件必须放在 URL query；不允许用 body 传查询条件 |
| 获取一个 record | `GET /api/records/:id` | 必须把 `id` 放在 URL；body 不承载语义 |
| 创建多个 record | `POST /api/records/batch` | body 必须使用 `{ "items": [...] }` |
| 删除多个 record | `DELETE /api/records/batch` | body 必须使用 `{ "ids": [...] }` |
| 全量更新多个 record | `PUT /api/records/batch` | body 必须使用 `{ "items": [...] }`，每个 item 必须带 `id`，同一个 `id` 只能出现一次 |
| 部分更新多个 record | `PATCH /api/records/batch` | body 必须使用 `{ "items": [...] }`，每个 item 必须带 `id` 和要修改的字段，同一个 `id` 只能出现一次 |

## 列表通用查询参数

除业务字段过滤外，列表接口的通用查询参数由后端按资源逐个启用：分页由 model 嵌入
`model.Pagination` 启用，游标分页由 `model.Cursor` 启用，排序、展开关联、
字段操作符过滤等参数由 `model.Query` 启用（`model.Query` 同时包含前两者）。资源未启用
对应能力时，传这些参数会返回 400。某个资源支持哪些参数以 Swagger 为准。

| 能力 | 参数 | 启用方式 | 示例 |
| --- | --- | --- | --- |
| 分页 | `_page`（从 1 开始）、`_size`；`_size` 缺省 20，上限 100（超限收敛到 100，不报错） | `model.Pagination` | `?_page=1&_size=20` |
| 排序 | `_sort_by`，逗号分隔多字段，方向 `asc`/`desc`（默认 `asc`） | `model.Query` | `?_sort_by=created_at desc,name` |
| 展开关联 | `_expand`，逗号分隔，`all` 表示全部可展开字段（可展开字段列表见 Swagger 中 `_expand` 参数说明，接受 snake_case 写法、大小写不敏感）；`_depth` 控制自引用字段的递归展开层数，范围 [1,10]，越界回退 1 | `model.Query` | `?_expand=all&_depth=5` |
| 字段操作符过滤 | `字段[op]=值`，与其他条件按 AND 组合；op 支持 `eq`、`ne`、`gt`、`gte`、`lt`、`lte`、`in`、`notin`（逗号分隔多值）、`like`、`notlike`（子串匹配）、`startswith`、`endswith`（前/后缀匹配，前缀可走索引）、`isnull`（值为 `true`/`false`，判断任意可空字段是否为 NULL）；`like` 类的值是字面量而非模式，`%`、`_` 会被转义；值按字段类型校验，非法返回 400：数字字段要求数字值；时间字段只支持比较类 op，值只收带时区偏移的 RFC 3339（如 `2025-01-01T00:00:00+08:00`，小数秒最多到纳秒），无时区写法、纯日期、Unix 时间戳一律 400——同一个无时区值在不同服务器时区代表不同时刻，宁可拒绝也不猜；整天范围由调用方自己展开成当天 00:00 的 `gte` 和当天 23:59:59 的 `lte`，query 里的 `+` 必须编码成 `%2B`（或改用 `Z`、负偏移）；时间范围用同字段 `gte`+`lte` 组合表达，`created_at`/`updated_at` 同样支持裸名精确过滤（值为时间，同上校验）与操作符过滤；字段名、操作符非法返回 400，空值视为不过滤 | `model.Query` | `?age[gte]=18&remark[like]=hello&created_at[gte]=2025-01-01T00:00:00%2B08:00&created_at[lte]=2025-01-15T23:59:59%2B08:00` |
| 游标分页 | `_cursor_value`、`_cursor_field`、`_cursor_next`；`_size` 控制批大小（缺省 20，上限 100）；`_cursor_field` 只接受主键、或带独占唯一索引的列（不含与别的列共用一个唯一索引的），传其他列返回 400——游标只有一个边界值，按两行可能相同的列翻页时，取值相同的行会被拆到两页之间，装不下的那些再也读不到；不传 `_cursor_field` 时按主键翻页；时间列的 `_cursor_value` 同样只收 RFC 3339；游标与 offset 翻页互斥：仅嵌 `model.Cursor` 的资源传 `_page` 返回 400，游标生效时 `_page` 被忽略；使用游标时响应的 `total` 为 0 | `model.Cursor` | `?_cursor_value=xxx&_cursor_next=true&_size=50` |

命名约定：框架控制参数一律以 `_` 开头，`_` 前缀是框架保留命名空间，业务字段的
query 名不要以 `_` 开头。反过来，所有裸名参数都属于业务字段过滤（如 `?name=xxx`），
`page`、`size`、`limit` 这类裸名也可以放心用作业务过滤列，不会与框架参数冲突。
方括号是操作符过滤的保留语法：`字段[op]=值` 不是业务字段精确过滤，裸名精确过滤
（`?age=10`）和同字段的操作符过滤（`?age[gt]=20`）可以同时出现，各自独立生效并
按 AND 组合。

## 请求体格式

创建一个 record：

```json
{
  "name": "g1",
  "status": "enabled"
}
```

部分更新一个 record：

```json
{
  "status": "disabled"
}
```

创建多个、全量更新多个、部分更新多个 record：

```json
{
  "items": [
    {
      "id": "record-id-1",
      "name": "g1",
      "status": "enabled"
    },
    {
      "id": "record-id-2",
      "name": "g2",
      "status": "disabled"
    }
  ]
}
```

批量创建时，`items` 中通常不需要传 `id`，`id` 由后端生成并在响应中返回；批量全量
更新和批量部分更新时，每个 item 都必须带 `id`，缺 `id` 返回 400，同一个 `id` 在一个
批次里只能出现一次，重复返回 400。删除多个时 `ids` 里同样不能有空值或只含空白的值，
否则返回 400。

删除多个 record：

```json
{
  "ids": ["record-id-1", "record-id-2"]
}
```

## 响应结构

所有接口统一返回如下 envelope：

```json
{
  "msg": "success",
  "data": {},
  "trace_id": "..."
}
```

- 成功时 HTTP 状态码为 `200`，业务数据在 `data` 中。
- 失败时 HTTP 状态码为 `4xx`/`5xx`，`msg` 是可展示的错误信息。
  前端只以 HTTP 状态码判定失败：`4xx` 是请求方的问题，`5xx` 是服务端的问题；信封里没有业务状态码。
  服务端自身的故障（没有明确拒绝理由的错误）统一返回 `500`，`msg` 是固定的通用文案；
  响应的 `data` 编码不成 JSON（例如 NaN）也算服务端故障，返回 `500`，不会出现 `200` 空响应。
- `trace_id` 用于排障，反馈接口问题时请带上。
- 每个接口会答的失败状态（`400`、`404`、`409`）与失败信封的形状写在 Swagger 里，其余失败走 `default` 响应。

列表接口的 `data` 固定为如下结构：

```json
{
  "total": 2,
  "items": [{ "id": "record-id-1" }, { "id": "record-id-2" }]
}
```

## SSE 事件流接口

部分接口以 Server-Sent Events 提供实时通知，前端用 `EventSource` 消费（自动携带
登录态 cookie）；此类接口在 Swagger 中标注为 `text/event-stream` 响应。

- 连接成功后是 `text/event-stream` 长连接，不使用统一 envelope；连接建立失败
  （未登录、无权限、参数非法）时返回普通 envelope 错误。
- 事件采用「薄提示」形态：只说明某资源有变化，不携带业务数据；前端收到后对
  相关视图做防抖回查，数据一律以查询接口为准。事件名与回查范围以具体接口文档为准。
- 服务端周期发送 `: ping` 注释帧保活，`EventSource` 自动忽略。
- 断线由 `EventSource` 自动重连；`onopen`（含重连成功）时应对相关视图全量回查
  一遍，作为漏事件的一致性兜底。

## 并发编辑保护（乐观锁）

响应中带 `version` 字段的资源启用并发编辑保护，防止两人同时编辑同一行数据时，
后保存的一方悄悄覆盖先保存的修改。

- 查询响应中的 `version` 是该行数据的版本号，前端随其它业务字段一起保存，编辑提交时
  原样带回；保存成功后响应中的 `version` 已自增，编辑页若不刷新可用它继续下一次保存。
- 全量更新（`PUT`）和部分更新（`PATCH`）都必须携带读取时的 `version`；`PATCH` 只提交
  部分字段时 `version` 也必须在 body 中。
- 新建不携带 `version`，服务端建行后版本从 1 开始。
- 保存与他人的修改冲突时返回 HTTP `409`，`msg` 是可直接展示的提示：前端展示 `msg`
  并引导用户刷新页面后重新编辑即可，不要按 `msg` 的具体文案做逻辑分支。
- 更新请求漏带 `version` 返回 HTTP `400`，属前端对接缺陷，处理方式同样是展示 `msg`。
  例外是没有独立新建入口、以同一保存接口读-判-写的单行资源：那里「未携带版本」本身
  就是「行还不存在」的断言，行已存在时按保存冲突返回 `409`。
- 启停、开关类单键动作接口不携带 `version`，服务端按无条件翻转处理；翻转会让版本
  前进，其他人已打开的编辑页保存时会收到 `409`，属预期行为。
- 数据不满足数据库约束时同样是 4xx，`msg` 可直接展示：外键指向不存在或还被引用的记录返回
  `409`；check 约束不满足、值超过列长度、必填列为空返回 `400`。
- 字段校验失败返回 `400`，`msg` 点名字段：`name is a required field`，多个字段用分号连，
  嵌套字段写路径 `address.city`，批量请求的项写 `items[1].name`；字段名是 JSON 名，没有 json 名的
  嵌入结构体不占路径一层，它的字段按 JSON 提升后的名字写。校验器没有现成句子的规则（含项目自己注册的）
  写成 `host failed the hostname check`，规则带参数时带上：`tag failed the startswith=ab check`。

## 关键规则

- 单个资源的 `id` 必须放在 URL 中，例如 `/api/records/record-id-1`。
- body 中的 `id` 不能替代 URL 中的 `:id`。
- `GET` 请求只认 URL query 中的查询条件，不使用 body。
- `PUT` 表示全量更新，body 应包含完整更新内容。
- `PATCH` 表示部分更新，body 只放需要修改的字段；写进 body 的字段整体替换，对象字段也整体换成 body 里的值（没写的子字段按零值算），给 null 或零值就是清空；不支持只改对象里的某个子字段；`id`、`created_at` 这类框架管理的字段不能改，写了也被忽略；模型没有的键同样忽略（和创建、全量更新一致）；`{}` 这样一个字段都没写的 body 照样把整行按原样写回，`updated_at` 会刷新。
- 批量创建、批量全量更新、批量部分更新统一使用 `items`。
- 批量删除统一使用 `ids`。
- 不要把参数放到“看起来也能传”的其他位置；本文指定的位置就是前端对接时必须遵守的位置。
- 前端解析响应只依赖统一 envelope 和 `data` 结构，不要依赖 `msg` 的具体文案做逻辑判断。
