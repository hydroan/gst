package cronjoblogmgmt

import (
	"context"
	"time"

	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
)

// Cleanup will delete logs older than 3 months
func Cleanup() error {
	end := time.Now().Add(-3 * 30 * 24 * time.Hour)
	// Filters count as a real condition, so the empty-query safety check stays
	// out of the way without opting in to AllowEmpty.
	expired := types.QueryOptions{Filters: []types.Filter{types.FilterLte("created_at", end)}}

	oplogs := make([]*modellogmgmt.OperationLog, 0)
	if err := database.Database[*modellogmgmt.OperationLog](context.Background()).WithQuery(nil, expired).List(&oplogs); err != nil {
		logger.Cronjob.Error(err)
	}
	if err := database.Database[*modellogmgmt.OperationLog](context.Background()).WithPurge().Delete(oplogs...); err != nil {
		logger.Cronjob.Error(err)
	}

	loginLogs := make([]*modellogmgmt.LoginLog, 0)
	if err := database.Database[*modellogmgmt.LoginLog](context.Background()).WithQuery(nil, expired).List(&loginLogs); err != nil {
		logger.Cronjob.Error(err)
	}
	if err := database.Database[*modellogmgmt.LoginLog](context.Background()).WithPurge().Delete(loginLogs...); err != nil {
		logger.Cronjob.Error(err)
	}

	return nil
}
