# API 对接契约

本文档是前后端共同遵守的对接契约：后端按此实现默认资源接口，前端按此调用，
出现分歧以本文为准。下面以 `group` 资源为例，对应资源路径为 `/api/groups`。

本文只描述默认资源接口。自定义接口可能有自己的路径、请求结构和响应结构，应以
对应接口文档或 Swagger 为准。请求体统一使用 JSON：

```http
Content-Type: application/json
```

字段名以接口返回、Swagger 或接口文档中的 JSON 字段名为准，例如 `id`、`name`、
`status`，不要使用未出现在接口契约中的内部字段名。

请求数据必须放在本文指定的位置。放错位置不属于接口契约的一部分，前端不要依赖后端
从 body、query 或 URL 之间兜底读取数据。

## 接口总览

| 操作 | 方法和路径 | 请求数据硬性要求 |
| --- | --- | --- |
| 创建一个 group | `POST /api/groups` | 必须把一个 group 对象放在 body |
| 删除一个 group | `DELETE /api/groups/:id` | 必须把 `id` 放在 URL；body 不承载语义 |
| 全量更新一个 group | `PUT /api/groups/:id` | 必须把 `id` 放在 URL，完整更新内容放在 body |
| 部分更新一个 group | `PATCH /api/groups/:id` | 必须把 `id` 放在 URL，只把要修改的字段放在 body |
| 查询 group 列表 | `GET /api/groups?name=g1&status=enabled` | 查询条件必须放在 URL query；不允许用 body 传查询条件 |
| 获取一个 group | `GET /api/groups/:id` | 必须把 `id` 放在 URL；body 不承载语义 |
| 创建多个 group | `POST /api/groups/batch` | body 必须使用 `{ "items": [...] }` |
| 删除多个 group | `DELETE /api/groups/batch` | body 必须使用 `{ "ids": [...] }` |
| 全量更新多个 group | `PUT /api/groups/batch` | body 必须使用 `{ "items": [...] }`，每个 item 必须带 `id` |
| 部分更新多个 group | `PATCH /api/groups/batch` | body 必须使用 `{ "items": [...] }`，每个 item 必须带 `id` 和要修改的字段 |

## 列表通用查询参数

除业务字段过滤外，列表接口的通用查询参数由后端按资源逐个启用：分页由 model 嵌入
`model.Pagination` 启用，游标分页由 `model.Cursor` 启用，排序、展开关联、时间范围、
字段操作符过滤等常规参数由 `model.Query` 启用（`model.Query` 同时包含前两者），OR 过滤、
跳过总数等改变查询语义或执行方式的参数由 `model.UnsafeQuery` 单独启用。资源未启用
对应能力时，传这些参数会返回 400。某个资源支持哪些参数以 Swagger 为准。

| 能力 | 参数 | 启用方式 | 示例 |
| --- | --- | --- | --- |
| 分页 | `_page`（从 1 开始）、`_size` | `model.Pagination` | `?_page=1&_size=20` |
| 排序 | `_sort_by`，逗号分隔多字段，方向 `asc`/`desc`（默认 `asc`） | `model.Query` | `?_sort_by=created_at desc,name` |
| 展开关联 | `_expand`，逗号分隔，`all` 表示全部可展开字段 | `model.Query` | `?_expand=all` |
| 时间范围 | `_time_column` 指定时间列，`_start_time`、`_end_time` 是范围边界（含边界）；格式支持 `2006-01-02 15:04:05`、`2006-01-02T15:04[:05]`、`2006-01-02`（纯日期作 `_end_time` 时覆盖到当天末尾）、带时区偏移的 RFC 3339、Unix 秒/毫秒时间戳，无时区格式按服务器本地时区解析，格式非法返回 400 | `model.Query` | `?_time_column=created_at&_start_time=2025-01-01 00:00:00&_end_time=2025-01-02 00:00:00` |
| 字段操作符过滤 | `字段[op]=值`，与其他条件按 AND 组合；op 支持 `eq`、`ne`、`gt`、`gte`、`lt`、`lte`、`in`、`notin`（逗号分隔多值）、`like`、`notlike`（子串匹配）；字段名、操作符非法或与 `_or=true` 同用返回 400，空值视为不过滤 | `model.Query` | `?age[gte]=18&remark[like]=hello&name[notin]=a,b` |
| 游标分页 | `_cursor_value`、`_cursor_field`、`_cursor_next`；使用游标时响应不返回 `total` | `model.Cursor` | `?_cursor_value=xxx&_cursor_next=true` |
| OR 过滤 | `_or` 为 `true` 时多个业务字段过滤条件之间用 OR 连接 | `model.UnsafeQuery` | `?name=g1&status=enabled&_or=true` |
| 跳过总数 | `_no_total` 为 `true` 时响应不返回 `total` | `model.UnsafeQuery` | `?_no_total=true` |

命名约定：框架控制参数一律以 `_` 开头，`_` 前缀是框架保留命名空间，业务字段的
query 名不要以 `_` 开头。反过来，所有裸名参数都属于业务字段过滤（如 `?name=xxx`），
`page`、`size`、`limit` 这类裸名也可以放心用作业务过滤列，不会与框架参数冲突。
方括号是操作符过滤的保留语法：`字段[op]=值` 不是业务字段精确过滤，裸名精确过滤
（`?age=10`）和同字段的操作符过滤（`?age[gt]=20`）可以同时出现，各自独立生效并
按 AND 组合。

## 请求体格式

创建一个 group：

```json
{
  "name": "g1",
  "status": "enabled"
}
```

部分更新一个 group：

```json
{
  "status": "disabled"
}
```

创建多个、全量更新多个、部分更新多个 group：

```json
{
  "items": [
    {
      "id": "group-id-1",
      "name": "g1",
      "status": "enabled"
    },
    {
      "id": "group-id-2",
      "name": "g2",
      "status": "disabled"
    }
  ]
}
```

批量创建时，`items` 中通常不需要传 `id`，`id` 由后端生成并在响应中返回；批量全量
更新和批量部分更新时，每个 item 必须能标识要更新的资源，通常需要传 `id`。

删除多个 group：

```json
{
  "ids": ["group-id-1", "group-id-2"]
}
```

## 响应结构

所有接口统一返回如下 envelope：

```json
{
  "code": 0,
  "msg": "success",
  "data": {},
  "trace_id": "..."
}
```

- 成功时 HTTP 状态码为 `200`，`code` 固定为 `0`，业务数据在 `data` 中。
- 失败时 HTTP 状态码为 `4xx`/`5xx`，`code` 为非 0 错误码，`msg` 是可展示的错误信息。
  前端以 HTTP 状态码或 `code != 0` 判定失败，具体错误码含义以 Swagger 为准。
- `trace_id` 用于排障，反馈接口问题时请带上。

列表接口的 `data` 固定为如下结构（`_no_total=true` 时没有 `total`）：

```json
{
  "total": 2,
  "items": [{ "id": "group-id-1" }, { "id": "group-id-2" }]
}
```

## 关键规则

- 单个资源的 `id` 必须放在 URL 中，例如 `/api/groups/group-id-1`。
- body 中的 `id` 不能替代 URL 中的 `:id`。
- `GET` 请求只认 URL query 中的查询条件，不使用 body。
- `PUT` 表示全量更新，body 应包含完整更新内容。
- `PATCH` 表示部分更新，body 只放需要修改的字段。
- 批量创建、批量全量更新、批量部分更新统一使用 `items`。
- 批量删除统一使用 `ids`。
- 不要把参数放到“看起来也能传”的其他位置；本文指定的位置就是前端对接时必须遵守的位置。
- 前端解析响应只依赖统一 envelope 和 `data` 结构，不要依赖 `msg` 的具体文案做逻辑判断。
