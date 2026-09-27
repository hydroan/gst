package cronjob

import (
	"context"
	"time"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// The report_audits job runs at 08:00 UTC every day.
const (
	reportAuditsName = "report_audits"
	reportAuditsSpec = "0 0 8 * * *"
)

// reportAudits logs how many audit rows the last day brought, by action.
func reportAudits(ctx context.Context) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	audits := make([]*model.Audit, 0)
	if err := database.Database[*model.Audit](ctx).
		WithQuery(&model.Audit{}, gst.QueryOptions{Filters: []gst.Filter{model.AuditCols.CreatedAt.Gte(since)}}).
		List(&audits); err != nil {
		return err
	}
	digest := make(map[string]int)
	for _, audit := range audits {
		digest[audit.Action]++
	}
	logger.App.Infoz("audit digest", zap.Any("actions", digest))
	return nil
}
