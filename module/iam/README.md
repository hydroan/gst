# module/iam

身份、凭证与会话。

账号、凭证、邮箱和资料落在 MySQL 的四张表里，会话完全活在 Redis 里，两者只靠一个 user id 相连。

## 它提供什么

- **账号** —— `users`、`password_credentials`、`email_identities`、`profiles` 四张表。注册、登录、登出、改密码、管理员重置密码。
- **会话** —— 一个不透明 cookie 背后是 Redis 里的快照，配套自服务与管理员两个视角的列表和吊销接口。
- **防爆破** —— 连续失败的登录在 Redis 里计数，达到阈值锁定一个窗口。
- **用户管理** —— 建号、列表、查询、更新，以及吊销任意用户的会话。

## 接入

```go
import "github.com/hydroan/gst/module/iam"

iam.Register()
```

**必须在 authz 之前注册**：`Authz` 读的是 `IAMSession` 放到请求上的 subject 和 tenant。

**Redis 是硬依赖，不是缓存**：会话不落库，没有 Redis 谁也认证不了。所以注册时就会检查
`Redis.Enabled`，没开直接让启动失败——这件事该在部署还能改配置的那一刻说。

复制到业务项目：

```sh
gg module copy iam
```

## 配置

| 键 | 默认 | 它约束什么 |
| --- | --- | --- |
| `IAM_SESSION_EXPIRATION` | `8h` | 会话寿命，登录时定死 |
| `IAM_SESSION_USER_STATE_TTL` | `30s` | 用户状态缓存最多能陈旧多久 |
| `IAM_LOGIN_FAILURE_LIMIT` | `5` | 连续失败几次锁定账号 |
| `IAM_LOGIN_FAILURE_WINDOW` | `15m` | 锁定持续多久 |

四个都在注册时或首次使用时读取，非法值让启动失败，而不是让第一个用到它的请求失败。

## 深入

- **[SESSION.md](SESSION.md)** —— 会话怎么存、每个请求怎么校验、吊销时到底动了哪些键。
  会话不落库，所以这部分的行为全部由 Redis 的键布局决定，值得单独读。
- **[TENANT.md](TENANT.md)** —— 开启多租户后，哪些接口属于租户、哪些跨租户、哪些只认当前登录主体。
