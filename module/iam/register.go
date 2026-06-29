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

// iamConfig stores the configuration for iam module
var iamConfig Config

// Config is the configuration for iam module.
type Config struct {
	DefaultUsers      []*DefaultUser // DefaultUsers are default users to create on registration.
	SessionExpiration time.Duration  // SessionExpiration is the session expiration time, default is 8 hours
}

// DefaultUser describes a user and password credential created during module registration.
type DefaultUser struct {
	ID                 string
	Username           string
	Password           string
	Email              string
	Status             modeliamuser.UserStatus
	MustChangePassword bool
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
//   - PATCH  /api/iam/admin/users/:id/status
//   - GET    /api/iam/profile
//   - PATCH  /api/iam/profile
//
// Middleware:
//   - IAMSession for protected IAM routes and session-aware APIs
//
// Configuration:
//   - SessionExpiration defaults to 8 hours when not configured
//
// NOTE: Register IAM modules before authz modules because authz middleware depends on IAMSession.
func Register(config ...Config) {
	cfg := Config{
		SessionExpiration: 8 * time.Hour, // default session expiration time
	}
	if len(config) > 0 {
		cfg = config[0]
		// Set default session expiration if not provided
		if cfg.SessionExpiration == 0 {
			cfg.SessionExpiration = 8 * time.Hour
		}
	}

	// Store only runtime configuration needed after registration.
	iamConfig = Config{SessionExpiration: cfg.SessionExpiration}

	// Set session expiration in service layer
	serviceiamsession.SetSessionExpiration(cfg.SessionExpiration)

	// Register auth middleware before protected routes so auth handlers are attached deterministically.
	middleware.RegisterAuth(middleware.IAMSession())

	module.Use(module.NewWrapper("/login", "id", true, &serviceiamaccount.LoginService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/logout", "id", false, &serviceiamaccount.LogoutService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/signup", "id", true, &serviceiamaccount.SignupService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/change-password", "id", false, &serviceiamaccount.ChangePasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/reset-password", "id", false, &serviceiamaccount.ResetPasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/admin/users/:id/status", "id", false, &serviceiamuser.UserStatusPatchService{}), module.Exact(consts.PHASE_PATCH))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfileGetService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/profile", "id", false, &serviceiamprofile.ProfilePatchService{}), module.Exact(consts.PHASE_PATCH))

	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentGetService{}), module.Exact(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsDeleteService{}), module.CRUD(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteAllService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteService{}), module.CRUD(consts.PHASE_DELETE))

	// Register the backing IAM tables and optional default users.
	defaultUsers, defaultCredentials, defaultEmailIdentities := buildDefaultUserRecords(cfg.DefaultUsers)
	model.Register[*modeliamuser.User](defaultUsers...)
	model.Register[*modeliamaccount.PasswordCredential](defaultCredentials...)
	model.Register[*modeliamaccount.EmailIdentity](defaultEmailIdentities...)
	model.Register[*modeliamprofile.Profile]()
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	if iamConfig.SessionExpiration == 0 {
		return 8 * time.Hour
	}
	return iamConfig.SessionExpiration
}

func buildDefaultUserRecords(configs []*DefaultUser) ([]*modeliamuser.User, []*modeliamaccount.PasswordCredential, []*modeliamaccount.EmailIdentity) {
	users := make([]*modeliamuser.User, 0, len(configs))
	credentials := make([]*modeliamaccount.PasswordCredential, 0, len(configs))
	emailIdentities := make([]*modeliamaccount.EmailIdentity, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		if cfg.Username == "" {
			panic("default user username is required")
		}

		userID := cfg.ID
		if userID == "" {
			userID = cfg.Username
		}
		status := cfg.Status
		if status == "" {
			status = modeliamuser.UserStatusActive
		}

		user := &modeliamuser.User{
			Username: cfg.Username,
			Status:   status,
		}
		user.ID = userID
		credential, err := serviceiamaccount.NewPasswordCredential(userID, cfg.Password, cfg.MustChangePassword)
		if err != nil {
			panic(err)
		}
		if cfg.Email != "" {
			emailIdentity, err := serviceiamaccount.NewEmailIdentity(userID, cfg.Email)
			if err != nil {
				panic(err)
			}
			emailIdentities = append(emailIdentities, emailIdentity)
		}

		users = append(users, user)
		credentials = append(credentials, credential)
	}
	return users, credentials, emailIdentities
}
