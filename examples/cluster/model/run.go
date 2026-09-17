// Package model holds what the cluster example records: the runs the replicas
// make, the steps of the leader's counter, and the rebuild the lock guards.
package model

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Run is one piece of work a replica did: a cron round or a run under the
// lock. It is written as the work starts — CreatedAt is the start — and marked
// ended once the work runs to its end, both in a transaction on the work's
// context: a run cut short, by its lease lost or its process stopping, keeps
// no end. Listing the runs across the replicas shows who did what and how
// often, and whether two runs of one job or one lock ever overlapped.
type Run struct {
	model.Base
	// Query enables the framework's list controls — _page, _size, _sort_by —
	// so a scenario can page through the runs and read the latest first.
	model.Query

	Kind    string     `json:"kind" query:"kind" gorm:"size:16;not null"`        // "cron" or "lock"
	Name    string     `json:"name" query:"name" gorm:"size:64;not null"`        // the job or the lock
	Replica string     `json:"replica" query:"replica" gorm:"size:191;not null"` // the replica that ran it, see helper.Replica
	EndedAt *time.Time `json:"ended_at"`                                         // when the work ran to its end; nil while it runs or once it was cut short
}

func (Run) TableName() string { return "runs" }

func (Run) Purge() bool { return true }

func (Run) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Kind", "Name", "CreatedAt"}}}
}

func (Run) Design() {
	Migrate()
	Endpoint("runs")

	List(func() {})
}
