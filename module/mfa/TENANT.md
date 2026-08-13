# MFA 租户边界

本文记录 `module/mfa` 在多租户项目中的边界。MFA 管理的是账号主体的第二因素，不直接归属某个 tenant。tenant 是否要求 MFA 是租户策略、权限或登录策略问题，不是 MFA 设备本身的归属问题。

## 不属于 tenant 的接口

- `POST /api/mfa/totp/bind`

  当前已登录用户发起 TOTP 绑定。绑定挑战和当前 `UserID`、当前 session 绑定，不归属 tenant。

- `POST /api/mfa/totp/confirm`

  当前已登录用户确认 TOTP 绑定。确认时要求绑定挑战匹配当前 `UserID` 和当前 session，不归属 tenant。

- `GET /api/mfa/totp/status`

  查询当前已登录用户自己的 TOTP 状态。查询范围是当前 `UserID`，不是当前 tenant。

- `POST /api/mfa/totp/unbind`

  当前已登录用户解绑自己的 TOTP 设备。解绑前需要一次 fresh auth——提交 TOTP code 或恢复码，证明仍持有第二因素本身；密码不被接受。解绑范围仍限定在当前 `UserID`。

登录时的二因子强制不是独立接口：`POST /api/login` 通过 authn login verifier 调用 MFA 校验，作用范围是登录的账号本身，不归属 tenant。

## in tenant 的接口

- `GET /api/mfa/admin/users/:id/totp`

  管理员查看目标用户的 TOTP 注册状态（脱敏视图，不含秘钥与恢复码哈希）。

- `DELETE /api/mfa/admin/users/:id/totp`

  管理员强制清除目标用户的全部 TOTP 设备。这是自助 unbind 砍掉密码路径后的救援通道：同时丢失设备与恢复码的账号只能由管理员重置。

两条接口的授权都经 `AccountAdministrator` 判定。内置 module adapter 委托框架 IAM 的 `EnsureTenantAdmin` 规则：system root 全局放行；tenant 管理员必须在当前 tenant 通过路由授权，且目标用户必须是该 tenant 成员；system root 永远不能作为 tenant 管理面的目标。

## 跨 tenant 的接口

当前没有。

system root 通过上述 in tenant 接口即可管理任意账号（`EnsureTenantAdmin` 对 root actor 全局放行）。如果以后新增平台级全局查询、批量清理接口，它们应该只对 root 或 platform admin 开放，不应授予普通 tenant admin。

## 模型边界

- `TOTPDevice` 不需要增加 `TenantID`。它表示用户账号的第二因素设备，不表示租户资源。
- MFA 设备与 `UserID` 或账号 ID 绑定，不与 tenant 绑定。
- tenant 是否要求登录或访问时完成 MFA，应由 tenant policy、authz 或项目自己的登录策略表达。
- `module/mfa` 通过 `AccountAdministrator` 连接宿主的管理授权模型。module copy 模式下，业务项目应从 service/mfa 之外的项目自有代码安装自己的 authorizer adapter；未安装时管理接口全部拒绝。
