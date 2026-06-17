package servicelogmgmt

import (
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/service"
)

var Enabled bool

type LoginLogService struct {
	service.Base[*modellogmgmt.LoginLog, *modellogmgmt.LoginLog, *modellogmgmt.LoginLog]
}
