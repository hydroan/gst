package iam

import (
	"time"

	"github.com/hydroan/gst/cronjob"
	cronjobiam "github.com/hydroan/gst/internal/cronjob/iam"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	serviceiamemail "github.com/hydroan/gst/internal/service/iam/email"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	serviceiamuser "github.com/hydroan/gst/internal/service/iam/user"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/service"
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
//   - POST   /api/iam/session/heartbeat
//   - GET    /api/iam/session/current
//   - DELETE /api/iam/session/current
//   - GET    /api/iam/sessions
//   - GET    /api/iam/admin/sessions
//   - GET    /api/iam/admin/sessions/:id
//   - DELETE /api/iam/sessions
//   - DELETE /api/iam/sessions/:id
//   - GET    /api/online-users
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
// Email workflow routes:
//   - POST   /api/iam/email/verification-confirm
//   - POST   /api/iam/email/verification-request
//   - POST   /api/iam/email/verification-resend
//   - POST   /api/iam/email/password-reset-confirm
//   - POST   /api/iam/email/password-reset-request
//   - POST   /api/iam/email/change-request
//   - POST   /api/iam/email/change-resend
//   - POST   /api/iam/email/change-cancel
//   - POST   /api/iam/email/change-confirm
//
// Middleware:
//   - IAMSession for protected IAM routes and session-aware APIs
//
// Scheduled jobs:
//   - CleanupOnlineUser runs every 30 seconds and starts immediately after bootstrap
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

	module.Use(module.NewWrapper("/login", "id", true, &serviceiamaccount.LoginService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/logout", "id", false, &serviceiamaccount.LogoutService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/signup", "id", true, &serviceiamaccount.SignupService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/change-password", "id", false, &serviceiamaccount.ChangePasswordService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/reset-password", "id", false, &serviceiamaccount.ResetPasswordService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/account-status", "id", false, &serviceiamaccount.AccountStatusService{}), consts.PHASE_CREATE)
	module.Use(
		module.NewWrapper("/iam/users", "id", false, &serviceiamuser.UserService{}),
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_LIST,
		consts.PHASE_GET,
		consts.PHASE_CREATE_MANY,
		consts.PHASE_DELETE_MANY,
	)
	module.UseCustom(module.NewWrapper("/iam/users/:id", "id", false, &serviceiamuser.UserPatchService{}), consts.PHASE_PATCH)

	module.Use(module.NewWrapper("/iam/session/heartbeat", "id", false, &serviceiamsession.HeartbeatService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentListService{}), consts.PHASE_LIST)
	module.UseCustom(module.NewWrapper("/iam/session/current", "id", false, &serviceiamsession.CurrentDeleteService{}), consts.PHASE_DELETE)
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsListService{}), consts.PHASE_LIST)
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsListService{}), consts.PHASE_LIST)
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsGetService{}), consts.PHASE_GET)
	module.Use(module.NewWrapper("/iam/admin/sessions", "id", false, &serviceiamsession.AdminSessionsDeleteService{}), consts.PHASE_DELETE)
	module.UseCustom(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsListService{}), consts.PHASE_LIST)
	module.UseCustom(module.NewWrapper("/iam/admin/users/:id/sessions", "id", false, &serviceiamsession.AdminUserSessionsDeleteService{}), consts.PHASE_DELETE)
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsGetService{}), consts.PHASE_GET)
	module.UseCustom(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteAllService{}), consts.PHASE_DELETE)
	module.Use(module.NewWrapper("/iam/sessions", "id", false, &serviceiamsession.SessionsDeleteService{}), consts.PHASE_DELETE)
	module.Use(module.NewWrapper("/online-users", "id", false, &service.Base[*OnlineUser, *OnlineUser, *OnlineUser]{}), consts.PHASE_LIST)

	module.Use(module.NewWrapper("/iam/email/verification-request", "id", true, &serviceiamemail.VerificationRequestService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/verification-resend", "id", true, &serviceiamemail.VerificationResendService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/verification-confirm", "id", true, &serviceiamemail.VerificationConfirmService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/password-reset-confirm", "id", true, &serviceiamemail.PasswordResetConfirmService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/password-reset-request", "id", true, &serviceiamemail.PasswordResetRequestService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/change-request", "id", false, &serviceiamemail.ChangeRequestService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/change-resend", "id", false, &serviceiamemail.ChangeResendService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/change-cancel", "id", true, &serviceiamemail.ChangeCancelService{}), consts.PHASE_CREATE)
	module.Use(module.NewWrapper("/iam/email/change-confirm", "id", false, &serviceiamemail.ChangeConfirmService{}), consts.PHASE_CREATE)

	// create default users
	if len(cfg.DefaultUsers) > 0 {
		for _, u := range cfg.DefaultUsers {
			if err := modeliamuser.GenerateHashedPassword(u); err != nil {
				panic(err)
			}
		}
		model.Register(cfg.DefaultUsers...)
	}

	// cleanup the oneline user that not active every 30 seconds, will run immediately after application bootstrap.
	cronjob.Register(cronjobiam.CleanupOnlineUser, "*/30 * * * * *", "cleanup online user", cronjob.Config{RunImmediately: true})
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	if iamConfig.SessionExpiration == 0 {
		return 8 * time.Hour
	}
	return iamConfig.SessionExpiration
}
