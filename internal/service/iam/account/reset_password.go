package serviceiamaccount

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type ResetPasswordService struct {
	service.Base[*modeliamaccount.ResetPassword, *modeliamaccount.ResetPasswordReq, *modeliamaccount.ResetPasswordRsp]
}

func (r *ResetPasswordService) Create(ctx *types.ServiceContext, req *modeliamaccount.ResetPasswordReq) (rsp *modeliamaccount.ResetPasswordRsp, err error) {
	log := r.WithContext(ctx, ctx.Phase())
	log.Info("resetpassword create")

	if err = validateResetPasswordInput(req); err != nil {
		return nil, err
	}

	actor, target, err := LoadActorAndTarget(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	if err = adminauth.EnsureTenantAdmin(ctx, actor, target); err != nil {
		return nil, err
	}

	credential, err := LoadPasswordCredential(ctx, target.ID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load password credential", err)
		}
		credential = &modeliamaccount.PasswordCredential{UserID: target.ID}
	}
	if err = ApplyPasswordCredentialUpdate(ctx, credential, req.NewPassword, true); err != nil {
		return nil, err
	}
	if credential.ID == "" {
		if err = database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); err != nil {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update password", err)
		}
	} else {
		if err = database.Database[*modeliamaccount.PasswordCredential](ctx).
			WithoutHook().
			WithSelect(colUserID, colPasswordHash, colMustChangePassword, colPasswordChangedAt).
			Update(credential); err != nil {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to update password", err)
		}
	}

	if err = serviceiamsession.Store.DeleteUserSessions(ctx, req.UserID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to revoke user sessions", err)
	}

	log.Info("password reset successfully", "target_user_id", req.UserID, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamaccount.ResetPasswordRsp{Msg: "password reset successfully"}, nil
}
