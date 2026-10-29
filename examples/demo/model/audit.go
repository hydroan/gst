package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Audit is one row per change to a record, written by the record hooks,
// and one per login lifecycle moment, a login that succeeded or failed and
// a logout, written by the login observer in module: a table with no API
// of its own, so its design declares Migrate and no action, and no route
// is generated for it. The job in cronjob purges the old rows.
type Audit struct {
	Action   string `json:"action"`
	RecordID string `json:"record_id,omitempty"` // empty for a login or a logout
	Actor    string `json:"actor"`

	model.Base
}

func (Audit) TableName() string { return "audits" }
func (Audit) Purge() bool       { return true } // deleted for good: a log, not something anyone restores

func (Audit) Design() {
	Migrate()
}
