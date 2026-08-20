package modellogmgmt

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
)

type OperationLog struct {
	User       string    `json:"user,omitempty" query:"user"`   // the operator, empty for local accounts such as root
	IP         string    `json:"ip,omitempty" query:"ip"`       // the operator's ip
	OP         consts.OP `json:"op,omitempty" query:"op"`       // the action: create, delete, update or read
	Table      string    `json:"table,omitempty" query:"table"` // the table that was operated on
	Model      string    `json:"model,omitempty" query:"model"`
	RecordID   string    `json:"record_id,omitempty" query:"record_id"`     // id of the table record
	RecordName string    `json:"record_name,omitempty" query:"record_name"` // name of the table record
	Record     string    `json:"record,omitempty" query:"record"`           // full content of the record
	Request    string    `json:"request,omitempty" query:"request"`
	Response   string    `json:"response,omitempty" query:"response"`
	OldRecord  string    `json:"old_record,omitempty"` // content before the update
	NewRecord  string    `json:"new_record,omitempty"` // content after the update
	Method     string    `json:"method,omitempty" query:"method"`
	URI        string    `json:"uri,omitempty" query:"uri"` // request uri
	UserAgent  string    `json:"user_agent,omitempty" query:"user_agent"`
	TraceID    string    `json:"trace_id,omitempty" query:"trace_id"`

	model.Base
}

// TableName pins the table name gorm would otherwise derive.
func (OperationLog) TableName() string { return "operation_logs" }

// Purge makes every OperationLog deletion a hard delete: the retention cronjob
// removes expired rows to reclaim space, and a soft-deleted audit trail row
// would defeat that while pretending the log was trimmed.
func (OperationLog) Purge() bool { return true }

func (OperationLog) Design() {
	Migrate()
	// The route matches the add path registration so the copy path generates
	// the same endpoints instead of a diverging default prefix.
	Route("log/operationlog", func() {
		List(func() {
			Enabled(true)
		})
		Get(func() {
			Enabled(true)
		})
	})
}
