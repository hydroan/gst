package servicelogmgmt

import (
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/service"
)

// LoginLogService backs the add-path module registration of the built-in
// CRUD routes. The copy path does not need it: copied model Design generates
// the same built-in routes without a service shell.
type LoginLogService struct {
	service.Base[*modellogmgmt.LoginLog, *modellogmgmt.LoginLog, *modellogmgmt.LoginLog]
}
