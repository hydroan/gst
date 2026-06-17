package logmgmt

import (
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*OperationLog, *OperationLog, *OperationLog] = (*OperationLogModule)(nil)

type (
	OperationLog       = modellogmgmt.OperationLog
	OperationLogModule struct{}
)

func (*OperationLogModule) Service() types.Service[*OperationLog, *OperationLog, *OperationLog] {
	return &servicelogmgmt.OperationLogService{}
}

func (*OperationLogModule) Route() string { return "/log/operationlog" }
func (*OperationLogModule) Pub() bool     { return false }
func (*OperationLogModule) Param() string { return "id" }
