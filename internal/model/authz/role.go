package modelauthz

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap/zapcore"
	"gorm.io/datatypes"
)

type Role struct {
	tenant.Scope

	// Name is the human-readable role name, unique within a tenant. Renaming
	// is free: policy rows and role bindings reference the immutable ID,
	// never the name.
	Name string `json:"name,omitempty" query:"name" gorm:"size:191"`

	Default *bool `json:"default,omitempty" query:"default"`

	// MenuIDs grants backend route permissions through each selected menu's
	// Routes, and is the only source of frontend menu visibility.
	MenuIDs datatypes.JSONSlice[string] `json:"menu_ids,omitempty"`

	Menus []*Menu `json:"menus,omitempty" gorm:"-"`

	// Query opts the role list into paging and ordering, which an
	// administration screen listing roles needs and which Menu beside it
	// already has.
	//
	// The two arrive together on purpose. A page is only meaningful over an
	// ordered set: without an order the database is free to return rows in any
	// order it likes, and a second page drawn from a different one repeats rows
	// it already showed and skips rows it never did. Paging alone would have
	// made that failure reachable through the very parameter added to fix it.
	model.Query
	model.Base
}

func (Role) Design() {
	dsl.Migrate()
	dsl.Route("authz/roles", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {})
		dsl.Update(func() {})
		dsl.Patch(func() {})
		dsl.List(func() {})
		dsl.Get(func() {})
	})
}

func (r *Role) Purge() bool { return true }

// Indexes declares the uniqueness of a role name inside its tenant.
//
// It moved off the struct tags because the tenant column now arrives through an
// embedded struct, and a tag on an embedded field cannot name a field beside it.
// The columns and the uniqueness are what they were; only the generated name
// differs, which is the framework's to choose.
func (Role) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"TenantID", "Name"}, Unique: true}}
}

func (r *Role) tenant() string {
	if r != nil && len(r.TenantID) > 0 {
		return string(r.TenantID)
	}
	return tenant.Default
}

func (r *Role) validate() error {
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	if len(r.Name) == 0 {
		return errors.New("role name is required")
	}

	// The system role is addressed by its constant ID and never lives in the
	// roles table; reject both the ID and the name to avoid a user-created
	// role masquerading as it.
	if r.ID == consts.AUTHZ_SYSTEM_ROLE_ROOT || r.Name == consts.AUTHZ_SYSTEM_ROLE_ROOT {
		return errors.New("system_root is reserved for the system role")
	}

	// Both checks below are on the ID alone, because the ID is what authorization
	// reads: a role binding stores Role.ID as the policy role, and syncPermissions
	// writes it into the role column of every policy it generates. The name is
	// display text no matcher ever compares, so reserving it would forbid an
	// ordinary label without closing anything.
	//
	// The matcher grants the built-in admin role unconditional access to every
	// object and action in its tenant without consulting a single policy. A role
	// created under this ID hands tenant-wide superuser access to everyone bound
	// to it, whatever permissions it appears to select.
	if r.ID == consts.AUTHZ_ROLE_ADMIN {
		return errors.New("admin is reserved for the built-in tenant administrator role")
	}

	// Policies written for the authenticated role are matched without a role
	// membership or tenant check, so they apply to every authenticated subject. A
	// role created under this ID turns its own permissions into global ones: every
	// policy syncPermissions generates for it would allow every subject that can
	// log in, including subjects that were never bound to the role.
	if r.ID == consts.AUTHZ_ROLE_AUTHENTICATED {
		return errors.New("authenticated is reserved for the implicit role of every authenticated subject")
	}

	return nil
}

// CreateBefore validates the new role. The ID is left to the framework,
// which assigns a UUIDv7 like every other model; clients no longer need to
// invent role identifiers.
func (r *Role) CreateBefore(ctx context.Context) error {
	return r.validate()
}

// CreateAfter syncs the role's permissions after the role row has been persisted.
func (r *Role) CreateAfter(ctx context.Context) error {
	// Get the full role info before syncing permissions.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	return r.syncPermissions(ctx)
}

// UpdateBefore validates role updates before database writes. Role ID is immutable.
func (r *Role) UpdateBefore(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}

	// The tenant is neither read back nor compared. Two things already settle
	// it: the row this update can reach is scoped to the caller's tenant, so
	// another tenant's role is not found at all, and the tenant column is
	// written on insert and never again, so no update can move a role between
	// tenants whatever it sends.
	current := new(Role)
	return database.Database[*Role](ctx).Get(current, r.ID)
}

// UpdateAfter syncs the role's permissions after the role row has been persisted.
func (r *Role) UpdateAfter(ctx context.Context) error {
	// Get the full role info before syncing permissions.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	return r.syncPermissions(ctx)
}

// DeleteBefore checks the role exists and captures the tenant its rules were
// written under, while the row is still there to read. The cleanup itself waits
// for DeleteAfter, which runs on the other side of the row lock.
func (r *Role) DeleteBefore(ctx context.Context) error {
	if r.ID == "" {
		return errors.New("role id is required")
	}

	current := new(Role)
	if err := database.Database[*Role](ctx).WithoutHook().Get(current, r.ID); err != nil {
		return err
	}
	if len(r.TenantID) == 0 {
		r.TenantID = current.TenantID
	}
	return nil
}

// DeleteAfter removes the role's bindings and RBAC policies, after the DELETE
// statement and inside its transaction.
//
// After rather than before is what serializes this against a concurrent
// permission rebuild. A rebuild replaces the role's policy rows only while it
// holds the role's row — its own write path updated the row, the menu path
// locks it — so a cleanup running after this transaction has deleted that row
// cannot interleave with one: the rebuild either committed first and its rows
// are here to be removed, or it is waiting on the row lock and will find the
// role gone. Run before the DELETE, the cleanup raced those rebuilds on
// backends whose statements see only committed rows, and lost by leaving the
// rebuilt rules stored for a role that no longer exists.
func (r *Role) DeleteAfter(ctx context.Context) error {
	// The role ID alone identifies the bindings: it is unique across tenants,
	// and the listing is scoped to the caller's tenant anyway.
	roleBindings := make([]*RoleBinding, 0)
	if err := database.Database[*RoleBinding](ctx).WithQuery(&RoleBinding{RoleID: r.ID}).List(&roleBindings); err != nil {
		return err
	}
	if len(roleBindings) > 0 {
		// The rows go without their hooks. Each binding's DeleteBefore reads
		// the row back and unassigns the role one subject at a time, while
		// RemoveRole below drops every assignment to this role in one filtered
		// delete — so the hooks spend a read and a policy write per binding on
		// rules the next statement removes anyway, each taking the enforcer
		// write lock and leaving an after-commit action until the transaction
		// ends.
		//
		// What that gives up is a binding stored under a tenant other than the
		// role it names, which RemoveRole's filter does not reach. Nothing
		// writes one: a binding's own CreateBefore refuses a role belonging to
		// another tenant. Such a row is written around the framework, and the
		// drift report is what surfaces it.
		if err := database.Database[*RoleBinding](ctx).WithoutHook().Delete(roleBindings...); err != nil {
			return err
		}
	}

	return rbac.RBAC().RemoveRole(ctx, r.tenant(), r.ID)
}

// syncPermissions rebuilds the authz policy rows for this role from Menu.Routes.
// Role.MenuIDs is the authoritative source for backend route grants.
//
// The whole set is replaced rather than diffed. Menu routes can be removed,
// renamed, or have methods changed, and a diff-based update can leave stale policy
// rows behind. Rebuilding the role's policy set keeps authz_rules consistent with
// the current menu bindings, and SetRolePermissions applies it as one step so the
// role's members are never authorized against a partially rebuilt set.
func (r *Role) syncPermissions(ctx context.Context) error {
	// Batch-load bound menus with a typed IN filter. The comma-joined ID
	// shortcut is avoided on purpose: it breaks on integer AutoBase keys and
	// relies on values never containing commas.
	newMenus := make([]*Menu, 0)
	if err := database.Database[*Menu](ctx).WithQuery(&Menu{}, types.QueryOptions{
		AllowEmpty: true,
		Filters:    []types.Filter{types.FilterIn("id", r.MenuIDs)},
	}).List(&newMenus); err != nil {
		return err
	}

	permissions := make([]types.Permission, 0)
	for _, m := range newMenus {
		permissions = append(permissions, RoutePermissionsForMenu(m)...)
	}

	return rbac.RBAC().SetRolePermissions(ctx, r.tenant(), r.ID, permissions)
}

// RoutePermissionsForMenu renders the backend route grants a menu carries. It
// is exported so the reconciliation can derive the same expectation the sync
// writes, from one implementation rather than two that could disagree.
func RoutePermissionsForMenu(m *Menu) []types.Permission {
	if m == nil {
		return make([]types.Permission, 0)
	}

	// A menu can bind multiple backend routes, and each route can bind multiple
	// HTTP methods. The policy set stores those as individual path + method rows.
	permissions := make([]types.Permission, 0)
	for _, route := range m.Routes {
		object := strings.TrimSpace(route.Path)
		if len(object) == 0 {
			continue
		}
		for _, method := range route.Methods {
			method = strings.ToUpper(strings.TrimSpace(method))
			if len(method) == 0 {
				continue
			}
			permissions = append(permissions, types.Permission{Object: object, Action: method})
		}
	}
	return permissions
}

func (r *Role) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("tenant_id", string(r.TenantID))
	enc.AddString("name", r.Name)
	enc.AddString("id", r.ID)
	return nil
}
