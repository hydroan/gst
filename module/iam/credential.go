package iam

import (
	"context"

	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
)

// NewPasswordCredential returns the password credential of the user userID,
// password hashed the way the module stores and verifies one; created with
// mustChangePassword, the credential asks the user to replace the password at
// the first login. CreateAccount writes an account with one; this is for a
// credential written on its own, to a user row that exists.
func NewPasswordCredential(ctx context.Context, userID, password string, mustChangePassword bool) (*PasswordCredential, error) {
	return serviceiamaccount.NewPasswordCredential(ctx, userID, password, mustChangePassword)
}

// CreateAccount writes user and its password credential, the two rows an
// account that logs in with a password is made of, in one transaction: its
// own, or the one ctx already carries. user is written as given, so an id set
// beforehand stays and an empty one is generated on insert; password is
// hashed the way the module stores and verifies one, and with
// mustChangePassword the first login asks for a replacement.
//
// It is how a project writes its baseline accounts: in a router.OnRoutesReady
// hook, which runs once the database is reachable and before the server
// serves, at every start, so the hook looks an account up before writing it.
// The built-in root account needs no hook of the project's: Register writes
// it from IAM_ROOT_PASSWORD at the first start.
//
//	router.OnRoutesReady(func(ctx context.Context, _ map[string][]string) error {
//		if err := database.Database[*iam.User](ctx).Get(new(iam.User), "auditor"); err == nil {
//			return nil
//		} else if !errors.Is(err, database.ErrRecordNotFound) {
//			return err
//		}
//		auditor := &iam.User{Username: "auditor", Status: iam.UserStatusActive}
//		auditor.ID = "auditor"
//		return iam.CreateAccount(ctx, auditor, "change-me", true)
//	})
func CreateAccount(ctx context.Context, user *User, password string, mustChangePassword bool) error {
	return serviceiamaccount.CreateAccount(ctx, user, password, mustChangePassword)
}
