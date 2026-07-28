package serviceiamaccount

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type ChangePasswordService struct {
	service.Base[*modeliamaccount.ChangePassword, *modeliamaccount.ChangePasswordReq, *modeliamaccount.ChangePasswordRsp]
}

func (c *ChangePasswordService) Create(ctx *types.ServiceContext, req *modeliamaccount.ChangePasswordReq) (rsp *modeliamaccount.ChangePasswordRsp, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("changepassword create")

	if err = validateChangePasswordInput(req); err != nil {
		return nil, err
	}

	// Get current session
	sessionID, currentSession, err := serviceiamsession.SessionManager.Current(ctx)
	if err != nil {
		return nil, err
	}

	// Get user from database
	currentUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(currentUser, currentSession.UserID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to query user", err)
	}

	credential, err := LoadPasswordCredential(ctx, currentUser.ID)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load password credential", err)
	}

	// Verify old password
	if err = VerifyPasswordCredential(ctx, credential, req.OldPassword); err != nil {
		return nil, service.NewError(http.StatusBadRequest, "old password is incorrect")
	}

	if err = ApplyPasswordCredentialUpdate(ctx, credential, req.NewPassword, false); err != nil {
		return nil, err
	}

	if err = serviceiamsession.DeleteUserSessionsExceptCurrent(ctx, currentUser.GetID(), sessionID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to revoke other sessions", err)
	}

	// Update password in database
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithoutHook().
		WithSelect("user_id", "password_hash", "must_change_password", "password_changed_at").
		Update(credential); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update password", err)
	}

	serviceiamsession.InvalidateUserStateCache(ctx, currentUser.GetID())
	if syncErr := serviceiamsession.UpdateSessionMustChangePassword(ctx, sessionID, false); syncErr != nil {
		log.Warn("failed to sync session after password change", syncErr)
	}

	log.Info("password changed successfully", "username", currentUser.Username)
	return &modeliamaccount.ChangePasswordRsp{Msg: "password changed successfully"}, nil
}
