package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
)

// TicketStatus tells whether a ticket is still open.
type TicketStatus string

const (
	TicketStatusOpen   TicketStatus = "open"
	TicketStatusClosed TicketStatus = "closed"
)

// Ticket is a tenant's resource: embedding tenant.Scope is the whole of the
// declaration, and brings the tenant_id column the framework stamps on
// insert and every statement is narrowed by, so a list, a get, an update or
// a delete reaches the caller's tenant alone and the service never names
// the tenant. The standard actions serve /api/tickets over HTTP and, the
// model declaring GRPC(), the TicketService rpcs; only Create declares
// Service(), for the hook in service/ticket that derives the key. A code is
// unique within a tenant and free for every other, which the index on the
// tenant and the code declares; one on the code alone would be refused.
type Ticket struct {
	Code   string       `json:"code" query:"code" gorm:"size:32;not null" binding:"required,max=32" pb:"11"`
	Title  string       `json:"title" query:"title" gorm:"size:191" binding:"max=191" pb:"12"`
	Status TicketStatus `json:"status" query:"status" gorm:"size:16;not null" binding:"omitempty,oneof=open closed" pb:"13"`
	// Key names the ticket across tenants, the tenant and the code joined:
	// derived in service/ticket from the tenant the row will carry, which
	// tenant.From reports, since the stamp is written after the hooks.
	Key string `json:"key" query:"key" gorm:"size:64" pb:"14"`

	tenant.Scope
	model.Query
	model.Base
}

func (Ticket) TableName() string { return "tickets" }

// Indexes declares the code unique within the tenant.
func (Ticket) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"TenantID", "Code"}, Unique: true}}
}

func (Ticket) Design() {
	GRPC()
	Migrate()
	Endpoint("tickets")

	Create(func() {
		Service()
	})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})
}
