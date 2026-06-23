package modelauthz

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
)

type UserRole struct {
	UserID string `json:"user_id,omitempty" schema:"user_id"`
	RoleID string `json:"role_id,omitempty" schema:"role_id"`

	RoleCode string `json:"rolecode,omitempty" schema:"rolecode"` // 角色Code, 用于 RBAC 角色控制和前端查询
	Username string `json:"username,omitempty" schema:"username"` // 用户名, 用于 RBAC 用户控制和前端查询

	User *User `json:"user,omitempty" gorm:"-"`
	Role *Role `json:"role,omitempty" gorm:"-"`

	model.Base
}

func (r *UserRole) Purge() bool { return true }
func (UserRole) Design() {
	dsl.Route("/authz/user_roles", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {
			dsl.Service(true)
			dsl.Flatten()
			dsl.Filename("user_role.go")
		})
		dsl.Update(func() {})
		dsl.Patch(func() {})
		dsl.List(func() {
			dsl.Service(true)
			dsl.Flatten()
			dsl.Filename("user_role.go")
		})
		dsl.Get(func() {})
	})
}

func (r *UserRole) CreateBefore(ctx *types.ModelContext) error {
	if len(r.UserID) == 0 {
		return errors.New("user_id is required")
	}
	if len(r.RoleID) == 0 {
		return errors.New("role_id is required")
	}
	// expands field: user and role
	user, role := new(User), new(Role)
	if err := database.Database[*User](ctx.DatabaseContext()).Get(user, r.UserID); err != nil {
		return err
	}
	if err := database.Database[*Role](ctx.DatabaseContext()).Get(role, r.RoleID); err != nil {
		return err
	}
	r.Username, r.RoleCode = user.Username, role.Code

	// If the user already has the role, set same id to just update it.
	r.SetID(util.HashID(r.UserID, r.RoleID))

	return nil
}

func (r *UserRole) CreateAfter(ctx *types.ModelContext) error {
	if err := database.Database[*UserRole](ctx.DatabaseContext()).Update(r); err != nil {
		return err
	}
	// NOTE: must be role name not role id.
	if err := rbac.RBAC().AssignRole(r.UserID, r.RoleCode); err != nil {
		return err
	}

	// update casbin_rule field: `user`, `role`, `remark`
	user := new(User)
	if err := database.Database[*User](ctx.DatabaseContext()).Get(user, r.UserID); err != nil {
		return err
	}
	casbinRules := make([]*CasbinRule, 0)
	if err := database.Database[*CasbinRule](ctx.DatabaseContext()).WithLimit(1).WithQuery(&CasbinRule{V0: r.UserID, V1: r.RoleCode}).List(&casbinRules); err != nil {
		return err
	}
	if len(casbinRules) > 0 {
		casbinRules[0].User = user.Username
		casbinRules[0].Role = r.RoleCode
		casbinRules[0].Remark = new(fmt.Sprintf("%s -> %s", r.Username, r.RoleCode))
		return database.Database[*CasbinRule](ctx.DatabaseContext()).Update(casbinRules[0])
	}
	return nil
}

func (r *UserRole) DeleteBefore(ctx *types.ModelContext) error {
	// The delete request always don't have user_id and role_id, so we should get the role from database.
	if err := database.Database[*UserRole](ctx.DatabaseContext()).Get(r, r.ID); err != nil {
		return err
	}
	// NOTE: must be role name not role id.
	return rbac.RBAC().UnassignRole(r.UserID, r.RoleCode)
}

func (r *UserRole) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("user_id", r.UserID)
	enc.AddString("role_id", r.RoleID)
	enc.AddString("user", r.Username)
	enc.AddString("role", r.RoleCode)
	_ = enc.AddObject("base", &r.Base)
	return nil
}
