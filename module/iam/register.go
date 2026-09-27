package iam

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"

	"github.com/hydroan/gst/consts"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/router"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	serviceiamprofile "github.com/hydroan/gst/internal/service/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	serviceiamuser "github.com/hydroan/gst/internal/service/iam/user"
	"github.com/hydroan/gst/module"
)

// Register registers IAM models and API routes.
//
// API Routes:
//
// Session routes:
//   - GET    /api/iam/session/current
//   - DELETE /api/iam/session/current
//   - GET    /api/iam/sessions
//   - GET    /api/iam/admin/sessions
//   - GET    /api/iam/admin/sessions/:id
//   - DELETE /api/iam/admin/sessions/:id
//   - GET    /api/iam/admin/users/:id/sessions
//   - DELETE /api/iam/admin/users/:id/sessions
//   - GET    /api/iam/sessions/:id
//   - DELETE /api/iam/sessions
//   - DELETE /api/iam/sessions/:id
//
// Note: DELETE /api/iam/sessions/:id treats id=others as a reserved
// self-service bulk logout that revokes every other session of the current user.
//
// Account management routes:
//   - POST   /api/login
//   - POST   /api/logout
//   - POST   /api/signup
//   - POST   /api/iam/change-password
//   - POST   /api/iam/reset-password
//   - POST   /api/iam/admin/users
//   - GET    /api/iam/admin/users
//   - GET    /api/iam/admin/users/:id
//   - PATCH  /api/iam/admin/users/:id
//   - GET    /api/iam/profile
//   - PATCH  /api/iam/profile
//
// Middleware and interceptor: Register mounts none. The project mounts the
// session check itself, middleware.IAMSession() in its middleware package
// with middleware.RegisterAuth and, when it serves gRPC,
// interceptor.IAMSession() in its interceptor package with
// interceptor.RegisterAuth — the way a project that copied the module does
// — ahead of whatever else of its own reads the caller: the authenticated
// chains run in registration order.
//
// TODO: a project that mounts no auth middleware serves every route of the
// authenticated group to anyone, and the HTTP listener does not even warn
// (the gRPC listener warns, see grpcserver.Run). Refuse to start when
// routes need authentication and nothing authenticates.
//
// Configuration:
//   - IAM_SESSION_EXPIRATION sets the session lifetime; it defaults to 8 hours.
//     It is read at registration so an unparseable value fails startup rather
//     than the first login.
func Register() {
	// Sessions live only in Redis, so a deployment without it cannot
	// authenticate anyone. Refusing at startup states that in the one place a
	// deployment can still act on it.
	router.OnRoutesReady(func(context.Context, map[string][]string) error {
		return requireRedisEnabled()
	})

	// Resolve once during registration so invalid environment configuration
	// fails during startup rather than at the first login.
	_ = serviceiamsession.GetSessionExpiration()

	// TODO: throttle POST /api/login by client IP. The route is public, so the
	// limiter registers with Register (global scope), not RegisterAuth. For
	// projects that copy this module to get it as well, build it in a
	// zero-argument constructor in a file of the public middleware package — a
	// ratelimiter.RateLimiter narrowed to this one path through
	// ratelimiter.WithSkipFunc, whose default key is already the client IP —
	// and declare that file in module.json with scope "global".
	module.Use(module.NewWrapper("/login", "id", true, &serviceiamaccount.LoginService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/logout", "id", false, &serviceiamaccount.LogoutService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/signup", "id", true, &serviceiamaccount.SignupService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/iam/change-password", "id", false, &serviceiamaccount.ChangePasswordService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/iam/reset-password", "id", false, &serviceiamaccount.ResetPasswordService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserCreateService{}), module.CRUD(consts.Create))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserListService{}), module.CRUD(consts.List))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserGetService{}), module.CRUD(consts.Get))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserPatchService{}), module.CRUD(consts.Patch))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfileGetService{}), module.Exact(consts.Get))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfilePatchService{}), module.Exact(consts.Patch))

	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentGetService{}), module.Exact(consts.Get))
	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentDeleteService{}), module.Exact(consts.Delete))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionListService{}), module.CRUD(consts.List))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionListService{}), module.CRUD(consts.List))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionGetService{}), module.CRUD(consts.Get))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionDeleteService{}), module.CRUD(consts.Delete))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionListService{}), module.CRUD(consts.List))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionDeleteService{}), module.Exact(consts.Delete))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionGetService{}), module.CRUD(consts.Get))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionDeleteAllService{}), module.Exact(consts.Delete))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionDeleteService{}), module.CRUD(consts.Delete))

	// TODO: add DELETE /api/iam/admin/users/:id. Deleting the IAM rows is the
	// easy half; the account's authz role bindings have to go with it, and IAM
	// cannot write them without depending on the authz module. It needs an
	// adapter the way module/mfa installs one for AccountAdministrator, so that
	// a deployment without authz still deletes cleanly.

	// Register the backing IAM tables. Baseline accounts are application
	// data: create them explicitly through the standard database chain in a
	// startup hook such as router.OnRoutesReady, using
	// serviceiamaccount.NewPasswordCredential for password hashing.
	modelregistry.Register[*modeliamuser.User]()
	modelregistry.Register[*modeliamaccount.PasswordCredential]()
	modelregistry.Register[*modeliamaccount.EmailIdentity]()
	modelregistry.Register[*modeliamprofile.Profile]()
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	return serviceiamsession.GetSessionExpiration()
}

// requireRedisEnabled fails startup when IAM is registered without the Redis it
// stores every session in.
//
// Without this the failure surfaces one request at a time, as a 500 from the
// first login, long after the only moment the configuration could have been
// corrected.
func requireRedisEnabled() error {
	if config.App.Redis.Enabled {
		return nil
	}
	return errors.Newf("module iam requires redis: set %s=true or the redis.enabled config key", config.REDIS_ENABLED)
}
