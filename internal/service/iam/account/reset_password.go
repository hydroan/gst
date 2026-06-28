package serviceiamaccount

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	serviceiamuser "github.com/hydroan/gst/internal/service/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type ResetPasswordService struct {
	service.Base[*model.Empty, *modeliamaccount.ResetPasswordReq, *modeliamaccount.ResetPasswordRsp]
}

func (s *ResetPasswordService) Create(ctx *types.ServiceContext, req *modeliamaccount.ResetPasswordReq) (rsp *modeliamaccount.ResetPasswordRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("resetpassword create")

	if err = validateResetPasswordInput(req); err != nil {
		return nil, err
	}

	actor, target, err := serviceiamuser.LoadPrivilegedActorAndTarget(ctx, req.UserID)
	if err != nil {
		log.Error("failed to resolve actor or target user", err)
		return nil, err
	}

	if err = serviceiamuser.MayManageProtectedUser(actor, target); err != nil {
		log.Error("reset password denied", err)
		return nil, err
	}

	credential, err := LoadPasswordCredential(ctx, target.ID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			log.Error("failed to query password credential", err)
			return nil, errors.Wrap(err, "failed to update password")
		}
		credential = &modeliamaccount.PasswordCredential{UserID: target.ID}
	}
	if err = ApplyPasswordCredentialUpdate(credential, req.NewPassword, true); err != nil {
		log.Error("failed to hash new password", err)
		return nil, errors.Wrap(err, "failed to process new password")
	}
	if credential.ID == "" {
		if err = database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); err != nil {
			log.Error("failed to create password credential", err)
			return nil, errors.Wrap(err, "failed to update password")
		}
	} else {
		if err = database.Database[*modeliamaccount.PasswordCredential](ctx).
			WithoutHook().
			WithSelect("user_id", "password_hash", "must_change_password", "password_changed_at").
			Update(credential); err != nil {
			log.Error("failed to update password credential", err)
			return nil, errors.Wrap(err, "failed to update password")
		}
	}

	if err = serviceiamsession.DeleteUserSessions(ctx, req.UserID); err != nil {
		log.Error("failed to revoke user sessions after password reset", err)
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to revoke user sessions", err)
	}

	log.Info("password reset successfully", "target_user_id", req.UserID, "actor_user_id", actor.GetID(), "actor_username", actor.Username)
	return &modeliamaccount.ResetPasswordRsp{Msg: "password reset successfully"}, nil
}
