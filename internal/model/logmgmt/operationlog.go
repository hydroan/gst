package modellogmgmt

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
)

type OperationLog struct {
	User       string    `json:"user,omitempty" query:"user"`   // 操作者, 本地账号该字段为空,例如 root
	IP         string    `json:"ip,omitempty" query:"ip"`       // 操作者的 ip
	OP         consts.OP `json:"op,omitempty" query:"op"`       // 动作: 增删改查
	Table      string    `json:"table,omitempty" query:"table"` // 操作了哪张表
	Model      string    `json:"model,omitempty" query:"model"`
	RecordID   string    `json:"record_id,omitempty" query:"record_id"`     // 表记录的 id
	RecordName string    `json:"record_name,omitempty" query:"record_name"` // 表记录的 name
	Record     string    `json:"record,omitempty" query:"record"`           // 记录全部内容
	Request    string    `json:"request,omitempty" query:"request"`
	Response   string    `json:"response,omitempty" query:"response"`
	OldRecord  string    `json:"old_record,omitempty"` // 更新前的内容
	NewRecord  string    `json:"new_record,omitempty"` // 更新后的内容
	Method     string    `json:"method,omitempty" query:"method"`
	URI        string    `json:"uri,omitempty" query:"uri"` // request uri
	UserAgent  string    `json:"user_agent,omitempty" query:"user_agent"`
	TraceID    string    `json:"trace_id,omitempty" query:"trace_id"`

	model.Base
}

func (OperationLog) Design() {
	Migrate(true)
	List(func() {
		Enabled(true)
	})
	Get(func() {
		Enabled(true)
	})
}
