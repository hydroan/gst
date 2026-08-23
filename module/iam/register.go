package iam

import (
	"time"

	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	serviceiamprofile "github.com/hydroan/gst/internal/service/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	serviceiamuser "github.com/hydroan/gst/internal/service/iam/user"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Config is the configuration for iam module.
type Config struct {
	SessionExpiration time.Duration // SessionExpiration is the session expiration time. It defaults to 8 hours and can be configured by IAM_SESSION_EXPIRATION.
}

// Register registers IAM models, API routes, middleware, and scheduled jobs.
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
//   - GET    /api/iam/admin/users
//   - GET    /api/iam/admin/users/:id
//   - PATCH  /api/iam/admin/users/:id/status
//   - GET    /api/iam/profile
//   - PATCH  /api/iam/profile
//
// Middleware:
//   - IAMSession for protected IAM routes and session-aware APIs
//
// Configuration:
//   - SessionExpiration defaults to 8 hours when not configured.
//   - IAM_SESSION_EXPIRATION overrides the default when SessionExpiration is empty.
//
// NOTE: Register IAM modules before authz modules because authz middleware depends on IAMSession.
func Register(config ...Config) {
	cfg := Config{}
	if len(config) > 0 {
		cfg = config[0]
	}

	// Set session expiration in service layer
	serviceiamsession.SetSessionExpiration(cfg.SessionExpiration)
	// Resolve once during registration so invalid environment configuration fails during startup.
	_ = serviceiamsession.GetSessionExpiration()

	// Register auth middleware before protected routes so auth handlers are attached deterministically.
	middleware.RegisterAuth(middleware.IAMSession())

	// TODO: throttle POST /api/login by client IP. The route is public, so the
	// limiter belongs on middleware.Register (global scope) rather than
	// middleware.RegisterAuth, narrowed to this one path through
	// ratelimiter.WithSkipFunc; the default key function is already the client
	// IP. See module/mfa for the option shape.
	module.Use(module.NewWrapper("/login", "id", true, &serviceiamaccount.LoginService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/logout", "id", false, &serviceiamaccount.LogoutService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/signup", "id", true, &serviceiamaccount.SignupService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/change-password", "id", false, &serviceiamaccount.ChangePasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/reset-password", "id", false, &serviceiamaccount.ResetPasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/users", "id", false, &serviceiamuser.AdminUserGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/admin/users/:id/status", "id", false, &serviceiamuser.UserStatusPatchService{}), module.Exact(consts.PHASE_PATCH))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfileGetService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfilePatchService{}), module.Exact(consts.PHASE_PATCH))

	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentGetService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionDeleteService{}), module.CRUD(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionDeleteAllService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionDeleteService{}), module.CRUD(consts.PHASE_DELETE))

	// Register the backing IAM tables. Baseline accounts are application
	// data: create them explicitly through the standard database chain in a
	// startup hook such as router.OnRoutesReady, using
	// serviceiamaccount.NewPasswordCredential for password hashing.
	model.Register[*modeliamuser.User]()
	model.Register[*modeliamaccount.PasswordCredential]()
	model.Register[*modeliamaccount.EmailIdentity]()
	model.Register[*modeliamprofile.Profile]()
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	return serviceiamsession.GetSessionExpiration()
}
