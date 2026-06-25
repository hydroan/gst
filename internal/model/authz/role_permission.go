package modelauthz

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
)

// RolePermission is a permission for a role
//
// # Deprecated
//
// RolePermission is deprecated, operations CasbinRule directly.
type RolePermission struct {
	Role string `json:"role" schema:"role"`

	Resource string        `json:"resource" schema:"resource"`
	Action   string        `json:"action" schema:"action"`
	Effect   consts.Effect `json:"effect" schema:"effect"`

	model.Base
}

func (r *RolePermission) Purge() bool { return true }
func (r *RolePermission) CreateBefore(context.Context) error {
	if len(r.Role) == 0 {
		return errors.New("role_id is required")
	}
	if len(r.Resource) == 0 {
		return errors.New("resource is required")
	}
	if len(r.Action) == 0 {
		return errors.New("action is required")
	}

	// default effect is allow.
	switch r.Effect {
	case consts.EffectAllow, consts.EffectDeny:
	default:
		r.Effect = consts.EffectAllow
	}
	// If the role already has the permission(Resource+Action), set same id to just update it.
	r.SetID(util.HashID(r.Role, r.Resource, r.Action))

	return nil
}

func (r *RolePermission) CreateAfter(context.Context) error {
	// grant the permission: (role, resource, action)
	return rbac.RBAC().GrantPermission(r.Role, r.Resource, r.Action)
}

func (r *RolePermission) DeleteBefore(ctx context.Context) error {
	// The request always only contains id, so we should get the RolePermission from database.
	if err := database.Database[*RolePermission](ctx).Get(r, r.ID); err != nil {
		return err
	}
	// revoke the role's permission
	return rbac.RBAC().RevokePermission(r.Role, r.Resource, r.Action)
}

func (r *RolePermission) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("role", r.Role)
	enc.AddString("resource", r.Resource)
	enc.AddString("action", r.Action)
	enc.AddString("effect", string(r.Effect))
	return nil
}
