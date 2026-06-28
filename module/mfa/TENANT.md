# MFA 租户边界

本文记录 `module/mfa` 在多租户项目中的边界。MFA 管理的是账号主体的第二因素，不直接归属某个 tenant。tenant 是否要求 MFA 是租户策略、权限或登录策略问题，不是 MFA 设备本身的归属问题。

## 当前接口结论

当前 `module/mfa` 没有 in tenant 接口，也没有跨 tenant 接口。所有已注册接口都不属于 tenant。

## 不属于 tenant 的接口

- `POST /api/mfa/totp/check`

  登录前检查账号是否需要 TOTP。该接口是 public pre-login 接口，通过 `AccountAuthenticator` 校验用户名和密码，再按认证出的账号 ID 查询该账号是否存在可用 TOTP 设备。它不使用 tenant membership。

- `POST /api/mfa/totp/bind`

  当前已登录用户发起 TOTP 绑定。绑定挑战和当前 `UserID`、当前 session 绑定，不归属 tenant。

- `POST /api/mfa/totp/confirm`

  当前已登录用户确认 TOTP 绑定。确认时要求绑定挑战匹配当前 `UserID` 和当前 session，不归属 tenant。

- `GET /api/mfa/totp/status`

  查询当前已登录用户自己的 TOTP 状态。查询范围是当前 `UserID`，不是当前 tenant。

- `POST /api/mfa/totp/verify`

  当前已登录用户验证 TOTP code。即使请求带了 device ID，校验范围也限定在当前 `UserID` 下的可用设备。

- `POST /api/mfa/totp/unbind`

  当前已登录用户解绑自己的 TOTP 设备。解绑前需要一次 fresh auth，解绑范围仍限定在当前 `UserID`。

## in tenant 的接口

当前没有。

如果以后新增租户管理员管理成员 MFA 的接口，例如重置、停用某个租户成员的 MFA，这类接口才属于 in tenant。它们应该要求当前 tenant 下的 RBAC 权限，并校验目标用户属于当前 tenant。

## 跨 tenant 的接口

当前没有。

如果以后新增平台级全局查询、清理或强制解绑 MFA 设备的接口，这类接口属于跨 tenant 管理面。它们应该只对 root 或 platform admin 开放，不应授予普通 tenant admin。

## 模型边界

- `TOTPDevice` 不需要增加 `TenantID`。它表示用户账号的第二因素设备，不表示租户资源。
- MFA 设备与 `UserID` 或账号 ID 绑定，不与 tenant 绑定。
- tenant 是否要求登录或访问时完成 MFA，应由 tenant policy、authz 或项目自己的登录策略表达。
- `module/mfa` 通过 `AccountAuthenticator` 连接宿主账号系统。module copy 模式下，业务项目应安装自己的 authenticator adapter，不要让 `internal/service/mfa` 直接依赖具体 IAM 或 User 模型。

## 与 tenant resolver 的关系

当前 MFA 接口不依赖 authz tenant resolver。`POST /api/mfa/totp/check` 是 public pre-login 接口，默认不会经过 authz tenant resolver。

如果业务项目的登录本身是 tenant-scoped，应在项目自己的 `AccountAuthenticator` 中根据可信来源解析 tenant，例如 host、path、session，或由网关写入且服务端信任的 header。通用 MFA 模块不应该直接拥有 tenant 语义。
