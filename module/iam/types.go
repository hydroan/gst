package iam

import (
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// Account API aliases.
type (
	LoginReq  = modeliamaccount.LoginReq
	LoginRsp  = modeliamaccount.LoginRsp
	LogoutRsp = modeliamaccount.LogoutRsp
	SignupReq = modeliamaccount.SignupReq
	SignupRsp = modeliamaccount.SignupRsp

	ChangePasswordReq = modeliamaccount.ChangePasswordReq
	ChangePasswordRsp = modeliamaccount.ChangePasswordRsp

	ResetPasswordReq = modeliamaccount.ResetPasswordReq
	ResetPasswordRsp = modeliamaccount.ResetPasswordRsp

	AccountStatusReq = modeliamaccount.AccountStatusReq
	AccountStatusRsp = modeliamaccount.AccountStatusRsp
)

// User API aliases.
type (
	User               = modeliamuser.User
	UserStatus         = modeliamuser.UserStatus
	PasswordCredential = modeliamaccount.PasswordCredential
	EmailIdentity      = modeliamaccount.EmailIdentity
)

const (
	UserStatusActive   = modeliamuser.UserStatusActive
	UserStatusInactive = modeliamuser.UserStatusInactive
	UserStatusLocked   = modeliamuser.UserStatusLocked
)

// Session API aliases.
type (
	Session                  = modeliamsession.Session
	SessionView              = modeliamsession.SessionView
	AuthenticatedSessionRsp  = modeliamsession.AuthenticatedSessionRsp
	AuthenticatedSessionView = modeliamsession.AuthenticatedSessionView
	PrincipalView            = modeliamsession.PrincipalView
	Token                    = modeliamsession.Token

	CurrentGetReq    = modeliamsession.CurrentGetReq
	CurrentGetRsp    = modeliamsession.CurrentGetRsp
	CurrentDeleteReq = modeliamsession.CurrentDeleteReq
	CurrentDeleteRsp = modeliamsession.CurrentDeleteRsp

	SessionsListReq      = modeliamsession.SessionsListReq
	SessionsListRsp      = modeliamsession.SessionsListRsp
	SessionsGetReq       = modeliamsession.SessionsGetReq
	SessionsGetRsp       = modeliamsession.SessionsGetRsp
	SessionsDeleteReq    = modeliamsession.SessionsDeleteReq
	SessionsDeleteRsp    = modeliamsession.SessionsDeleteRsp
	SessionsDeleteAllReq = modeliamsession.SessionsDeleteAllReq
	SessionsDeleteAllRsp = modeliamsession.SessionsDeleteAllRsp
)

// Admin session API aliases.
type (
	AdminSessionOwnerView  = modeliamsession.AdminSessionOwnerView
	AdminSessionsListReq   = modeliamsession.AdminSessionsListReq
	AdminSessionsListRsp   = modeliamsession.AdminSessionsListRsp
	AdminSessionsGetReq    = modeliamsession.AdminSessionsGetReq
	AdminSessionsGetRsp    = modeliamsession.AdminSessionsGetRsp
	AdminSessionsDeleteReq = modeliamsession.AdminSessionsDeleteReq
	AdminSessionsDeleteRsp = modeliamsession.AdminSessionsDeleteRsp

	AdminUserSessionsListReq   = modeliamsession.AdminUserSessionsListReq
	AdminUserSessionsListRsp   = modeliamsession.AdminUserSessionsListRsp
	AdminUserSessionsDeleteReq = modeliamsession.AdminUserSessionsDeleteReq
	AdminUserSessionsDeleteRsp = modeliamsession.AdminUserSessionsDeleteRsp
)
