package modeliamtenant

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusInactive  TenantStatus = "inactive"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusExpired   TenantStatus = "expired"
)

type TenantType string

const (
	TenantTypeEnterprise TenantType = "enterprise"
	TenantTypePro        TenantType = "pro"
	TenantTypeBasic      TenantType = "basic"
	TenantTypeTrial      TenantType = "trial"
)

type Tenant struct {
	Name   string       `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Status TenantStatus `json:"status" gorm:"type:varchar(20);default:'inactive';index"`
	Type   TenantType   `json:"type" gorm:"type:varchar(20);default:'basic';index"`

	model.Base
}

func (Tenant) Design() {
	Migrate(true)
	Endpoint("tenants")

	Create(func() {
		Enabled(true)
	})
	CreateMany(func() {
		Enabled(true)
	})
	Delete(func() {
		Enabled(true)
	})
	DeleteMany(func() {
		Enabled(true)
	})
	Update(func() {
		Enabled(true)
	})
	UpdateMany(func() {
		Enabled(true)
	})
	Patch(func() {
		Enabled(true)
	})
	PatchMany(func() {
		Enabled(true)
	})
	List(func() {
		Enabled(true)
	})
	Get(func() {
		Enabled(true)
	})
}

func (Tenant) Purge() bool { return true }
