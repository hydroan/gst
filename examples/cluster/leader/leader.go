// Package leader registers the example's leader work.
package leader

import (
	"context"
	"time"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/leader"
)

func init() {
	leader.Register(count, "counter")
}

// count is the leader work: every second, move the shared counter forward
// in a transaction under the lease. A replica that takes the leadership over
// continues from the count in the database, and records that it did; a
// replica that lost the leadership without noticing yet has its next
// transaction refused — the lease is verified first — which ends its tenure
// with "lease lost" in leader.log.
func count(ctx context.Context) error {
	if err := recordTenure(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if err := advance(ctx); err != nil {
			return err
		}
	}
}

// recordTenure writes the event that marks this replica taking the
// leadership over.
func recordTenure(ctx context.Context) error {
	return database.Database[*model.Event](ctx).Create(&model.Event{
		Kind:    "leader",
		Name:    "counter",
		Replica: helper.Replica(),
		Detail:  "took the leadership",
	})
}

// advance moves the counter by one, creating it on the first tenure ever.
func advance(ctx context.Context) error {
	return database.Transaction(ctx, func(ctx context.Context) error {
		rows := make([]*model.Progress, 0, 1)
		if err := database.Database[*model.Progress](ctx).WithQuery(&model.Progress{Name: "counter"}).List(&rows); err != nil {
			return err
		}
		if len(rows) == 0 {
			return database.Database[*model.Progress](ctx).Create(&model.Progress{Name: "counter", Count: 1, Replica: helper.Replica()})
		}
		progress := rows[0]
		progress.Count++
		progress.Replica = helper.Replica()
		return database.Database[*model.Progress](ctx).Update(progress)
	})
}
