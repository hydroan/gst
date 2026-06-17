package cronjoblogmgmt

import (
	"time"

	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/logger"
)

// Cleanup will delete logs older than 3 months
func Cleanup() error {
	end := time.Now().Add(-3 * 30 * 24 * time.Hour)
	oplogs := make([]*modellogmgmt.OperationLog, 0)

	if err := database.Database[*modellogmgmt.OperationLog](nil).WithTimeRange("created_at", time.Time{}, end).List(&oplogs); err != nil {
		logger.Cronjob.Error(err)
	}
	if err := database.Database[*modellogmgmt.OperationLog](nil).WithPurge().Delete(oplogs...); err != nil {
		logger.Cronjob.Error(err)
	}

	loginLogs := make([]*modellogmgmt.LoginLog, 0)
	if err := database.Database[*modellogmgmt.LoginLog](nil).WithTimeRange("created_at", time.Time{}, end).List(&loginLogs); err != nil {
		logger.Cronjob.Error(err)
	}
	if err := database.Database[*modellogmgmt.LoginLog](nil).WithPurge().Delete(loginLogs...); err != nil {
		logger.Cronjob.Error(err)
	}

	return nil
}
