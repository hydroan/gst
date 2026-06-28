# IAM 租户边界

本文记录 `module/iam` 在开启多租户后的边界。IAM 的核心原则是：用户、凭证、身份、会话是全局身份主体数据，不直接归属某个 tenant；tenant 成员关系和权限由 `module/authz` 的 RoleBinding、Role 和 Casbin tenant domain 表达。

## 不属于 tenant 的接口

这些接口只处理当前登录主体或公开认证流程，不需要 tenant 上下文：

- `POST /api/signup`
- `POST /api/login`
- `POST /api/logout`
- `POST /api/iam/change-password`
- `GET /api/iam/profile`
- `PATCH /api/iam/profile`
- `GET /api/iam/session/current`
- `DELETE /api/iam/session/current`
- `GET /api/iam/sessions`
- `GET /api/iam/sessions/:id`
- `DELETE /api/iam/sessions`
- `DELETE /api/iam/sessions/:id`
- `DELETE /api/iam/sessions/others`

这些接口的判断依据是当前 session 对应的 user，不应该因为切换 tenant 而改变用户自己的资料、密码或会话列表。

## in tenant 的接口

这些接口管理某个目标用户，必须使用当前 tenant 做授权，并校验目标用户属于当前 tenant：

- `PATCH /api/iam/admin/users/:id/status`
- `POST /api/iam/reset-password`
- `GET /api/iam/admin/users/:id/sessions`
- `DELETE /api/iam/admin/users/:id/sessions`

调用者需要在当前 tenant 中拥有对应 RBAC 权限。目标用户需要在当前 tenant 中存在 RoleBinding，否则服务层拒绝操作。内置 root 用户仍作为 break-glass 管理员绕过 tenant 校验。

## 跨 tenant 的接口

这些接口查看或操作全局会话集合，不按 tenant 过滤数据：

- `GET /api/iam/admin/sessions`
- `GET /api/iam/admin/sessions/:id`
- `DELETE /api/iam/admin/sessions/:id`

这些接口属于平台级管理面，目前保持 root-only。业务项目如果要开放给非 root 管理员，应先设计平台级 tenant、全局管理员角色和审计策略，不应直接授予普通 tenant 管理员。

## 模型边界

- `User` 不增加 `TenantID`，表示全局身份主体。
- `PasswordCredential` 不增加 `TenantID`，表示用户的登录凭证。
- `EmailIdentity` 不增加 `TenantID`，表示用户的邮箱身份。
- `Profile` 不增加 `TenantID`，表示用户的全局基础资料。
- `Session` 不增加 `TenantID`，表示一次登录会话。
- 用户属于哪些 tenant、在 tenant 内拥有哪些权限，由 `authz.RoleBinding{TenantID, SubjectID, RoleID}` 表达。

## tenant 来源

`module/authz` 支持项目提供 TenantResolver。resolver 返回空 tenant 时，authz 会回退到 `rbac.DefaultTenant`。

通过 `gg module add authz` 使用内置模块时，在项目的模块注册处传入 resolver：

```go
authz.Register(authz.Config{
    TenantResolver: authz.HeaderTenantResolver("X-Tenant-ID"),
})
```

通过 `gg module copy authz` 复制到业务项目后，copy manifest 会注册零参 `middleware.Authz()`。项目需要在自己的接入文件中设置 resolver：

```go
middleware.SetAuthzTenantResolver(func(c *gin.Context) (string, error) {
    return c.GetHeader("X-Tenant-ID"), nil
})
```

`HeaderTenantResolver` 和 header 示例只适合测试、demo 或可信网关注入 tenant 的场景。生产项目应优先从 session、JWT claims、子域名或项目自己的 tenant 上下文中解析 tenant，不要把任意客户端 header 当作可信身份来源。
