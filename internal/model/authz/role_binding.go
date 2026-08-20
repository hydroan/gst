package modelauthz

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/util"
)

type RoleBinding struct {
	tenant.Scope

	SubjectID string `json:"subject_id,omitempty" query:"subject_id" gorm:"size:191"`
	RoleID    string `json:"role_id,omitempty" query:"role_id" gorm:"size:191"`

	model.Base
}

// Indexes declares that a subject holds a role at most once inside a tenant.
//
// It moved off the struct tags because the tenant column now arrives through an
// embedded struct, and a tag on an embedded field cannot name the fields beside
// it. The columns and the uniqueness are unchanged.
func (RoleBinding) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"TenantID", "SubjectID", "RoleID"}, Unique: true}}
}

func (r *RoleBinding) Purge() bool { return true }

// TableName pins the table name gorm would otherwise derive.
func (r *RoleBinding) TableName() string { return "role_bindings" }

func (r *RoleBinding) tenant() string {
	if r != nil && len(r.TenantID) > 0 {
		return string(r.TenantID)
	}
	return tenant.Default
}

func (RoleBinding) Design() {
	dsl.Migrate()
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

	// The tenant comes from the context, not from the binding. The framework
	// stamps the column on insert, which happens after this hook, so the value
	// the binding arrived with is whatever the client sent — and keying the row
	// by that would file it under a tenant it will not end up in. A
	// cross-tenant caller acts in no single tenant and has to have named one.
	scoped, ok := tenant.From(ctx)
	if !ok {
		scoped = r.tenant()
	}

	// ensure the role exists, in the tenant this binding will belong to
	var role Role
	if err := database.Database[*Role](ctx).Get(&role, r.RoleID); err != nil {
		return err
	}
	if role.tenant() != scoped {
		return errors.New("role tenant does not match binding tenant")
	}

	// A subject holds a role at most once in a tenant, so the same three values
	// always name the same row.
	r.SetID(util.HashID(scoped, r.SubjectID, r.RoleID))

	return nil
}

func (r *RoleBinding) CreateAfter(ctx context.Context) error {
	if err := rbac.RBAC().AssignRole(ctx, r.tenant(), r.SubjectID, r.RoleID); err != nil {
		return err
	}

	return nil
}

func (r *RoleBinding) DeleteBefore(ctx context.Context) error {
	// The delete request always doesn't have subject_id and role_id, so load the binding first.
	if err := database.Database[*RoleBinding](ctx).Get(r, r.ID); err != nil {
		return err
	}
	return rbac.RBAC().UnassignRole(ctx, r.tenant(), r.SubjectID, r.RoleID)
}
