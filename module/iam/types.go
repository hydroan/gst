package iam

import (
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
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
)

// User API aliases.
type (
	User               = modeliamuser.User
	UserStatus         = modeliamuser.UserStatus
	UserStatusPatchReq = modeliamuser.UserStatusPatchReq
	UserStatusPatchRsp = modeliamuser.UserStatusPatchRsp
	PasswordCredential = modeliamaccount.PasswordCredential
	EmailIdentity      = modeliamaccount.EmailIdentity
)

const (
	UserStatusActive   = modeliamuser.UserStatusActive
	UserStatusInactive = modeliamuser.UserStatusInactive
	UserStatusLocked   = modeliamuser.UserStatusLocked
)

// Profile API aliases.
type (
	Profile         = modeliamprofile.Profile
	ProfileGet      = modeliamprofile.ProfileGet
	ProfileGetReq   = modeliamprofile.ProfileGetReq
	ProfileGetRsp   = modeliamprofile.ProfileGetRsp
	ProfilePatch    = modeliamprofile.ProfilePatch
	ProfilePatchReq = modeliamprofile.ProfilePatchReq
	ProfilePatchRsp = modeliamprofile.ProfilePatchRsp
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

	SessionListReq      = modeliamsession.SessionListReq
	SessionListRsp      = modeliamsession.SessionListRsp
	SessionGetReq       = modeliamsession.SessionGetReq
	SessionGetRsp       = modeliamsession.SessionGetRsp
	SessionDeleteReq    = modeliamsession.SessionDeleteReq
	SessionDeleteRsp    = modeliamsession.SessionDeleteRsp
	SessionDeleteAllReq = modeliamsession.SessionDeleteAllReq
	SessionDeleteAllRsp = modeliamsession.SessionDeleteAllRsp
)

// Admin session API aliases.
type (
	AdminSessionOwnerView = modeliamsession.AdminSessionOwnerView
	AdminSessionList      = modeliamsession.AdminSessionList
	AdminSessionListReq   = modeliamsession.AdminSessionListReq
	AdminSessionListRsp   = modeliamsession.AdminSessionListRsp
	AdminSessionGet       = modeliamsession.AdminSessionGet
	AdminSessionGetReq    = modeliamsession.AdminSessionGetReq
	AdminSessionGetRsp    = modeliamsession.AdminSessionGetRsp
	AdminSessionDelete    = modeliamsession.AdminSessionDelete
	AdminSessionDeleteReq = modeliamsession.AdminSessionDeleteReq
	AdminSessionDeleteRsp = modeliamsession.AdminSessionDeleteRsp

	AdminUserSessionList      = modeliamsession.AdminUserSessionList
	AdminUserSessionListReq   = modeliamsession.AdminUserSessionListReq
	AdminUserSessionListRsp   = modeliamsession.AdminUserSessionListRsp
	AdminUserSessionDelete    = modeliamsession.AdminUserSessionDelete
	AdminUserSessionDeleteReq = modeliamsession.AdminUserSessionDeleteReq
	AdminUserSessionDeleteRsp = modeliamsession.AdminUserSessionDeleteRsp
)
