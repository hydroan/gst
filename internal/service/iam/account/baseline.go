package serviceiamaccount

import (
	"context"
	"net/http"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// rootPasswordEnv is the environment variable SeedRoot reads the root
// account's password from.
const rootPasswordEnv = "IAM_ROOT_PASSWORD"

// CreateAccount writes newUser and its password credential, the two rows an
// account that logs in with a password is made of, in one transaction: its
// own, or the one ctx already carries. newUser is written as given, so an id
// set beforehand stays and an empty one is generated on insert; password is
// hashed the way the module stores and verifies one, and with
// mustChangePassword the first login asks for a replacement.
func CreateAccount(ctx context.Context, newUser *modeliamuser.User, password string, mustChangePassword bool) error {
	if newUser == nil {
		return gst.NewError(http.StatusBadRequest, "user is required")
	}
	return database.Transaction(ctx, func(ctx context.Context) error {
		if err := database.Database[*modeliamuser.User](ctx).Create(newUser); err != nil {
			return err
		}
		credential, err := NewPasswordCredential(ctx, newUser.ID, password, mustChangePassword)
		if err != nil {
			return err
		}
		return database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential)
	})
}

// SeedRoot is the startup hook that writes the built-in root account, the
// user whose id is rbac.UserRoot, which RBAC grants the system root role to,
// from the environment: IAM_ROOT_PASSWORD is the password, held to the rule a
// signup's password is held to, and an unset or empty variable writes
// nothing. The hook runs at every start and leaves the account alone once it
// exists, the password included: a changed variable rotates nothing, a
// password is changed through the module's routes. Register installs the
// hook; a project that copied the module installs it in assembly code of its
// own, router.OnRoutesReady(SeedRoot).
func SeedRoot(ctx context.Context, _ map[string][]string) error {
	password := os.Getenv(rootPasswordEnv)
	if password == "" {
		return nil
	}
	if err := validateNewAccountPassword(password); err != nil {
		return errors.Wrap(err, rootPasswordEnv)
	}
	err := database.Database[*modeliamuser.User](ctx).Get(new(modeliamuser.User), rbac.UserRoot)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrRecordNotFound) {
		return err
	}
	root := &modeliamuser.User{Username: rbac.UserRoot, Status: modeliamuser.UserStatusActive}
	root.ID = rbac.UserRoot
	return CreateAccount(ctx, root, password, false)
}
