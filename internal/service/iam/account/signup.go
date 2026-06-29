package serviceiamaccount

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

type SignupService struct {
	service.Base[*modeliamaccount.Signup, *modeliamaccount.SignupReq, *modeliamaccount.SignupRsp]
}

func (s *SignupService) Create(ctx *types.ServiceContext, req *modeliamaccount.SignupReq) (rsp *modeliamaccount.SignupRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	// Validate input
	if req.Username == "" {
		return nil, errors.New("username is required")
	}
	if req.Password == "" {
		return nil, errors.New("password is required")
	}
	if req.Password != req.RePassword {
		return nil, errors.New("passwords do not match")
	}
	if len(req.Password) < 6 {
		return nil, errors.New("password must be at least 6 characters long")
	}

	// Check if username already exists
	existingUsers := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).WithLimit(1).WithQuery(&modeliamuser.User{Username: req.Username}).List(&existingUsers); err != nil {
		log.Error("failed to check existing user", zap.Error(err))
		return nil, errors.New("failed to create user")
	}
	if len(existingUsers) > 0 {
		return nil, errors.New("username already exists")
	}

	// Create new user
	newUser := &modeliamuser.User{
		Username: req.Username,
	}

	// Save the user and password credential atomically.
	if err = database.Database[*modeliamuser.User](ctx).TransactionFunc(func(tx any) error {
		if createErr := database.Database[*modeliamuser.User](ctx).WithTx(tx).Create(newUser); createErr != nil {
			return createErr
		}

		passwordCredential, createErr := NewPasswordCredential(newUser.ID, req.Password, false)
		if createErr != nil {
			return createErr
		}
		if createErr = database.Database[*modeliamaccount.PasswordCredential](ctx).WithTx(tx).Create(passwordCredential); createErr != nil {
			return createErr
		}
		if req.Email == "" {
			return nil
		}

		emailIdentity, createErr := NewEmailIdentity(newUser.ID, req.Email)
		if createErr != nil {
			return createErr
		}
		return database.Database[*modeliamaccount.EmailIdentity](ctx).WithTx(tx).Create(emailIdentity)
	}); err != nil {
		log.Error("failed to create user", zap.Error(err))
		return nil, errors.New("failed to create user")
	}

	log.Info("user created successfully", zap.String("username", req.Username), zap.String("user_id", newUser.ID))

	return &modeliamaccount.SignupRsp{
		UserID:   newUser.ID,
		Username: newUser.Username,
		Message:  "User created successfully",
	}, nil
}
