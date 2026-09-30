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

	Seq     int64  `json:"seq" query:"seq" gorm:"not null" pb:"11"`
	Tenure  string `json:"tenure" query:"tenure" gorm:"size:32;not null" pb:"12"`    // one id per leadership, drawn as it starts
	Replica string `json:"replica" query:"replica" gorm:"size:191;not null" pb:"13"` // the replica that led, see helper.Replica
}

func (CounterStep) TableName() string { return "counter_steps" }
func (CounterStep) Purge() bool       { return true }
func (CounterStep) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Seq"}, Unique: true}}
}

func (CounterStep) Design() {
	GRPC()
	Migrate()
	Endpoint("counter_steps")

	List(func() {
	})

	// WatchCounterStep streams the numbers as the leader writes them: one
	// request in, numbers out until the client hangs up. Any replica can
	// serve it, the counter being in the database, so a client cut off by a
	// rolling update reconnects to another replica and resumes.
	Route("counter_steps/watch", func() {
		Stream(func() {
			Service("watch")
			Payload[*CounterStepWatchReq]()
			StreamingResult[*CounterStep]()
		})
	})
}

type (
	// CounterStepWatchReq says where a watch starts.
	CounterStepWatchReq struct {
		After int64 `json:"after" pb:"1"` // the numbers after it are streamed, so a client that lost its stream resumes from the last number it saw
	}
)
