package serviceiamaccount

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/modulemodel/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/modulemodel/iam/user"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
)

type SignupService struct {
	service.Base[*modeliamaccount.Signup, *modeliamaccount.SignupReq, *modeliamaccount.SignupRsp]
}

func (s *SignupService) Create(ctx *gst.ServiceContext, req *modeliamaccount.SignupReq) (rsp *modeliamaccount.SignupRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	// Validate input
	if req.Username == "" {
		return nil, gst.NewError(http.StatusBadRequest, usernameRequiredMsg)
	}
	if req.Password == "" {
		return nil, gst.NewError(http.StatusBadRequest, passwordRequiredMsg)
	}
	if req.Password != req.RePassword {
		return nil, gst.NewError(http.StatusBadRequest, passwordsDoNotMatchMsg)
	}
	if len(req.Password) < 6 {
		return nil, gst.NewError(http.StatusBadRequest, passwordTooShortMsg)
	}

	// Check if username already exists
	existingUsers := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&existingUsers); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToCheckExistingUserMsg, err)
	}
	if len(existingUsers) > 0 {
		return nil, gst.NewError(http.StatusConflict, usernameAlreadyExistsMsg)
	}

	// Create new user
	newUser := &modeliamuser.User{
		Username: req.Username,
	}

	// Save the user and password credential atomically.
	if err = database.Transaction(ctx, func(ctx context.Context) error {
		if createErr := database.Database[*modeliamuser.User](ctx).Create(newUser); createErr != nil {
			return createErr
		}

		passwordCredential, createErr := NewPasswordCredential(ctx, newUser.ID, req.Password, false)
		if createErr != nil {
			return createErr
		}
		if createErr = database.Database[*modeliamaccount.PasswordCredential](ctx).Create(passwordCredential); createErr != nil {
			return createErr
		}
		if req.Email == "" {
			return nil
		}

		emailIdentity, createErr := NewEmailIdentity(newUser.ID, req.Email)
		if createErr != nil {
			return createErr
		}
		return database.Database[*modeliamaccount.EmailIdentity](ctx).Create(emailIdentity)
	}); err != nil {
		if errors.Is(err, database.ErrDuplicatedKey) {
			return nil, gst.NewErrorWithCause(http.StatusConflict, usernameAlreadyExistsMsg, err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToCreateUserMsg, err)
	}

	log.Infoz("user created successfully", zap.String("username", req.Username), zap.String("user_id", newUser.ID))

	return &modeliamaccount.SignupRsp{
		UserID:   newUser.ID,
		Username: newUser.Username,
		Message:  "User created successfully",
	}, nil
}
