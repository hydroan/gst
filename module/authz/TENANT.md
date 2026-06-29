# Authz 租户边界

本文记录 `module/authz` 在多租户项目中的边界。`module/authz` 是 tenant-aware RBAC 模块：`Role`、`RoleBinding` 和 Casbin tenant domain 表达租户内权限；`Menu` 和 `Routes` 表达全局权限目录；`system_root` 表达跨 tenant 的系统级角色。

## 当前接口结论

- `Role`、`RoleBinding` 是 in tenant 资源。
- `Menu` 是全局权限目录，不属于某个 tenant。
- `GET /api/authz/menus` 是特殊接口：底层读取全局 `Menu` 目录，但返回当前用户在当前 tenant 下可见的菜单树。
- `Routes` 是全局后端路由快照，不属于某个 tenant。
- `CasbinRule` 当前只注册内部存储表，不暴露公开 CRUD API。
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

`Role` 定义当前 tenant 下的一组权限。`Role.MenuIDs` 是后端路由权限生成的来源，`Role.MenuPartialIDs` 只用于前端菜单树中保留部分选中的父级节点，不直接授予后端 API 权限。

`RoleBinding` 表达某个 subject 在某个 tenant 内拥有某个 role。创建 binding 时，binding 的 `TenantID` 必须和目标 role 的 `TenantID` 一致。

## in tenant 视图接口

- `GET /api/authz/menus`

该接口的资源来源是全局 `Menu` 目录，但响应会根据当前 request tenant 下的 `RoleBinding`、默认 role、`Role.MenuIDs`、`Role.MenuPartialIDs` 和 `Menu.DomainPattern` 过滤。因此它不是 tenant 拥有菜单资源，而是全局菜单目录在当前 tenant 下的可见投影。

`system_root` 用户绕过菜单过滤，可看到完整菜单目录。

## 跨 tenant 的接口

这些接口管理全局权限目录，会影响所有 tenant 的权限配置和菜单可见性，应视为平台级管理面：

- `POST /api/authz/menus`
- `GET /api/authz/menus/:id`
- `PUT /api/authz/menus/:id`
- `PATCH /api/authz/menus/:id`
- `DELETE /api/authz/menus/:id`

`Menu` 不带 `TenantID`。修改菜单的 backend `Routes` 会触发相关 role 的权限同步；删除菜单会从所有引用它的 role 中移除 `MenuIDs` 和 `MenuPartialIDs`。因此普通 tenant admin 不应该直接管理这些接口。

## 内部跨 tenant 能力

`CasbinRule` 表保存 Casbin 策略数据，包括 tenant 内 permission、tenant 内 role assignment，以及 `system_root` 这类系统级 assignment。当前模块只注册该表给 Casbin adapter 使用，不暴露公开 CRUD API。业务代码不应直接通过通用 CRUD 管理 `CasbinRule`。

`system_root` 通过 RBAC 的 system role 能力表达，不属于任何 tenant，也不通过 `RoleBinding` 表达。系统级角色应由 bootstrap 或受控内部流程设置，不应开放给普通 tenant 管理员。

## 模型边界

- `Role` 有 `TenantID`，表示某个 tenant 内的角色。
- `RoleBinding` 有 `TenantID`，表示某个 subject 在某个 tenant 内绑定某个 role。
- `Menu` 不增加 `TenantID`，表示全局 capability、菜单和后端 route catalog。
- `Routes` 不落库，表示框架运行时注册的全局路由快照。
- `CasbinRule` 是 RBAC runtime 的内部存储，不作为业务模型暴露。

## tenant 来源

`module/authz` 的默认 resolver 会优先读取请求上下文中的 `CTX_TENANT_ID`；为空时回退到 `rbac.DefaultTenant`。当项目同时使用 `module/iam` 时，`IAMSession` 会把 `Session.TenantID` 写入该上下文，因此登录时选择的 tenant 可以直接成为 authz 的默认 tenant 来源。

如果项目的 tenant 来源不是 IAM session，例如 JWT claims、子域名或可信网关注入 header，可以在注册时传入 `TenantResolver`：

```go
authz.Register(authz.Config{
    TenantResolver: authz.HeaderTenantResolver("X-Tenant-ID"),
})
```

`HeaderTenantResolver` 只适合测试、demo 或可信网关注入 tenant 的场景。生产项目应优先从 session、JWT claims、子域名或项目自己的 tenant 上下文中解析 tenant，不要把任意客户端 header 当作可信身份来源。
