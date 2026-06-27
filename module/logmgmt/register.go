package logmgmt

import (
	"os"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/cronjob"
	cronjoblogmgmt "github.com/hydroan/gst/internal/cronjob/logmgmt"
	servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers two modules: LoginLog and OperationLog.
//
// Models:
//   - LoginLog
//   - OperationLog
//
// Routes:
//   - GET /api/log/loginlog
//   - GET /api/log/loginlog/:id
//   - GET /api/log/operationlog
//   - GET /api/log/operationlog/:id
//
// Cronjob:
//   - cleanup operationlog and loginlog hourly.
//
// Enable Audit to records all operation logs.
func Register() {
	servicelogmgmt.Enabled = true

	// enable audit function to records the operation logs.
	os.Setenv(config.AUDIT_ENABLE, "true")

	module.Use[*LoginLog,
		*LoginLog,
		*LoginLog](
		&LoginLogModule{},
		module.CRUD(
			consts.PHASE_LIST,
			consts.PHASE_GET,
		),
	)

	module.Use[
		*OperationLog,
		*OperationLog,
		*OperationLog](
		&OperationLogModule{},
		module.CRUD(
			consts.PHASE_LIST,
			consts.PHASE_GET,
		),
	)

	cronjob.Register(cronjoblogmgmt.Cleanup, "0 0 * * * *", "cleanup operationlog and loginlog hourly")
}
