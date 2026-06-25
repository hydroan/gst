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

type ChangePasswordService struct {
	service.Base[*model.Empty, *modeliamaccount.ChangePasswordReq, *modeliamaccount.ChangePasswordRsp]
}

func (s *ChangePasswordService) Create(ctx *types.ServiceContext, req *modeliamaccount.ChangePasswordReq) (rsp *modeliamaccount.ChangePasswordRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("changepassword create")

	// Get current session
	sessionID, session, err := serviceiamsession.GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, errors.New("invalid session")
	}

	// Get user from database
	users := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).WithLimit(1).WithQuery(&modeliamuser.User{Username: session.Username}).List(&users); err != nil {
		log.Error("failed to query user", err)
		return nil, errors.New("database error")
	}
	if len(users) == 0 {
		log.Error("user not found", "username", session.Username)
		return nil, errors.New("user not found")
	}
	user := users[0]

	// Verify old password
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		log.Error("old password verification failed", "username", user.Username)
		return nil, errors.New("old password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Error("failed to hash new password", err)
		return nil, errors.New("failed to process new password")
	}

	// Update password in database
	user.PasswordHash = string(hashedPassword)
	user.MustChangePassword = false
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).Update(user); err != nil {
		log.Error("failed to update password", err)
		return nil, errors.New("failed to update password")
	}

	if syncErr := serviceiamsession.UpdateSessionMustChangePassword(ctx.Context(), sessionID, false); syncErr != nil {
		log.Error("failed to sync session after password change", syncErr)
		return nil, errors.Wrap(syncErr, "failed to refresh session")
	}

	log.Info("password changed successfully", "username", user.Username)
	return &modeliamaccount.ChangePasswordRsp{Msg: "password changed successfully"}, nil
}
