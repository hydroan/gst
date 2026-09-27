package component

import (
	"context"
	"sync/atomic"
	"time"

	"demo/model"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// recordCount is the count countRecords keeps, read by RecordCount.
var recordCount atomic.Int64

// RecordCount returns how many records the table held when the count was
// last refreshed, a process-local figure a request answers without a query.
func RecordCount() int64 {
	return recordCount.Load()
}

// countRecords refreshes the record count once the process serves and then
// every half minute, until the process shuts down. It has the shape of
// every component: a loop that returns when ctx ends, and only then —
// returning earlier, nil included, fails the process. A count that fails is
// logged and tried again next round.
func countRecords(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		var total int
		if err := database.Database[*model.Record](ctx).Count(&total); err != nil {
			logger.App.Errorz("count records", zap.Error(err))
		} else {
			recordCount.Store(int64(total))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
