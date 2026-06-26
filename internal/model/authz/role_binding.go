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
	SubjectID string `json:"subject_id,omitempty" schema:"subject_id"`
	RoleID    string `json:"role_id,omitempty" schema:"role_id"`

	model.Base
}

func (r *RoleBinding) Purge() bool { return true }
func (RoleBinding) Design() {
	dsl.Migrate(true)
	dsl.Route("/authz/role-bindings", func() {
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

	// If the subject already has the role, set the same ID to update it.
	r.SetID(util.HashID(r.SubjectID, r.RoleID))

	return nil
}

func (r *RoleBinding) CreateAfter(ctx context.Context) error {
	// role.ID always equal role.Code
	if err := rbac.RBAC().AssignRole(r.SubjectID, r.RoleID); err != nil {
		return err
	}

	return nil
}

func (r *RoleBinding) DeleteBefore(ctx context.Context) error {
	// The delete request always doesn't have subject_id and role_id, so load the binding first.
	if err := database.Database[*RoleBinding](ctx).Get(r, r.ID); err != nil {
		return err
	}
	// role.ID always equal role.Code
	return rbac.RBAC().UnassignRole(r.SubjectID, r.RoleID)
}

func (r *RoleBinding) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if r == nil {
		return nil
	}
	enc.AddString("subject_id", r.SubjectID)
	enc.AddString("role_id", r.RoleID)
	_ = enc.AddObject("base", &r.Base)
	return nil
}
