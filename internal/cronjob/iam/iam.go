package cronjobiam

import (
	"context"
	"time"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/logger"
)

// CleanupOnlineUser cleanups the online user that not active for 1 minute.
func CleanupOnlineUser() error {
	end := time.Now().Add(-1 * time.Minute)
	ous := make([]*modeliamsession.OnlineUser, 0)

	if err := database.Database[*modeliamsession.OnlineUser](context.Background()).WithTimeRange("updated_at", time.Time{}, end).List(&ous); err != nil {
		logger.Cronjob.Error(err)
	}
	if err := database.Database[*modeliamsession.OnlineUser](context.Background()).WithPurge().Delete(ous...); err != nil {
		logger.Cronjob.Error(err)
	}
	return nil
}
