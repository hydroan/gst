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

func (OperationLog) Design() {
	Migrate()
	List(func() {
		Enabled(true)
	})
	Get(func() {
		Enabled(true)
	})
}
