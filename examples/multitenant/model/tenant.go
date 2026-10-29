package model

import (
	"context"
	"net/http"
	"strings"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/consts"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Tenant is the registry of the tenants the deployment serves: one row per
// tenant, its id the tenant the sessions and the scoped rows name. It is a
// global table, not a tenant's own: it carries no tenant.Scope, every
// tenant reads it, and the one thing the row scoping cannot decide is who
// may write it, since a write reaches every tenant. The hooks below answer
// that: a write with a subject behind it is the system root's alone, and a
// write with none is the deployment's own, the seed in module. GET
// /api/tenants is granted to every authenticated subject by the seed.
type Tenant struct {
	Name string `json:"name" query:"name" gorm:"size:64" binding:"required,max=64"`

	model.Base
}

func (Tenant) TableName() string { return "tenants" }

func (Tenant) Design() {
	Migrate()
	Endpoint("tenants")

	Create(func() {})
	Delete(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})
}

// CreateBefore holds the id to what a tenant is named by: lower-case
// letters, digits and dashes, which every session names it with as it is.
func (t *Tenant) CreateBefore(ctx context.Context) error {
	if err := errIfNotSystemRoot(ctx); err != nil {
		return err
	}
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" || strings.Trim(t.ID, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return gst.NewError(http.StatusBadRequest, "a tenant is named by lower-case letters, digits and dashes")
	}
	return nil
}

func (t *Tenant) UpdateBefore(ctx context.Context) error { return errIfNotSystemRoot(ctx) }
func (t *Tenant) DeleteBefore(ctx context.Context) error { return errIfNotSystemRoot(ctx) }

// errIfNotSystemRoot refuses a tenant write from a subject that is not the
// system root. Tenants are global: a tenant's administrator reaches every
// route of its tenant, this one included, so the boundary sits on the
// write, where every path to it converges. A write with no subject behind
// it is the deployment's own hand, the seed.
func errIfNotSystemRoot(ctx context.Context) error {
	subject := strings.TrimSpace(gst.RequestUserID(ctx))
	if subject == "" {
		return nil
	}
	systemRoot, err := rbac.RBAC().HasSystemRole(ctx, subject, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if !systemRoot {
		return gst.NewError(http.StatusForbidden, "only the system root may change the tenants")
	}
	return nil
}
