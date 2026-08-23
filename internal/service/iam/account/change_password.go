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
	sessionID, currentSession, err := serviceiamsession.CurrentSession(ctx)
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

	// The new password is written before anything is revoked. Revoking first
	// would log the user out of their other devices for a password change that
	// the write below could still fail to make, leaving them with the old
	// password and none of the sessions they had.
	if err = database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithoutHook().
		WithSelect(colUserID, colPasswordHash, colMustChangePassword, colPasswordChangedAt).
		Update(credential); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update password", err)
	}

	// Dropping the cache is the whole of the sync for the session that stays.
	// Its snapshot still says a password change is required, but no reader
	// trusts that copy: authentication overwrites it from this cache on every
	// request, and the next request will miss and read the cleared flag straight
	// from the row written above.
	serviceiamsession.Store.DropUserState(ctx, currentUser.GetID())

	// Revoking last means a failure here reports a password that did change and
	// devices that may not have been logged out — the honest answer, and the one
	// a caller can act on by asking again. The reverse order could only report
	// the same failure for a password that had not changed at all.
	if err = serviceiamsession.Store.DeleteUserSessionsExcept(ctx, currentUser.GetID(), sessionID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to revoke other sessions", err)
	}

	log.Info("password changed successfully", "username", currentUser.Username)
	return &modeliamaccount.ChangePasswordRsp{Msg: "password changed successfully"}, nil
}
