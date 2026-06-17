package modellogmgmt

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
)

type OperationLog struct {
	User       string    `json:"user,omitempty" schema:"user"`   // 操作者, 本地账号该字段为空,例如 root
	IP         string    `json:"ip,omitempty" schema:"ip"`       // 操作者的 ip
	OP         consts.OP `json:"op,omitempty" schema:"op"`       // 动作: 增删改查
	Table      string    `json:"table,omitempty" schema:"table"` // 操作了哪张表
	Model      string    `json:"model,omitempty" schema:"model"`
	RecordID   string    `json:"record_id,omitempty" schema:"record_id"`     // 表记录的 id
	RecordName string    `json:"record_name,omitempty" schema:"record_name"` // 表记录的 name
	Record     string    `json:"record,omitempty" schema:"record"`           // 记录全部内容
	Request    string    `json:"request,omitempty" schema:"request"`
	Response   string    `json:"response,omitempty" schema:"response"`
	OldRecord  string    `json:"old_record,omitempty"` // 更新前的内容
	NewRecord  string    `json:"new_record,omitempty"` // 更新后的内容
	Method     string    `json:"method,omitempty" schema:"method"`
	URI        string    `json:"uri,omitempty" schema:"uri"` // request uri
	UserAgent  string    `json:"user_agent,omitempty" schema:"user_agent"`
	RequestID  string    `json:"request_id,omitempty" schema:"request_id"`

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
