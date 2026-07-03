# 前端 API 对接规范

本文档面向前端开发者，说明项目默认资源接口的 REST 对接约定。下面以 `group`
资源为例，对应资源路径为 `/api/groups`。

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

批量创建时，`items` 中通常不需要传 `id`；批量全量更新和批量部分更新时，每个 item
必须能标识要更新的资源，通常需要传 `id`。

删除多个 group：

```json
{
  "ids": ["group-id-1", "group-id-2"]
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

