package cronjob

import (
	"context"

	"demo/dao"
	"demo/model"

	"github.com/hydroan/gst/database"
)

// The count_records job runs every half minute on every replica: a
// per-instance job refreshes what this process keeps for itself, so each
// replica counts for its own figure, and a missed instant is not caught up.
const (
	countRecordsName = "count_records"
	countRecordsSpec = "@every 30s"
)

// countRecords refreshes the record count the ping answers from.
func countRecords(ctx context.Context) error {
	var total int
	if err := database.Database[*model.Record](ctx).Count(&total); err != nil {
		return err
	}
	dao.SetRecordCount(int64(total))
	return nil
}
