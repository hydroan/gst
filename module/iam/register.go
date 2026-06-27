package iam

import (
	"time"

	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
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
	DefaultUsers      []*User       // DefaultUsers are default users to create on registration
	SessionExpiration time.Duration // SessionExpiration is the session expiration time, default is 8 hours
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
//   - DELETE /api/iam/sessions
//   - DELETE /api/iam/sessions/:id
//
// Account management routes:
//   - POST   /api/login
//   - POST   /api/logout
//   - POST   /api/signup
//   - POST   /api/iam/change-password
//   - POST   /api/iam/reset-password
//   - POST   /api/iam/account-status
//
// IAM resource routes:
//   - POST   /api/iam/users
//   - DELETE /api/iam/users/:id
//   - PATCH  /api/iam/users/:id
//   - GET    /api/iam/users
//   - GET    /api/iam/users/:id
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

	// Store config globally
	iamConfig = cfg

	// Set session expiration in service layer
	serviceiamsession.SetSessionExpiration(cfg.SessionExpiration)

	// Register auth middleware before protected routes so auth handlers are attached deterministically.
	middleware.RegisterAuth(middleware.IAMSession())

	module.Use(module.NewWrapper("/login", "id", true, &serviceiamaccount.LoginService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/logout", "id", false, &serviceiamaccount.LogoutService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/signup", "id", true, &serviceiamaccount.SignupService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/change-password", "id", false, &serviceiamaccount.ChangePasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/reset-password", "id", false, &serviceiamaccount.ResetPasswordService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(module.NewWrapper("/iam/account-status", "id", false, &serviceiamaccount.AccountStatusService{}), module.CRUD(consts.PHASE_CREATE))
	module.Use(
		module.NewWrapper("/iam/users", "id", false, &serviceiamuser.UserService{}),
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_LIST,
			consts.PHASE_GET,
			consts.PHASE_CREATE_MANY,
			consts.PHASE_DELETE_MANY,
		),
	)
	module.Use(module.NewWrapper("/iam/users/:id", "id", false, &serviceiamuser.UserPatchService{}), module.Exact(consts.PHASE_PATCH))

	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsListService{}), module.CRUD(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsDeleteService{}), module.CRUD(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsListService{}), module.Exact(consts.PHASE_LIST))
	module.Use(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsDeleteService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsGetService{}), module.CRUD(consts.PHASE_GET))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteAllService{}), module.Exact(consts.PHASE_DELETE))
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteService{}), module.CRUD(consts.PHASE_DELETE))

	// create default users
	if len(cfg.DefaultUsers) > 0 {
		for _, u := range cfg.DefaultUsers {
			if err := modeliamuser.GenerateHashedPassword(u); err != nil {
				panic(err)
			}
		}
		model.Register(cfg.DefaultUsers...)
	}
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	if iamConfig.SessionExpiration == 0 {
		return 8 * time.Hour
	}
	return iamConfig.SessionExpiration
}
