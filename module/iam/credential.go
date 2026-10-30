package iam

import (
	"context"

	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
)

// NewPasswordCredential returns the password credential of the user userID,
// password hashed the way the module stores and verifies one; created with
// mustChangePassword, the credential asks the user to replace the password at
// the first login. It is how a project writes a baseline account, the
// built-in root subject among them: the user row and the credential this
// returns go through the database chain in a router.OnRoutesReady hook, which
// runs once the database is reachable and before the server serves. RBAC
// grants the system root role to the user whose id is rbac.UserRoot
// at initialization, but nothing creates that user. The hook runs at every
// start, so it looks the account up before writing it:
//
//	router.OnRoutesReady(func(ctx context.Context, _ map[string][]string) error {
//		if err := database.Database[*iam.User](ctx).Get(new(iam.User), rbac.UserRoot); err == nil {
//			return nil
//		} else if !errors.Is(err, database.ErrRecordNotFound) {
//			return err
//		}
//		root := &iam.User{Username: rbac.UserRoot, Status: iam.UserStatusActive}
//		root.ID = rbac.UserRoot
//		credential, err := iam.NewPasswordCredential(ctx, root.ID, "change-me", true)
//		if err != nil {
//			return err
//		}
//		return database.Transaction(ctx, func(ctx context.Context) error {
//			if err := database.Database[*iam.User](ctx).Create(root); err != nil {
//				return err
//			}
//			return database.Database[*iam.PasswordCredential](ctx).Create(credential)
//		})
//	})
func NewPasswordCredential(ctx context.Context, userID, password string, mustChangePassword bool) (*PasswordCredential, error) {
	return serviceiamaccount.NewPasswordCredential(ctx, userID, password, mustChangePassword)
}
