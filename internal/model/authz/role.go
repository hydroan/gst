package modelauthz

import (
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
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

	Remark *string `json:"remark,omitempty" gorm:"size:10240" schema:"remark"` // Optional role description generated from menus.

	model.Base
}

func (r *Role) Purge() bool                                { return true }
func (r *Role) CreateBefore(ctx *types.ModelContext) error { return r.validate(ctx) }

// CreateAfter will creates the role's permissions.
func (r *Role) CreateAfter(ctx *types.ModelContext) error {
	if err := database.Database[*Role](ctx.DatabaseContext()).Get(r, r.ID); err != nil {
		return err
	}
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// UpdateBefore will delete the old role's permissions and create the new role's permissions.
// more details see "UpdatePermission".
func (r *Role) UpdateBefore(ctx *types.ModelContext) error {
	e1 := r.UpdatePermission(ctx)
	e2 := rbac.RBAC().AddRole(r.Code)
	return errors.Join(e1, e2)
}

// DeleteBefore will delete the role's permissions
func (r *Role) DeleteBefore(ctx *types.ModelContext) error {
	// The delete request always don't have role id, so we should get the role from database.
	if err := database.Database[*Role](ctx.DatabaseContext()).Get(r, r.ID); err != nil {
		return err
	}

	if err := rbac.RBAC().RemoveRole(r.Code); err != nil {
		return err
	}

	// removes the role's permissions
	menus := make([]*Menu, 0)
	permissions := make([]*Permission, 0)
	if err := database.Database[*Menu](ctx.DatabaseContext()).
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

// UpdatePermission must run in the "UpdateBefore" hook.
// It uses a brute-force strategy: revoke all existing policies for the role,
// then grant permissions derived from the current menus. This avoids any
// unknown leftovers in the casbin_rule table and ensures strong consistency.
func (r *Role) UpdatePermission(ctx *types.ModelContext) error {
	// We should always iterate role's "MenuIds", not "MenuPartialIds".
	// "MenuIds" is the frontend menus, "MenuPartialIds" is the frontend menus group that has no menus.
	// A "Menu" contains one or multiple backend routes, each route binding one or multiple permissions.

	var (
		newMenus       = make([]*Menu, 0)
		newPermissions = make([]*Permission, 0)
	)

	o := new(Role)
	if err := database.Database[*Role](ctx.DatabaseContext()).Get(o, r.ID); err != nil {
		zap.S().Error(err)
		return err
	}

	// query the new role's menus
	if err := database.Database[*Menu](ctx.DatabaseContext()).
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

func permissionsForRoutes(ctx *types.ModelContext, routes []Route) ([]*Permission, error) {
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
			if err := database.Database[*Permission](ctx.DatabaseContext()).
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

// validate will validate the role's name and code and ensure the role not exists.
func (r *Role) validate(ctx *types.ModelContext) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Code = strings.TrimSpace(r.Code)
	if len(r.Name) == 0 {
		return errors.New("role name is required")
	}
	if len(r.Code) == 0 {
		return errors.New("role code is required")
	}

	// Ensure uniqueness on (name, code)
	roles := make([]*Role, 0)
	if err := database.Database[*Role](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(&Role{Name: r.Name, Code: r.Code}).
		List(&roles); err != nil {
		return err
	}
	if len(roles) > 0 {
		return errors.New("role already exists")
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
