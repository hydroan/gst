// Package leader registers the example's leader work.
package leader

import (
	"context"
	"crypto/rand"
	"time"

	"cluster/helper"
	"cluster/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/leader"
)

func init() {
	leader.Register(count, "counter")
}

// stepDown carries the request to end the leadership from the outside, one
// at a time; see StepDown.
var stepDown = make(chan struct{}, 1)

// ErrSteppedDown is what the work returns when it was asked to step down: a
// failure of its own, not the ending of its context, which is what makes the
// framework hand the name back and campaign for it again.
var ErrSteppedDown = errors.New("the leader work was asked to step down")

// StepDown asks the leader work running on this replica to return, and
// reports whether the request was taken; a second request while one is
// pending is not. A replica that is not the leader takes the request and
// nothing happens.
func StepDown() bool {
	select {
	case stepDown <- struct{}{}:
		return true
	default:
		return false
	}
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
		case <-stepDown:
			return ErrSteppedDown
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
