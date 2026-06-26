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
	Name    string `json:"name,omitempty" schema:"name" gorm:"size:191;unique"`
	Code    string `json:"code,omitempty" schema:"code" gorm:"size:191;unique"`
	Default *bool  `json:"default,omitempty" schema:"default"`

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
func (r *Role) CreateBefore(ctx context.Context) error {
	// validate fields
	if err := r.validate(); err != nil {
		return err
	}
	// ensure role id always equal to role code.
	r.SetID(r.Code)
	return nil
}

// CreateAfter creates the role's permissions after the role row has been persisted.
func (r *Role) CreateAfter(ctx context.Context) error {
	// get the full role info before UpdatePermission.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// UpdateBefore validates role updates before database writes. Role code is immutable.
func (r *Role) UpdateBefore(ctx context.Context) error {
	// validate fields
	if err := r.validate(); err != nil {
		return err
	}

	// query the full role.
	current := new(Role)
	if err := database.Database[*Role](ctx).Get(current, r.ID); err != nil {
		return err
	}

	// ensure role code is immutable
	if current.Code != r.Code {
		return errors.New("role code is immutable")
	}
	return nil
}

// UpdateAfter refreshes the role's permissions after the role row has been persisted.
func (r *Role) UpdateAfter(ctx context.Context) error {
	// get the full role info before UpdatePermission.
	if err := database.Database[*Role](ctx).WithoutHook().Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// DeleteBefore will delete the role's permissions
// We must remove role in DeleteBefore hook, otherwise database.Database[*Role](ctx).Get(r, r.ID) will failed.
func (r *Role) DeleteBefore(ctx context.Context) error {
	if r.ID == "" {
		return errors.New("role id is required")
	}
	// removes the role's permissions
	if err := rbac.RBAC().RevokePermission(r.ID, "", ""); err != nil {
		return err
	}
	return rbac.RBAC().RemoveRole(r.ID)
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

// UpdatePermission refreshes permissions for the current role.
// It uses a brute-force strategy: revoke all existing policies for the role,
// then grant permissions derived from the current menus. This avoids any
// unknown leftovers in the casbin_rule table and ensures strong consistency.
func (r *Role) UpdatePermission(ctx context.Context) error {
	// We should always iterate role's "MenuIds", not "MenuPartialIds".
	// "MenuIds" is the frontend menus, "MenuPartialIds" is the frontend menus group that has no menus.
	// A "Menu" contains one or multiple backend routes, each route binding one or multiple permissions.

	o := new(Role)
	if err := database.Database[*Role](ctx).Get(o, r.ID); err != nil {
		zap.S().Error(err)
		return err
	}

	// query the new role's menus
	newMenus := make([]*Menu, 0)
	if err := database.Database[*Menu](ctx).WithQuery(&Menu{Base: model.Base{ID: strings.Join(r.MenuIDs, ",")}}).List(&newMenus); err != nil {
		zap.S().Error(err)
		return err
	}
	// derive new role policies from menu routes.
	newPolicies := make([]routePolicy, 0)
	for _, m := range newMenus {
		newPolicies = append(newPolicies, routePoliciesForMenu(m)...)
	}

	// revoke all existing policies for this role to avoid leftovers
	if err := rbac.RBAC().RevokePermission(r.ID, "", ""); err != nil {
		zap.S().Error(err)
		return err
	}
	// grant the new role's permissions
	for _, p := range newPolicies {
		if err := rbac.RBAC().GrantPermission(r.ID, p.resource, p.action); err != nil {
			zap.S().Error(err)
			return err
		}
	}

	return nil
}

type routePolicy struct {
	resource string
	action   string
}

func routePoliciesForMenu(m *Menu) []routePolicy {
	if m == nil {
		return make([]routePolicy, 0)
	}

	policies := make([]routePolicy, 0)
	for _, route := range m.Routes {
		resource := strings.TrimSpace(route.Path)
		if len(resource) == 0 {
			continue
		}
		for _, method := range route.Methods {
			method = strings.ToUpper(strings.TrimSpace(method))
			if len(method) == 0 {
				continue
			}
			policies = append(policies, routePolicy{resource: resource, action: method})
		}
	}
	return policies
}

func (r *Role) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("code", r.Code)
	enc.AddString("name", r.Name)
	enc.AddString("id", r.ID)
	return nil
}
