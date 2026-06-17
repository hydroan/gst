package modeliamgroup

import (
	. "github.com/hydroan/gst/dsl"
	modeliamtenant "github.com/hydroan/gst/internal/model/iam/tenant"
	"github.com/hydroan/gst/model"
)

var DefaultGroup = Group{
	Name: "default",
	Base: model.Base{ID: "default"},
}

// GroupType defines IAM group categories.
type GroupType string

const (
	GroupTypeRegular    GroupType = "regular"
	GroupTypeDepartment GroupType = "department"
	GroupTypeTeam       GroupType = "team"
	GroupTypeProject    GroupType = "project"
	GroupTypeRole       GroupType = "role"
)

// GroupStatus defines IAM group lifecycle states.
type GroupStatus string

const (
	GroupStatusActive   GroupStatus = "active"
	GroupStatusInactive GroupStatus = "inactive"
)

// Group represents an IAM group resource.
type Group struct {
	Name   string      `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Type   GroupType   `json:"type" gorm:"type:varchar(20);default:'regular';index"`
	Status GroupStatus `json:"status" gorm:"type:varchar(20);default:'active';index"`

	ParentID *string `json:"parent_id" gorm:"index"`
	Path     string  `json:"path" gorm:"type:varchar(500);index"`
	Level    int     `json:"level" gorm:"default:0;index"`

	TenantID *string                `json:"tenant_id" gorm:"index"`
	Tenant   *modeliamtenant.Tenant `json:"tenant,omitempty" gorm:"-"`

	model.Base
}

func (Group) Design() {
	Migrate(true)
	Endpoint("groups")

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

func (Group) Purge() bool { return true }
