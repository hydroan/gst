# logmgmt 租户边界

本文记录 `module/logmgmt` 在多租户项目中的边界。登录日志与操作日志都是平台级审计数据：它们记录「谁在什么时候做了什么」，主体是账号与操作行为本身，不归属某个 tenant。

## 当前接口结论

- `GET /api/log/loginlog`、`GET /api/log/loginlog/:id`
- `GET /api/log/operationlog`、`GET /api/log/operationlog/:id`

四条查询接口都是**跨租户读取**：`LoginLog` 与 `OperationLog` 模型都没有 `TenantID` 列，List 也没有任何租户过滤钩子，返回的是全平台的日志。

## 跨 tenant 的边界要求

正因为读取是跨租户的，这些接口只应授予 root 或平台级管理员，绝不应授予普通 tenant admin：

- `OperationLog` 默认记录请求体与响应体全文（`audit.record_request_body` / `audit.record_response_body` 默认开启），拿到读取权限即可读取全平台所有租户的请求与响应内容。
- 部署方应通过 authz RBAC 把 `log/*` 路由限制在平台管理员角色内；不装 authz 时它们仅受登录态保护，多租户部署必须自行加闸。

## 与登录链路的关系

`LoginLog` 的写入方是 `internal/service/logmgmt` 在包初始化时向 `authn` 注册的登录观察者，事件来自 IAM 登录/登出流程，覆盖成功、失败、登出三态。观察者永不阻塞登录。

## 模型边界

- `LoginLog` / `OperationLog` 不加 `TenantID`：审计数据的隔离应表达在读取授权上，而不是把平台审计拆进租户。
- 若未来产品需要「租户管理员只看本租户成员的日志」，那是新的 in-tenant 查询接口，应带显式租户过滤与成员校验，而不是放宽现有跨租户接口。
