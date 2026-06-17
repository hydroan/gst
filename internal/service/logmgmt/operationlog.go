package servicelogmgmt

import (
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/service"
)

type OperationLogService struct {
	service.Base[*modellogmgmt.OperationLog, *modellogmgmt.OperationLog, *modellogmgmt.OperationLog]
}
