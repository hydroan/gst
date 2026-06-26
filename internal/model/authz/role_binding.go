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

type RoleBinding struct {
	TenantID  string `json:"tenant_id,omitempty" schema:"tenant_id" gorm:"size:191;default:default;uniqueIndex:idx_authz_role_bindings_tenant_subject_role"`
	SubjectID string `json:"subject_id,omitempty" schema:"subject_id" gorm:"size:191;uniqueIndex:idx_authz_role_bindings_tenant_subject_role"`
	RoleID    string `json:"role_id,omitempty" schema:"role_id" gorm:"size:191;uniqueIndex:idx_authz_role_bindings_tenant_subject_role"`

	model.Base
}

func (r *RoleBinding) Purge() bool { return true }

func (r *RoleBinding) tenant() string {
	if r != nil && len(r.TenantID) > 0 {
		return r.TenantID
	}
	return rbac.DefaultTenant
}

func (RoleBinding) Design() {
	dsl.Migrate(true)
	dsl.Route("authz/role-bindings", func() {
		dsl.Create(func() {})
		dsl.Delete(func() {})
		dsl.List(func() {})
		dsl.Get(func() {})
	})
}

func (r *RoleBinding) CreateBefore(ctx context.Context) error {
	if len(r.SubjectID) == 0 {
		return errors.New("subject_id is required")
	}
	if len(r.RoleID) == 0 {
		return errors.New("role_id is required")
	}

	// ensure role exists
	var role Role
	if err := database.Database[*Role](ctx).Get(&role, r.RoleID); err != nil {
		return err
	}
	if role.tenant() != r.tenant() {
		return errors.New("role tenant does not match binding tenant")
	}

	// If the subject already has the role, set the same ID to update it.
	r.SetID(util.HashID(r.tenant(), r.SubjectID, r.RoleID))

	return nil
}

func (r *RoleBinding) CreateAfter(ctx context.Context) error {
	if err := rbac.RBAC().AssignRole(r.tenant(), r.SubjectID, r.RoleID); err != nil {
		return err
	}

	return nil
}

func (r *RoleBinding) DeleteBefore(ctx context.Context) error {
	// The delete request always doesn't have subject_id and role_id, so load the binding first.
	if err := database.Database[*RoleBinding](ctx).Get(r, r.ID); err != nil {
		return err
	}
	return rbac.RBAC().UnassignRole(r.tenant(), r.SubjectID, r.RoleID)
}

func (r *RoleBinding) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("tenant_id", r.TenantID)
	enc.AddString("subject_id", r.SubjectID)
	enc.AddString("role_id", r.RoleID)
	_ = enc.AddObject("base", &r.Base)
	return nil
}
