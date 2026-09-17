// Package leader registers the example's leader work.
package leader

import (
	"context"
	"crypto/rand"
	"time"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/leader"
)

func init() {
	leader.Register(count, "counter")
}

// count is the leader work: every second, append the next number of the
// shared counter in a transaction under the lease. Each leadership draws an id
// of its own and writes it with every number, so the counter shows which
// leadership wrote what. A replica that takes the leadership over continues
// from the last number in the database; one that lost the leadership without
// noticing yet has its next transaction refused — the lease is verified first —
// so its numbers never interleave with the new leader's.
func count(ctx context.Context) error {
	tenure := rand.Text()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err := step(ctx, tenure); err != nil {
			return err
		}
	}
}

// step appends the number after the last one, 1 on the first step ever.
func step(ctx context.Context, tenure string) error {
	return database.Transaction(ctx, func(ctx context.Context) error {
		last := make([]*model.CounterStep, 0, 1)
		if err := database.Database[*model.CounterStep](ctx).WithOrder(model.CounterStepCols.Seq.Desc()).WithLimit(1).List(&last); err != nil {
			return err
		}
		next := int64(1)
		if len(last) > 0 {
			next = last[0].Seq + 1
		}
		return database.Database[*model.CounterStep](ctx).Create(&model.CounterStep{Seq: next, Tenure: tenure, Replica: helper.Replica()})
	})
}
