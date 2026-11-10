package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
)

// Ticket is the tenant's resource of the multi-tenant scenario: embedding
// tenant.Scope brings the tenant_id column the framework stamps on insert
// and narrows every statement by, so a list reaches the caller's tenant
// alone. The standard actions serve /api/tickets. The code is unique within
// a tenant, which the index on the tenant and the code declares; the index
// on the tenant and the creation time serves the default order of a list.
type Ticket struct {
	Code     string `json:"code" query:"code" gorm:"size:32;not null" binding:"required,max=32"`
	Title    string `json:"title" query:"title" gorm:"size:200" binding:"max=200"`
	Category string `json:"category" query:"category" gorm:"size:32;not null" binding:"required,max=32"`
	Status   int    `json:"status" query:"status"`
	Body     string `json:"body" gorm:"type:text"`

	tenant.Scope
	model.Query
	model.Base
}

func (Ticket) TableName() string { return "tickets" }

func (Ticket) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"TenantID", "Code"}, Unique: true},
		{Fields: []string{"TenantID", "CreatedAt"}},
		{Fields: []string{"TenantID", "Category", "CreatedAt"}},
	}
}

func (Ticket) Design() {
	Migrate()
	Endpoint("tickets")

	Create(func() {})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})
}
