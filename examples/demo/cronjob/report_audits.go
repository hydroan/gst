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

// reportAudits logs how many audit rows the last day brought, by action. The
// database counts them: database.Select projects the action as the group
// key and a count beside it, so one row per action comes back, not the
// day's rows.
func reportAudits(ctx context.Context) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	type auditsOfAction struct {
		Action string
		Total  int
	}
	rows := make([]auditsOfAction, 0)
	if err := database.Select[*model.Audit, auditsOfAction](ctx, model.AuditCols.Action.Group(), gst.Count().As("total")).
		Where(model.AuditCols.CreatedAt.Gte(since)).
		Scan(&rows); err != nil {
		return err
	}
	digest := make(map[string]int, len(rows))
	for _, row := range rows {
		digest[row.Action] = row.Total
	}
	logger.App.Infoz("audit digest", zap.Any("actions", digest))
	return nil
}
