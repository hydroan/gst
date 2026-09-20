// Package cronjob registers the example's scheduled jobs: one whose instants
// are each claimed once across the deployment, one that runs on every
// replica, and one that runs longer than its lease lasts.
package cronjob

import (
	"context"
	"time"

	"cluster/configx"
	"cluster/dao"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/cronjob"
)

func init() {
	cronjob.Register(tick, "@every 10s", "tick")
	cronjob.RegisterPerInstance(localTick, "@every 10s", "local-tick")
	cronjob.Register(slow, "@every 30s", "slow")
}

// tick has each instant claimed once across the deployment: the runs show one
// round every 10 seconds however many replicas run, each naming the replica
// that won the instant.
func tick(ctx context.Context) error {
	return round(ctx, "tick", nil)
}

// localTick runs on every replica: the runs show one round every 10 seconds
// per replica.
func localTick(ctx context.Context) error {
	return round(ctx, "local-tick", nil)
}

// slow runs longer than the 15 seconds a lease lasts, so the round keeps the
// lease alive by renewing it: expires_at_ms of cron:slow in gst_leases keeps
// moving while the round runs, and no other replica can take the name over
// meanwhile. Its period is longer than a round, so no instant passes while
// one runs while it takes the 20 seconds it takes by default; raise
// JOBS_SLOW_SECONDS past the period and the instants that pass while a round
// runs are skipped, with a warning while the round runs and another naming
// how many it skipped once it returns. A round cut short — its pod
// deleted or its process killed halfway — returns its context's ending or
// never returns, so it has not run to its end, and another replica runs it a
// second time for the same instant.
func slow(ctx context.Context) error {
	return round(ctx, "slow", func(ctx context.Context) error {
		select {
		case <-time.After(time.Duration(config.Get[configx.Jobs]().SlowSeconds) * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// round records one round of the job named name around work, nil for a round
// with nothing to do but be recorded: the run is written before the work and
// marked ended once the work returns nil. The error is returned as it came,
// so a round cut short reads as cut short.
func round(ctx context.Context, name string, work func(ctx context.Context) error) error {
	run, err := dao.StartRun(ctx, "cron", name)
	if err != nil {
		return err
	}
	if work != nil {
		if err := work(ctx); err != nil {
			return err
		}
	}
	return dao.EndRun(ctx, run)
}
