package mfa

import (
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// iamUserAuthenticator adapts the framework IAM user model for the built-in MFA
// module. It lives under module/mfa so copied MFA service code does not import
// the framework IAM model or password hashing policy.
type iamUserAuthenticator struct{}

func (iamUserAuthenticator) AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*servicemfa.AuthenticatedUser, error) {
	users := make([]*modeliamuser.User, 0)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(&modeliamuser.User{Username: username}).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, servicemfa.ErrUserAuthenticationFailed
	}
	return authenticateIAMUser(users[0], password)
}

func (iamUserAuthenticator) AuthenticateByUserID(ctx *types.ServiceContext, userID, password string) (*servicemfa.AuthenticatedUser, error) {
	query := &modeliamuser.User{}
	query.ID = userID

	users := make([]*modeliamuser.User, 0)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(query).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, servicemfa.ErrUserAuthenticationFailed
	}
	return authenticateIAMUser(users[0], password)
}

func authenticateIAMUser(user *modeliamuser.User, password string) (*servicemfa.AuthenticatedUser, error) {
	if user == nil || user.Status != modeliamuser.UserStatusActive {
		return nil, servicemfa.ErrUserAuthenticationFailed
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, servicemfa.ErrUserAuthenticationFailed
	}
	return &servicemfa.AuthenticatedUser{
		ID:       user.ID,
		Username: user.Username,
	}, nil
}
