package logmgmt

import (
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	servicelogmgmt "github.com/hydroan/gst/internal/moduleservice/logmgmt"
)

var _ gst.Module[*OperationLog, *OperationLog, *OperationLog] = (*OperationLogModule)(nil)

type (
	OperationLog       = modellogmgmt.OperationLog
	OperationLogModule struct{}
)

func (*OperationLogModule) Service() gst.Service[*OperationLog, *OperationLog, *OperationLog] {
	return &servicelogmgmt.OperationLogService{}
}

func (*OperationLogModule) Route() string { return "/log/operationlog" }
func (*OperationLogModule) Public() bool  { return false }
func (*OperationLogModule) Param() string { return "id" }
