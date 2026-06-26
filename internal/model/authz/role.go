package modelauthz

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/datatypes"
)

type Role struct {
	TenantID string `json:"tenant_id,omitempty" schema:"tenant_id" gorm:"size:191;default:default;uniqueIndex:idx_authz_roles_tenant_code"`
	Name     string `json:"name,omitempty" schema:"name" gorm:"size:191"`
	Code     string `json:"code,omitempty" schema:"code" gorm:"size:191;uniqueIndex:idx_authz_roles_tenant_code"`
	Default  *bool  `json:"default,omitempty" schema:"default"`

	// Scope holds generic constraints for regional roles.
	// Keys and values are user-defined and framework-agnostic.
	Scope datatypes.JSONMap `json:"scope,omitempty"`

	// Menu permissions owned by this role
	MenuIDs        datatypes.JSONSlice[string] `json:"menu_ids,omitempty"`
	MenuPartialIDs datatypes.JSONSlice[string] `json:"menu_partial_ids,omitempty"`
	ButtonIDs      datatypes.JSONSlice[string] `json:"button_ids,omitempty"`

	Menus        []*Menu `json:"menus,omitempty" gorm:"-"`
	MenuPartials []*Menu `json:"menu_partials,omitempty" gorm:"-"`

	model.Base
}

func (Role) Design() {
	dsl.Migrate(true)
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

func (r *Role) tenant() string {
	if r != nil && len(r.TenantID) > 0 {
		return r.TenantID
	}
	return rbac.DefaultTenant
}

func (r *Role) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Code = strings.TrimSpace(r.Code)

	if len(r.Name) == 0 {
		return errors.New("role name is required")
	}
	if len(r.Code) == 0 {
		return errors.New("role code is required")
	}

	return nil
}

func (r *Role) CreateBefore(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}

	r.SetID(r.Code)
	return nil
}

// CreateAfter syncs the role's permissions after the role row has been persisted.
func (r *Role) CreateAfter(ctx context.Context) error {
	// Get the full role info before syncing permissions.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.syncPermissions(ctx)
	e2 := rbac.RBAC().AddRole(r.tenant(), r.ID)
	return errors.Join(e1, e2)
}

// UpdateBefore validates role updates before database writes. Role code is immutable.
func (r *Role) UpdateBefore(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}

	current := new(Role)
	if err := database.Database[*Role](ctx).Get(current, r.ID); err != nil {
		return err
	}

	if current.Code != r.Code {
		return errors.New("role code is immutable")
	}
	if len(r.TenantID) == 0 {
		r.TenantID = current.TenantID
	}
	if current.tenant() != r.tenant() {
		return errors.New("role tenant is immutable")
	}
	return nil
}

// UpdateAfter syncs the role's permissions after the role row has been persisted.
func (r *Role) UpdateAfter(ctx context.Context) error {
	// Get the full role info before syncing permissions.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.syncPermissions(ctx)
	e2 := rbac.RBAC().AddRole(r.tenant(), r.ID)
	return errors.Join(e1, e2)
}

// DeleteBefore deletes the role's RBAC policies before the role row is removed.
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

	roleBindings := make([]*RoleBinding, 0)
	if err := database.Database[*RoleBinding](ctx).WithQuery(&RoleBinding{TenantID: r.tenant(), RoleID: r.ID}).List(&roleBindings); err != nil {
		return err
	}
	if len(roleBindings) > 0 {
		if err := database.Database[*RoleBinding](ctx).Delete(roleBindings...); err != nil {
			return err
		}
	}

	return rbac.RBAC().RemoveRole(r.tenant(), r.ID)
}

type routePolicy struct {
	object string
	action string
}

// syncPermissions rebuilds Casbin policy rows for this role from Menu.Routes.
// Role.MenuIDs is the authoritative source for backend route grants. MenuPartialIDs
// only keeps partially selected parent menus visible in the frontend tree and must
// not grant backend API access by itself.
//
// The method intentionally uses revoke-all-then-grant. Menu routes can be removed,
// renamed, or have methods changed, and a diff-based update can leave stale Casbin
// rows behind. Rebuilding the role's policy set keeps casbin_rule consistent with
// the current menu bindings.
func (r *Role) syncPermissions(ctx context.Context) error {
	newMenus := make([]*Menu, 0)
	if err := database.Database[*Menu](ctx).WithQuery(&Menu{Base: model.Base{ID: strings.Join(r.MenuIDs, ",")}}).List(&newMenus); err != nil {
		zap.S().Error(err)
		return err
	}

	newPolicies := make([]routePolicy, 0)
	for _, m := range newMenus {
		newPolicies = append(newPolicies, routePoliciesForMenu(m)...)
	}

	if err := rbac.RBAC().RevokeRolePermissions(r.tenant(), r.ID); err != nil {
		zap.S().Error(err)
		return err
	}
	for _, p := range newPolicies {
		if err := rbac.RBAC().GrantPermission(r.tenant(), r.ID, p.object, p.action); err != nil {
			zap.S().Error(err)
			return err
		}
	}

	return nil
}

func routePoliciesForMenu(m *Menu) []routePolicy {
	if m == nil {
		return make([]routePolicy, 0)
	}

	// A menu can bind multiple backend routes, and each route can bind multiple
	// HTTP methods. Casbin stores those as individual path + method policies.
	policies := make([]routePolicy, 0)
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
			policies = append(policies, routePolicy{object: object, action: method})
		}
	}
	return policies
}

func (r *Role) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("tenant_id", r.TenantID)
	enc.AddString("code", r.Code)
	enc.AddString("name", r.Name)
	enc.AddString("id", r.ID)
	return nil
}
