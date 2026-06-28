package serviceiamaccount

import (
	"net/http"

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
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("changepassword create")

	if err = validateChangePasswordInput(req); err != nil {
		return nil, err
	}

	// Get current session
	sessionID, session, err := serviceiamsession.SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, errors.New("invalid session")
	}

	// Get user from database
	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil {
		log.Error("failed to query user", err)
		return nil, errors.New("database error")
	}

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

	if err = serviceiamsession.DeleteUserSessionsExceptCurrent(ctx, user.GetID(), sessionID); err != nil {
		log.Error("failed to revoke other sessions after password change", err)
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to revoke other sessions", err)
	}

	// Update password in database
	user.PasswordHash = string(hashedPassword)
	user.MustChangePassword = false
	if err := database.Database[*modeliamuser.User](ctx).
		WithoutHook().
		WithSelect("username", "password_hash", "must_change_password").
		Update(user); err != nil {
		log.Error("failed to update password", err)
		return nil, errors.New("failed to update password")
	}

	if syncErr := serviceiamsession.UpdateSessionMustChangePassword(ctx, sessionID, false); syncErr != nil {
		log.Error("failed to sync session after password change", syncErr)
		return nil, errors.Wrap(syncErr, "failed to refresh session")
	}
	serviceiamsession.InvalidateUserStateCache(ctx, user.GetID())

	log.Info("password changed successfully", "username", user.Username)
	return &modeliamaccount.ChangePasswordRsp{Msg: "password changed successfully"}, nil
}
