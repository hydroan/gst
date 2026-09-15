package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Progress is the state of the leader work: how far the counter got, and
// which replica moved it last. It lives in the database so that a replica
// taking the leadership over continues from where the last leader stopped.
type Progress struct {
	model.Base
	model.Query

	Name    string `json:"name" query:"name" gorm:"size:64;not null"`
	Count   int64  `json:"count" gorm:"not null;default:0"`
	Replica string `json:"replica" gorm:"size:191;not null"` // the replica that moved the counter last
}

func (Progress) TableName() string { return "progress" }

func (Progress) Purge() bool { return true }

func (Progress) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Name"}, Unique: true}}
}

func (Progress) Design() {
	Migrate()
	Endpoint("progress")

	List(func() {})
}
