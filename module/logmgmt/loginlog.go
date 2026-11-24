package logmgmt

import (
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	servicelogmgmt "github.com/hydroan/gst/internal/moduleservice/logmgmt"
)

var _ gst.Module[*LoginLog, *LoginLog, *LoginLog] = (*LoginLogModule)(nil)

type (
	LoginStatus = modellogmgmt.LoginStatus

	LoginLog       = modellogmgmt.LoginLog
	LoginLogModule struct{}
)

const (
	LoginStatusSuccess = modellogmgmt.LoginStatusSuccess
	LoginStatusFailure = modellogmgmt.LoginStatusFailure
	LoginStatusLogout  = modellogmgmt.LoginStatusLogout
)

func (*LoginLogModule) Service() gst.Service[*LoginLog, *LoginLog, *LoginLog] {
	return &servicelogmgmt.LoginLogService{}
}
func (*LoginLogModule) Route() string { return "/log/loginlog" }
func (*LoginLogModule) Pub() bool     { return false }
func (*LoginLogModule) Param() string { return "id" }
