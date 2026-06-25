package modelauthz

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
)

type UserRole struct {
	UserID string `json:"user_id,omitempty" schema:"user_id"`
	RoleID string `json:"role_id,omitempty" schema:"role_id"`

	model.Base
}

func (r *UserRole) Purge() bool { return true }
func (UserRole) Design() {
	dsl.Migrate(true)
	dsl.Route("/authz/user-roles", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {})
		dsl.List(func() {})
		dsl.Get(func() {})
	})
}

func (r *UserRole) CreateBefore(ctx context.Context) error {
	if len(r.UserID) == 0 {
		return errors.New("user_id is required")
	}
	if len(r.RoleID) == 0 {
		return errors.New("role_id is required")
	}

	// ensure role exists
	var role Role
	if err := database.Database[*Role](ctx).Get(&role, r.RoleID); err != nil {
		return err
	}

	// If the user already has the role, set same id to just update it.
	r.SetID(util.HashID(r.UserID, r.RoleID))

	return nil
}

func (r *UserRole) CreateAfter(ctx context.Context) error {
	// role.ID always equal role.Code
	if err := rbac.RBAC().AssignRole(r.UserID, r.RoleID); err != nil {
		return err
	}

	return nil
}

func (r *UserRole) DeleteBefore(ctx context.Context) error {
	// The delete request always don't have user_id and role_id, so we should get the role from database.
	if err := database.Database[*UserRole](ctx).Get(r, r.ID); err != nil {
		return err
	}
	// role.ID always equal role.Code
	return rbac.RBAC().UnassignRole(r.UserID, r.RoleID)
}

func (r *UserRole) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("user_id", r.UserID)
	enc.AddString("role_id", r.RoleID)
	_ = enc.AddObject("base", &r.Base)
	return nil
}
