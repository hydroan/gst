// Package dao holds the writes the example's work makes.
package dao

import (
	"context"
	"time"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst/database"
)

// StartRun records that this replica started the work named name. The write
// runs in a transaction on the work's context, so under a lease it is refused
// once the lease is lost.
func StartRun(ctx context.Context, kind, name string) (*model.Run, error) {
	run := &model.Run{Kind: kind, Name: name, Replica: helper.Replica()}
	err := database.Transaction(ctx, func(ctx context.Context) error {
		return database.Database[*model.Run](ctx).Create(run)
	})
	return run, err
}

// EndRun marks run ended. The write runs in a transaction on the work's
// context: a run whose lease was lost, or whose context ended, keeps no end.
func EndRun(ctx context.Context, run *model.Run) error {
	ended := time.Now().UTC().Truncate(time.Millisecond)
	run.EndedAt = &ended
	return database.Transaction(ctx, func(ctx context.Context) error {
		return database.Database[*model.Run](ctx).Update(run)
	})
}
