package mfa

import (
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// iamAccountAuthenticator adapts the framework IAM user model for the built-in MFA
// module. It lives under module/mfa so copied MFA service code does not import
// the framework IAM model or password hashing policy.
type iamAccountAuthenticator struct{}

func (iamAccountAuthenticator) AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*servicemfa.AuthenticatedAccount, error) {
	users := make([]*modeliamuser.User, 0)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(&modeliamuser.User{Username: username}).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, servicemfa.ErrAccountAuthenticationFailed
	}
	return authenticateIAMAccount(users[0], password)
}

func (iamAccountAuthenticator) AuthenticateByAccountID(ctx *types.ServiceContext, accountID, password string) (*servicemfa.AuthenticatedAccount, error) {
	query := &modeliamuser.User{}
	query.ID = accountID

	users := make([]*modeliamuser.User, 0)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(query).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, servicemfa.ErrAccountAuthenticationFailed
	}
	return authenticateIAMAccount(users[0], password)
}

func authenticateIAMAccount(user *modeliamuser.User, password string) (*servicemfa.AuthenticatedAccount, error) {
	if user == nil || user.Status != modeliamuser.UserStatusActive {
		return nil, servicemfa.ErrAccountAuthenticationFailed
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, servicemfa.ErrAccountAuthenticationFailed
	}
	return &servicemfa.AuthenticatedAccount{
		ID:       user.ID,
		Username: user.Username,
	}, nil
}
