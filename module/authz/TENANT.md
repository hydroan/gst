# Authz 租户边界

本文记录 `module/authz` 在多租户项目中的边界。`module/authz` 是 tenant-aware RBAC 模块：`Role`、`RoleBinding` 和 Casbin tenant domain 表达租户内权限；`Menu` 和 `Routes` 表达全局权限目录；`system_root` 表达跨 tenant 的系统级角色。

## 当前接口结论

- `Role`、`RoleBinding` 是 in tenant 资源。
- `Menu` 是全局权限目录，不属于某个 tenant。
- `GET /api/authz/menus` 是特殊接口：底层读取全局 `Menu` 目录，但返回当前用户在当前 tenant 下可见的菜单树。
- `Routes` 是全局后端路由快照，不属于某个 tenant。
- `AuthzRule` 当前只注册内部存储表，不暴露公开 CRUD API。
- `system_root` 是系统级角色，不通过 `RoleBinding` 管理。

## 不属于 tenant 的接口

- `GET /api/authz/routes`

  返回框架当前注册的全局后端路由目录。它可以被 RBAC 保护，但路由目录本身不是 tenant 数据，也不应该增加 `TenantID`。

## in tenant 的接口

这些接口管理租户内 RBAC 数据，必须按当前 tenant 授权，并且读写范围应限制在当前 tenant：

- `POST /api/authz/roles`
- `GET /api/authz/roles`
- `GET /api/authz/roles/:id`
- `PUT /api/authz/roles/:id`
- `PATCH /api/authz/roles/:id`
- `DELETE /api/authz/roles/:id`
- `POST /api/authz/role-bindings`
- `GET /api/authz/role-bindings`
- `GET /api/authz/role-bindings/:id`
- `DELETE /api/authz/role-bindings/:id`

`Role` 定义当前 tenant 下的一组权限。`Role.MenuIDs` 是后端路由权限生成的来源，同时也是前端菜单树可见范围的唯一来源。

`RoleBinding` 表达某个 subject 在某个 tenant 内拥有某个 role。创建 binding 时，binding 的 `TenantID` 必须和目标 role 的 `TenantID` 一致。

## in tenant 视图接口

- `GET /api/authz/menus`

该接口的资源来源是全局 `Menu` 目录，但响应会根据当前 request tenant 下的 `RoleBinding`、默认 role 和 `Role.MenuIDs` 过滤。因此它不是 tenant 拥有菜单资源，而是全局菜单目录在当前 tenant 下的可见投影。

`system_root` 用户绕过菜单过滤，可看到完整菜单目录。

## 跨 tenant 的接口

这些接口管理全局权限目录，会影响所有 tenant 的权限配置和菜单可见性，应视为平台级管理面：

- `POST /api/authz/menus`
- `GET /api/authz/menus/:id`
- `PUT /api/authz/menus/:id`
- `PATCH /api/authz/menus/:id`
- `DELETE /api/authz/menus/:id`

`Menu` 不带 `TenantID`。修改菜单的 backend `Routes` 会触发相关 role 的权限同步；删除菜单会从所有引用它的 role 的 `MenuIDs` 中移除。因此普通 tenant admin 不应该直接管理这些接口。

## 内部跨 tenant 能力

`AuthzRule` 表保存授权策略数据，包括 tenant 内 permission、tenant 内 role assignment，以及 `system_root` 这类系统级 assignment。当前模块只注册该表给 authz/rbac 的策略 adapter 使用，不暴露公开 CRUD API。业务代码不应直接通过通用 CRUD 管理 `AuthzRule`。

`system_root` 通过 RBAC 的 system role 能力表达，不属于任何 tenant，也不通过 `RoleBinding` 表达。系统级角色应由 bootstrap 或受控内部流程设置，不应开放给普通 tenant 管理员。

## 模型边界

- `Role` 有 `TenantID`，表示某个 tenant 内的角色。
- `RoleBinding` 有 `TenantID`，表示某个 subject 在某个 tenant 内绑定某个 role。
- `Menu` 不增加 `TenantID`，表示全局 capability、菜单和后端 route catalog。
- `Routes` 不落库，表示框架运行时注册的全局路由快照。
- `AuthzRule` 是 RBAC runtime 的内部存储，不作为业务模型暴露。

## tenant 来源

请求 tenant 只有一个来源：请求上下文中的 `CTX_TENANT_ID`。`Authz` 中间件按现值读取，为空时回退到默认 tenant（`tenant.Default`）。当项目同时使用 `module/iam` 时，`IAMSession` 会把 `Session.TenantID` 写入该上下文，因此登录时选择的 tenant 直接成为 authz 的 tenant 来源，多租户不需要任何额外配置。

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
