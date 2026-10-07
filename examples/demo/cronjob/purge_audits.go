package cronjob

import (
	"context"
	"time"

	"demo/configx"
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
)

// The purge_audits job runs at 03:00 UTC every day: a 6-field cron
// expression, seconds first, read in UTC.
const (
	purgeAuditsName = "purge_audits"
	purgeAuditsSpec = "0 0 3 * * *"
)

// purgeAudits deletes the audit rows older than the cleanup section keeps,
// at most a batch of them per round: the rest go the next day.
func purgeAudits(ctx context.Context) error {
	cleanup := config.Get[configx.Cleanup]()
	cutoff := time.Now().UTC().AddDate(0, 0, -cleanup.AuditDays)
	stale := make([]*model.Audit, 0)
	if err := database.Database[*model.Audit](ctx).
		WithQuery(&model.Audit{}, gst.QueryOptions{Filters: []gst.Filter{model.AuditCols.CreatedAt.Lt(cutoff)}}).
		WithLimit(cleanup.BatchSize).
		List(&stale); err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	// A row another round removed since the listing is already gone, which
	// is what this round wants.
	return database.Database[*model.Audit](ctx).WithAllowMissing().Delete(stale...)
}
