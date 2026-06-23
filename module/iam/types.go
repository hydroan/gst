package iam

import (
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// account
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

	User = modeliamuser.User

	Session     = modeliamsession.Session
	SessionView = modeliamsession.SessionView
	Token       = modeliamsession.Token
	Heartbeat   = modeliamsession.Heartbeat
	OnlineUser  = modeliamsession.OnlineUser

	CurrentListReq   = modeliamsession.CurrentListReq
	CurrentListRsp   = modeliamsession.CurrentListRsp
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

	AdminSessionUserView   = modeliamsession.AdminSessionUserView
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
