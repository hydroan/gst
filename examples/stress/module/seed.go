package module

import (
	"context"
	"net/http"

	"stress/configx"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/tenant"
)

// The accounts the scenarios log in as beside root, which the iam module seeds
// from IAM_ROOT_PASSWORD; both get the password [seed] password names. reader
// holds the reader role alone, which selects the read routes of the records
// and the orders: the authorization path of an ordinary session. member
// belongs to the tenant memberTenant, holding the ticket reader role there:
// the path of a session acting in a tenant, whose rows the tenant scope
// filters.
const (
	readerUsername = "reader"
	memberUsername = "member"
	memberTenant   = "t-03"
	menuRead       = "read"
	menuTickets    = "tickets"
)

// seed creates the accounts, their menus and roles once, at the first start
// against an empty database.
func seed(ctx context.Context, _ map[string][]string) error {
	password := config.Get[configx.Seed]().Password
	if password == "" {
		return errors.Newf("the seeded accounts need a password: set %s or seed.password", configx.SEED_PASSWORD)
	}
	err := database.Database[*iam.User](ctx).Get(new(iam.User), readerUsername)
	if err == nil {
		return nil
	}
	if !errors.Is(err, database.ErrRecordNotFound) {
		return err
	}
	return database.Transaction(ctx, func(ctx context.Context) error {
		reader := &iam.User{Username: readerUsername, Status: iam.UserStatusActive}
		reader.ID = readerUsername
		if err := iam.CreateAccount(ctx, reader, password, false); err != nil {
			return err
		}
		member := &iam.User{Username: memberUsername, Status: iam.UserStatusActive}
		member.ID = memberUsername
		if err := iam.CreateAccount(ctx, member, password, false); err != nil {
			return err
		}

		// The menu catalog is global; a role selects menus and is written in
		// the tenant it grants them in.
		for _, m := range []*authz.Menu{
			{ID: menuRead, ParentID: "root", Label: "Read", Path: "/read", Routes: []authz.Route{
				{Path: "/api/records", Methods: []string{http.MethodGet}},
				{Path: "/api/records/{id}", Methods: []string{http.MethodGet}},
				{Path: "/api/orders", Methods: []string{http.MethodGet}},
				{Path: "/api/orders/{id}", Methods: []string{http.MethodGet}},
			}},
			{ID: menuTickets, ParentID: "root", Label: "Tickets", Path: "/tickets", Routes: []authz.Route{
				{Path: "/api/tickets", Methods: []string{http.MethodGet}},
				{Path: "/api/tickets/{id}", Methods: []string{http.MethodGet}},
			}},
		} {
			if err := database.Database[*authz.Menu](ctx).Create(m); err != nil {
				return err
			}
		}
		role := &authz.Role{Name: "reader", MenuIDs: []string{menuRead}}
		if err := database.Database[*authz.Role](ctx).Create(role); err != nil {
			return err
		}
		if err := database.Database[*authz.RoleBinding](ctx).Create(&authz.RoleBinding{SubjectID: reader.ID, RoleID: role.ID}); err != nil {
			return err
		}
		ticketReader := &authz.Role{Name: "ticket_reader", MenuIDs: []string{menuTickets}}
		if err := database.Database[*authz.Role](tenant.In(ctx, memberTenant)).Create(ticketReader); err != nil {
			return err
		}
		return database.Database[*authz.RoleBinding](tenant.In(ctx, memberTenant)).Create(&authz.RoleBinding{SubjectID: member.ID, RoleID: ticketReader.ID})
	})
}
