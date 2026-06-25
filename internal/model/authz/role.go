package modelauthz

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
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

func (r *Role) Purge() bool                            { return true }
func (r *Role) CreateBefore(ctx context.Context) error { return r.validateCreate(ctx) }

// CreateAfter creates the role's permissions after the role row has been persisted.
func (r *Role) CreateAfter(ctx context.Context) error {
	if err := database.Database[*Role](ctx).Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// UpdateBefore validates role updates before database writes. Role code is immutable.
func (r *Role) UpdateBefore(ctx context.Context) error {
	return r.validateUpdate(ctx)
}

// UpdateAfter refreshes the role's permissions after the role row has been persisted.
func (r *Role) UpdateAfter(ctx context.Context) error {
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// DeleteBefore will delete the role's permissions
func (r *Role) DeleteBefore(ctx context.Context) error {
	// The delete request always don't have role id, so we should get the role from database.
	if err := database.Database[*Role](ctx).Get(r, r.ID); err != nil {
		return err
	}

	if err := rbac.RBAC().RemoveRole(r.Code); err != nil {
		return err
	}

	// removes the role's permissions
	menus := make([]*Menu, 0)
	permissions := make([]*Permission, 0)
	if err := database.Database[*Menu](ctx).
		WithQuery(&Menu{Base: model.Base{ID: strings.Join(r.MenuIDs, ",")}}).
		List(&menus); err != nil {
		return err
	}
	for _, m := range menus {
		result, err := permissionsForRoutes(ctx, m.Routes)
		if err != nil {
			return err
		}
		permissions = append(permissions, result...)
	}

	// revoke the role's permissions
	for _, p := range permissions {
		if err := rbac.RBAC().RevokePermission(r.Code, p.Resource, p.Action); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePermission refreshes permissions for the current role code.
// It uses a brute-force strategy: revoke all existing policies for the role,
// then grant permissions derived from the current menus. This avoids any
// unknown leftovers in the casbin_rule table and ensures strong consistency.
func (r *Role) UpdatePermission(ctx context.Context) error {
	// We should always iterate role's "MenuIds", not "MenuPartialIds".
	// "MenuIds" is the frontend menus, "MenuPartialIds" is the frontend menus group that has no menus.
	// A "Menu" contains one or multiple backend routes, each route binding one or multiple permissions.

	var (
		newMenus       = make([]*Menu, 0)
		newPermissions = make([]*Permission, 0)
	)

	o := new(Role)
	if err := database.Database[*Role](ctx).Get(o, r.ID); err != nil {
		zap.S().Error(err)
		return err
	}

	// query the new role's menus
	if err := database.Database[*Menu](ctx).
		WithQuery(&Menu{Base: model.Base{ID: strings.Join(r.MenuIDs, ",")}}).
		List(&newMenus); err != nil {
		zap.S().Error(err)
		return err
	}

	// query the new role's permissions
	for _, m := range newMenus {
		result, err := permissionsForRoutes(ctx, m.Routes)
		if err != nil {
			return err
		}
		newPermissions = append(newPermissions, result...)
	}

	for _, p := range newPermissions {
		zap.S().Infow("new permission", "role", r.Code, "resource", p.Resource, "action", p.Action, "effect", consts.EffectAllow)
	}

	// revoke all existing policies for this role to avoid leftovers
	if err := rbac.RBAC().RevokePermission(r.Code, "", ""); err != nil {
		zap.S().Error(err)
		return err
	}
	// grant the new role's permissions
	for _, p := range newPermissions {
		if err := rbac.RBAC().GrantPermission(r.Code, p.Resource, p.Action); err != nil {
			zap.S().Error(err)
			return err
		}
	}

	zap.S().Infow("update role", "old", o.Code, "new", r.Code)

	return nil
}

func permissionsForRoutes(ctx context.Context, routes []Route) ([]*Permission, error) {
	permissions := make([]*Permission, 0)
	for _, route := range routes {
		if len(route.Path) == 0 {
			continue
		}
		for _, method := range route.Methods {
			method = strings.ToUpper(strings.TrimSpace(method))
			if len(method) == 0 {
				continue
			}
			result := make([]*Permission, 0)
			if err := database.Database[*Permission](ctx).
				WithQuery(&Permission{Resource: route.Path, Action: method}).
				List(&result); err != nil {
				zap.S().Error(err)
				return nil, err
			}
			permissions = append(permissions, result...)
		}
	}
	return permissions, nil
}

func (r *Role) validateCreate(ctx context.Context) error {
	return r.validateFields()
}

func (r *Role) validateUpdate(ctx context.Context) error {
	if err := r.validateFields(); err != nil {
		return err
	}
	current := new(Role)
	if err := database.Database[*Role](ctx).Get(current, r.ID); err != nil {
		return err
	}
	if current.Code != r.Code {
		return errors.New("role code is immutable")
	}
	return nil
}

func (r *Role) validateFields() error {
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

func (r *Role) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("code", r.Code)
	enc.AddString("name", r.Name)
	enc.AddString("id", r.ID)
	return nil
}
