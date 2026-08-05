# IAM 租户边界

本文记录 `module/iam` 在开启多租户后的边界。IAM 的核心原则是：用户、凭证、身份是全局身份主体数据，不直接归属某个 tenant；会话可以保存本次登录选择的当前 tenant，但不表达 tenant 成员关系或权限。tenant 成员关系和权限由 `module/authz` 的 RoleBinding、Role 和 Casbin tenant domain 表达。

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

`POST /api/login` 可以携带 `tenant_id` 来选择当前 session tenant。非空 `tenant_id` 会校验登录用户属于该 tenant；登录接口本身仍然不是 tenant-owned API。

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
- `Session` 可以保存 `TenantID`，表示本次登录会话选择的当前 tenant；它不是用户的 tenant membership 或权限来源。
- 用户属于哪些 tenant、在 tenant 内拥有哪些权限，由 `authz.RoleBinding{TenantID, SubjectID, RoleID}` 表达。

## tenant 来源

请求 tenant 只有一个来源：请求上下文中的 `CTX_TENANT_ID`。`IAMSession` 会把 `Session.TenantID` 写入它，`Authz` 中间件按现值读取，为空时回退到默认 tenant（`tenant.Default`）。

无论通过 `gg module add` 使用内置模块，还是通过 `gg module copy` 复制到业务项目（此时注册的是项目自己的零参 `Authz()`），只要项目使用 IAM session tenant，在登录时传入 `tenant_id` 即可，不需要任何额外配置。

如果项目的 tenant 来源不是 IAM session，例如 JWT claims、子域名或可信网关注入的 header，在 `IAMSession` 之后、`Authz` 之前注册一个项目自己的中间件覆写 `CTX_TENANT_ID`：

```go
middleware.RegisterAuth(func(c *gin.Context) {
    if tenantID := strings.TrimSpace(c.GetHeader("X-Tenant-ID")); tenantID != "" {
        c.Set(consts.CTX_TENANT_ID, tenantID)
    }
    c.Next()
})
```

`CTX_TENANT_ID` 是被双重信任的值：授权在这个 tenant 内判定，请求随后读写的 tenant 作用域行也 scope 到它。它只允许由部署方担保的来源写入（session、经受信网关校验后注入的 header、子域名解析），不要把任意客户端输入原样透传。
