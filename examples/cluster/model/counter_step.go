package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CounterStep is one number of the counter the leader work keeps: every second
// the leader appends the next number, in a transaction under its lease. Seq is
// unique, so no number is written twice, and Tenure names the leadership that
// wrote it: the numbers of one tenure form one unbroken run, and the runs
// follow each other, unless two leaderships ever wrote at the same time. The
// counter lives in the database, so a replica taking the leadership over
// continues from the last number.
type CounterStep struct {
	model.Base
	model.Query

	Seq     int64  `json:"seq" query:"seq" gorm:"not null"`
	Tenure  string `json:"tenure" query:"tenure" gorm:"size:32;not null"`    // one id per leadership, drawn as it starts
	Replica string `json:"replica" query:"replica" gorm:"size:191;not null"` // the replica that led, see helper.Replica
}

func (CounterStep) TableName() string { return "counter_steps" }

func (CounterStep) Purge() bool { return true }

func (CounterStep) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Seq"}, Unique: true}}
}

func (CounterStep) Design() {
	Migrate()
	Endpoint("counter_steps")

	List(func() {})
}
