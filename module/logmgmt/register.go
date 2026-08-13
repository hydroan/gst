package logmgmt

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/cronjob"
	servicelogmgmt "github.com/hydroan/gst/internal/service/logmgmt"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/router"
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
// Operation logs come from the audit pipeline, which stays a deployment
// decision: the module requires AUDIT_ENABLED=true (or the audit.enabled
// config key) and fails startup when it is missing, instead of silently
// flipping the configuration on behalf of the deployment.
func Register() {
	// The check runs on the routes-ready hook: it fires after config loading
	// and module.Wait, the earliest point where both the loaded configuration
	// and the finished registration are visible.
	router.OnRoutesReady(func(map[string][]string) error {
		return requireAuditEnabled()
	})

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

	cronjob.Register(servicelogmgmt.Cleanup, cleanupSchedule(), "cleanup operationlog and loginlog")
}

// cleanupSchedule resolves the cleanup cron expression. Registration happens
// at package initialization, before configuration files are loaded, so the
// schedule is read from the environment key only; the retention itself is
// ordinary configuration read at execution time.
func cleanupSchedule() string {
	if expr := os.Getenv(config.LOGMGMT_CLEANUP_CRON); expr != "" {
		return expr
	}
	return "0 0 * * * *" // hourly
}

// requireAuditEnabled fails startup when the logmgmt module is registered
// without the audit pipeline it depends on: a silently empty operation log
// would look like "no operations happened".
func requireAuditEnabled() error {
	if config.App.Audit.Enabled {
		return nil
	}
	return errors.Newf("module logmgmt requires the audit pipeline: set %s=true or the audit.enabled config key", config.AUDIT_ENABLED)
}
