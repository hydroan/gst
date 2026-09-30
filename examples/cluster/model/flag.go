package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Flag is a switch every replica reads from the shared database: turned on or
// off through any replica, over HTTP or gRPC, it is what all of them see from
// then on. It declares the whole standard set of actions, the batch ones
// included, so every standard rpc has a table behind it in this project. The
// Create hook in service/flag checks the name and fills in the percent of a
// flag turned on without one: a binding tag would apply to a patch too, which
// carries only the fields it changes.
type Flag struct {
	model.Base
	model.Query

	Name    string `json:"name" query:"name" gorm:"size:64;not null" pb:"11"`
	On      bool   `json:"on" query:"on" gorm:"not null" pb:"12"`
	Percent int    `json:"percent" query:"percent" gorm:"not null" binding:"min=0,max=100" pb:"13"` // the share of traffic the flag applies to, 0 to 100
	Note    string `json:"note" gorm:"size:191" pb:"14"`
}

func (Flag) TableName() string      { return "flags" }
func (Flag) Indexes() []model.Index { return []model.Index{{Fields: []string{"Name"}}} }

func (Flag) Design() {
	GRPC()
	Migrate()
	Endpoint("flags")

	Create(func() {
		Service()
	})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})
	CreateMany(func() {})
	DeleteMany(func() {})
	UpdateMany(func() {})
	PatchMany(func() {})
}
