package serviceiamaccount

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

type ResetPasswordService struct {
	service.Base[*model.Empty, *modeliamaccount.ResetPasswordReq, *modeliamaccount.ResetPasswordRsp]
}

func (s *ResetPasswordService) Create(ctx *types.ServiceContext, req *modeliamaccount.ResetPasswordReq) (rsp *modeliamaccount.ResetPasswordRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("resetpassword create")

	actorUsername, actor, target, err := loadPrivilegedActorAndTarget(ctx, req.UserID)
	if err != nil {
		log.Error("failed to resolve actor or target user", err)
		return nil, err
	}

	if err = mayManageProtectedUser(actorUsername, actor, target); err != nil {
		log.Error("reset password denied", err)
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Error("failed to hash new password", err)
		return nil, errors.Wrap(err, "failed to process new password")
	}

	target.PasswordHash = string(hashedPassword)
	target.MustChangePassword = true
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithoutHook().
		WithSelect("username", "password_hash", "must_change_password").
		Update(target); err != nil {
		log.Error("failed to update user password fields", err)
		return nil, errors.Wrap(err, "failed to update password")
	}

	serviceiamsession.InvalidateUserSessions(req.UserID)

	log.Info("password reset successfully", "target_user_id", req.UserID, "actor", actorUsername)
	return &modeliamaccount.ResetPasswordRsp{Msg: "password reset successfully"}, nil
}
