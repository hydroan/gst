package module

import (
	"context"
	"net/http"

	"stress/configx"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
)

// The accounts the scenarios log in as, both with the password [seed]
// root_password names: root holds the system role and reaches every route;
// reader holds the reader role alone, which selects the read routes of the
// records and the orders, the authorization path of an ordinary session.
const (
	readerUsername = "reader"
	menuRead       = "read"
)

// seed creates the accounts and the role once, at the first start against
// an empty database.
func seed(ctx context.Context, _ map[string][]string) error {
	password := config.Get[configx.Seed]().RootPassword
	if password == "" {
		return errors.Newf("the accounts need a password: set %s or seed.root_password", configx.SEED_ROOT_PASSWORD)
	}
	err := database.Database[*iam.User](ctx).Get(new(iam.User), rbac.UserRoot)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrRecordNotFound) {
		return err
	}
	return database.Transaction(ctx, func(ctx context.Context) error {
		if _, err := seedAccount(ctx, rbac.UserRoot, rbac.UserRoot, password); err != nil {
			return err
		}
		readerID, err := seedAccount(ctx, "", readerUsername, password)
		if err != nil {
			return err
		}
		menu := &authz.Menu{ID: menuRead, ParentID: "root", Label: "Read", Path: "/read", Routes: []authz.Route{
			{Path: "/api/records", Methods: []string{http.MethodGet}},
			{Path: "/api/records/{id}", Methods: []string{http.MethodGet}},
			{Path: "/api/orders", Methods: []string{http.MethodGet}},
			{Path: "/api/orders/{id}", Methods: []string{http.MethodGet}},
		}}
		if err := database.Database[*authz.Menu](ctx).Create(menu); err != nil {
			return err
		}
		role := &authz.Role{Name: "reader", MenuIDs: []string{menuRead}}
		if err := database.Database[*authz.Role](ctx).Create(role); err != nil {
			return err
		}
		return database.Database[*authz.RoleBinding](ctx).Create(&authz.RoleBinding{SubjectID: readerID, RoleID: role.ID})
	})
}

// seedAccount writes an account with its password credential, under id when
// one is given, and returns the id it was written with.
func seedAccount(ctx context.Context, id, username, password string) (string, error) {
	user := &iam.User{Username: username, Status: iam.UserStatusActive}
	user.ID = id
	if err := database.Database[*iam.User](ctx).Create(user); err != nil {
		return "", err
	}
	credential, err := iam.NewPasswordCredential(ctx, user.ID, password, false)
	if err != nil {
		return "", err
	}
	if err := database.Database[*iam.PasswordCredential](ctx).Create(credential); err != nil {
		return "", err
	}
	return user.ID, nil
}
