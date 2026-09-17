// Package cronjob registers the example's scheduled jobs: one that runs once
// per instant across the deployment, one that runs on every replica, and one
// that runs longer than its lease lasts.
package cronjob

import (
	"context"
	"time"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst/cronjob"
	"github.com/hydroan/gst/database"
)

func init() {
	cronjob.Register(tick, "@every 10s", "tick")
	cronjob.RegisterPerInstance(localTick, "@every 10s", "local-tick")
	cronjob.Register(slow, "@every 30s", "slow")
}

// tick runs once per instant across the deployment: the event list shows
// one row every 10 seconds however many replicas run, each row naming the
// replica that won the instant.
func tick(ctx context.Context) error {
	return record(ctx, "tick", "once per instant across the deployment")
}

// localTick runs on every replica: the event list shows one row every 10
// seconds per replica.
func localTick(ctx context.Context) error {
	return record(ctx, "local-tick", "on every replica")
}

// slow runs longer than the 15 seconds a lease lasts, so the round keeps the
// lease alive by renewing it: expires_at_ms of cron:slow in gst_leases keeps
// moving while the round runs, and no other replica can take the name over
// meanwhile. Its period is longer than a round, so no instant passes while
// one runs; a job that overran its period would have the instants that
// passed skipped, with a warning naming how many. A round whose pod is
// deleted halfway returns its context's ending, so it has not run to its
// end, and another replica runs it a second time for the same instant.
func slow(ctx context.Context) error {
	select {
	case <-time.After(20 * time.Second):
		return record(ctx, "slow", "ran for 20s, longer than the lease")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// record writes the event of one round.
func record(ctx context.Context, name, detail string) error {
	return database.Database[*model.Event](ctx).Create(&model.Event{
		Kind:    "cron",
		Name:    name,
		Replica: helper.Replica(),
		Detail:  detail,
	})
}
