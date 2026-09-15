// Package dao holds the work the example's actions run.
package dao

import (
	"context"
	"fmt"
	"time"

	"cluster/helper"
	"cluster/lock"
	"cluster/model"

	"github.com/hydroan/gst/database"
)

// Rebuild runs the rebuild under its lock: it takes the given number of
// seconds, then records one lock event. A rebuild already running — on this
// replica or another — makes the try return lock.ErrHeld at once; a lease
// lost while it runs ends it with lock.ErrLost.
func Rebuild(ctx context.Context, seconds int) error {
	return lock.Rebuild.TryRun(ctx, func(ctx context.Context) error {
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return database.Database[*model.Event](ctx).Create(&model.Event{
			Kind:    "lock",
			Name:    lock.Rebuild.Name(),
			Replica: helper.Replica(),
			Detail:  fmt.Sprintf("rebuilt in %ds", seconds),
		})
	})
}
