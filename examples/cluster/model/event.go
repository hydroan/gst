// Package model declares what the example keeps in the database: the events
// the replicas record, the counter the leader work moves, and the rebuild
// action a client triggers.
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Event is one thing a replica did: a cron round, a leader tenure, a run
// under the lock. Listing the events across the replicas shows who did what
// and how often, which is what the scenarios in the README read.
type Event struct {
	model.Base
	// Query enables the framework's list controls — _page, _size, _sort_by —
	// so a scenario can page through the events and read the latest first.
	model.Query

	Kind    string `json:"kind" query:"kind" gorm:"size:16;not null"`        // "cron", "leader" or "lock"
	Name    string `json:"name" query:"name" gorm:"size:64;not null"`        // the job, the leader work or the lock
	Replica string `json:"replica" query:"replica" gorm:"size:191;not null"` // the replica that did it, see helper.Replica
	Detail  string `json:"detail" gorm:"size:191;not null"`
}

func (Event) TableName() string { return "events" }

func (Event) Purge() bool { return true }

func (Event) Design() {
	Migrate()
	Endpoint("events")

	List(func() {})
}
