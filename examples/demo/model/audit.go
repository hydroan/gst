package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Audit is one row per change to a record, written by the record hooks: a
// table with no API of its own, so its design declares Migrate and no
// action, and no route is generated for it. The job in cronjob purges the
// old rows.
type Audit struct {
	Action   string `json:"action"`
	RecordID string `json:"record_id"`
	Actor    string `json:"actor"`

	model.Base
}

func (Audit) TableName() string { return "audits" }

// Purge deletes audit rows for good: they are a log, not something anyone
// restores.
func (Audit) Purge() bool { return true }

func (Audit) Design() {
	Migrate()
}
