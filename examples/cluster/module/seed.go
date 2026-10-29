package module

import (
	"context"

	"cluster/configx"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/module/iam"
)

// seedRoot writes the built-in root account the first time the deployment
// starts: the user whose id is consts.AUTHZ_USER_ROOT, which RBAC grants
// the system root role to, and its credential, the password the [seed]
// section names. The hook runs on every start, under the startup lock the
// replicas take turns on, so a replica finding the account leaves it; a
// deployment naming no password does not start, since nothing could
// administer it.
func seedRoot(ctx context.Context, _ map[string][]string) error {
	password := config.Get[configx.Seed]().RootPassword
	if password == "" {
		return errors.Newf("the root account needs a password: set %s or seed.root_password", configx.SEED_ROOT_PASSWORD)
	}
	err := database.Database[*iam.User](ctx).Get(new(iam.User), consts.AUTHZ_USER_ROOT)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrRecordNotFound) {
		return err
	}
	root := &iam.User{Username: consts.AUTHZ_USER_ROOT, Status: iam.UserStatusActive}
	root.ID = consts.AUTHZ_USER_ROOT
	credential, err := iam.NewPasswordCredential(ctx, root.ID, password, false)
	if err != nil {
		return err
	}
	return database.Transaction(ctx, func(ctx context.Context) error {
		if err := database.Database[*iam.User](ctx).Create(root); err != nil {
			return err
		}
		return database.Database[*iam.PasswordCredential](ctx).Create(credential)
	})
}
