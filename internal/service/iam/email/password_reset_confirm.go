package serviceiamemail

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamemail "github.com/hydroan/gst/internal/model/iam/email"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// PasswordResetConfirmService handles the token confirmation step that finalizes
// the email-driven password reset flow.
type PasswordResetConfirmService struct {
	service.Base[*modeliamemail.PasswordResetConfirm, *modeliamemail.PasswordResetConfirmReq, *modeliamemail.PasswordResetConfirmRsp]
}

var (
	// passwordResetLoadUserByID loads the account referenced by the password reset token.
	passwordResetLoadUserByID = func(ctx *types.ServiceContext, userID string) (*modeliamuser.User, error) {
		user := new(modeliamuser.User)
		if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(user, userID); err != nil {
			return nil, err
		}
		return user, nil
	}
	// passwordResetUpdateUser persists the new password state while skipping hooks that
	// are unrelated to the reset flow and selecting only the fields changed here.
	passwordResetUpdateUser = func(ctx *types.ServiceContext, user *modeliamuser.User) error {
		return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
			WithoutHook().
			WithSelect("username", "password_hash", "must_change_password").
			Update(user)
	}
	// passwordResetInvalidateSessions clears the cached user-session index so a
	// password reset immediately revokes access granted by previously issued sessions.
	passwordResetInvalidateSessions = serviceiamsession.InvalidateUserSessions
)

// Create completes the password reset flow by consuming the one-time token,
// updating the stored password hash, and invalidating active sessions.
func (s *PasswordResetConfirmService) Create(ctx *types.ServiceContext, req *modeliamemail.PasswordResetConfirmReq) (rsp *modeliamemail.PasswordResetConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(passwordResetContext(ctx), iamEmailFlowKindPasswordReset, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modeliamemail.PasswordResetConfirmRsp{
				Reset: false,
				Msg:   "invalid or expired password reset token",
			}, nil
		}
		log.Error("failed to consume password reset flow", err)
		return nil, errors.Wrap(err, "failed to consume password reset flow")
	}
	if strings.TrimSpace(flow.UserID) == "" {
		return nil, errors.New("password reset user id is required")
	}

	user, err := passwordResetLoadUserByID(ctx, flow.UserID)
	if err != nil {
		log.Error("failed to load password reset user", err)
		return nil, errors.Wrap(err, "failed to load password reset user")
	}
	if normalizePasswordResetEmail(user.Email) != normalizeEmailScope(flow.Email) {
		return &modeliamemail.PasswordResetConfirmRsp{
			Reset: false,
			Msg:   "invalid or expired password reset token",
		}, nil
	}

	if err = applyPasswordReset(user, req.NewPassword); err != nil {
		log.Error("failed to apply password reset", err)
		return nil, err
	}
	if err = passwordResetUpdateUser(ctx, user); err != nil {
		log.Error("failed to update password reset user", err)
		return nil, errors.Wrap(err, "failed to update password")
	}

	passwordResetInvalidateSessions(user.ID)
	return &modeliamemail.PasswordResetConfirmRsp{
		Reset: true,
		Msg:   "password reset successfully",
	}, nil
}

// applyPasswordReset hashes the supplied password and updates the in-memory user
// model before persistence.
func applyPasswordReset(user *modeliamuser.User, newPassword string) error {
	if user == nil {
		return errors.New("password reset user is required")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "failed to process new password")
	}
	user.PasswordHash = string(hashedPassword)
	user.MustChangePassword = false
	return nil
}
