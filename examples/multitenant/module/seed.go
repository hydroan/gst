package module

import (
	"context"
	"net/http"

	"multitenant/configx"
	"multitenant/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/tenant"
)

// The tenants the deployment starts with, and the menus of the global
// catalog their roles select from.
const (
	tenantAlpha = "alpha"
	tenantBeta  = "beta"

	menuTickets       = "tickets"
	menuTicketsManage = "tickets_manage"
	menuDigests       = "digests"
)

// seed writes the baseline the scenarios start from, the first time the
// process starts with an empty database: the tenant registry, the accounts,
// the menu catalog, each tenant's roles and the bindings that make an
// account a member of a tenant. It runs in a startup hook, after the
// framework has initialized the authorization and before the server serves,
// and finds the root account on every later start, so it writes once.
//
// What the hook acts in is what a job acts in: no request decided a tenant,
// so the context acts in the default tenant until tenant.In names another.
// The registry, the accounts and the menus are global and written as they
// are; a tenant's roles and bindings are written on a context acting in that
// tenant, and the stamp files them under it.
func seed(ctx context.Context, _ map[string][]string) error {
	err := database.Database[*iam.User](ctx).Get(new(iam.User), consts.AUTHZ_USER_ROOT)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrRecordNotFound) {
		return err
	}
	password := config.Get[configx.Seed]().Password
	return database.Transaction(ctx, func(ctx context.Context) error {
		tenants := []struct{ id, name string }{{tenantAlpha, "Alpha"}, {tenantBeta, "Beta"}}
		for _, t := range tenants {
			row := &model.Tenant{Name: t.name}
			row.ID = t.id
			if err := database.Database[*model.Tenant](ctx).Create(row); err != nil {
				return err
			}
		}

		// The accounts: root is the built-in system root, by its id; the
		// others are ordinary accounts whose memberships the bindings below
		// decide. Alice administers alpha, carol beta, dave belongs to both.
		if _, err := seedAccount(ctx, consts.AUTHZ_USER_ROOT, consts.AUTHZ_USER_ROOT, password); err != nil {
			return err
		}
		ids := make(map[string]string, 4)
		for _, username := range []string{"alice", "bob", "carol", "dave"} {
			id, err := seedAccount(ctx, "", username, password)
			if err != nil {
				return err
			}
			ids[username] = id
		}

		// The menu catalog is global: a menu names the routes it grants, and
		// a role selects menus. The hook runs with no subject behind it,
		// which the catalog's write guard treats as the deployment's own
		// hand.
		for _, m := range []*authz.Menu{
			{ID: menuTickets, ParentID: "root", Label: "Tickets", Path: "/tickets", Routes: []authz.Route{
				{Path: "/api/tickets", Methods: []string{http.MethodGet}},
				{Path: "/api/tickets/{id}", Methods: []string{http.MethodGet}},
			}},
			{ID: menuTicketsManage, ParentID: "root", Label: "Manage tickets", Path: "/tickets/manage", Routes: []authz.Route{
				{Path: "/api/tickets", Methods: []string{http.MethodPost}},
				{Path: "/api/tickets/{id}", Methods: []string{http.MethodPut, http.MethodPatch, http.MethodDelete}},
			}},
			{ID: menuDigests, ParentID: "root", Label: "Digests", Path: "/digests", Routes: []authz.Route{
				{Path: "/api/digests", Methods: []string{http.MethodGet}},
				{Path: "/api/digests/{id}", Methods: []string{http.MethodGet}},
			}},
		} {
			if err := database.Database[*authz.Menu](ctx).Create(m); err != nil {
				return err
			}
		}

		// Each tenant's roles, written in the tenant: the role's hooks derive
		// its permissions from the menus it selects, in the tenant the row
		// carries. The built-in administrator role is nobody's to create; a
		// binding names it by its constant id.
		alpha := tenant.In(ctx, tenantAlpha)
		beta := tenant.In(ctx, tenantBeta)
		agentAlpha, err := seedRole(alpha, "agent", menuTickets, menuDigests)
		if err != nil {
			return err
		}
		managerAlpha, err := seedRole(alpha, "manager", menuTickets, menuTicketsManage, menuDigests)
		if err != nil {
			return err
		}
		agentBeta, err := seedRole(beta, "agent", menuTickets, menuDigests)
		if err != nil {
			return err
		}
		for _, b := range []struct {
			ctx     context.Context
			subject string
			role    string
		}{
			{alpha, ids["alice"], consts.AUTHZ_ROLE_ADMIN},
			{alpha, ids["bob"], agentAlpha},
			{alpha, ids["dave"], managerAlpha},
			{beta, ids["carol"], consts.AUTHZ_ROLE_ADMIN},
			{beta, ids["dave"], agentBeta},
		} {
			if err := database.Database[*authz.RoleBinding](b.ctx).Create(&authz.RoleBinding{SubjectID: b.subject, RoleID: b.role}); err != nil {
				return err
			}
		}

		// What every authenticated subject may reach, in every tenant and
		// holding no role at all: the registry and the caller's own
		// memberships, which a client needs before it can choose a tenant,
		// and the menu list, which answers each subject the menus its own
		// roles select, so that granting it hands nothing out.
		return rbac.RBAC().SetPermissionsForAuthenticated(ctx, []gst.Permission{
			{Object: "/api/tenants", Action: http.MethodGet},
			{Object: "/api/tenants/{id}", Action: http.MethodGet},
			{Object: "/api/memberships", Action: http.MethodGet},
			{Object: "/api/authz/menus", Action: http.MethodGet},
		})
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

// seedRole writes a role selecting the menus given, in the tenant ctx acts
// in, and returns its id, which the bindings name.
func seedRole(ctx context.Context, name string, menuIDs ...string) (string, error) {
	role := &authz.Role{Name: name, MenuIDs: menuIDs}
	if err := database.Database[*authz.Role](ctx).Create(role); err != nil {
		return "", err
	}
	return role.ID, nil
}
