// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// the scheduler once the process is ready to serve and stops it first at
// shutdown. fn is a func(ctx context.Context) error: ctx ends when the
// process begins shutting down or the round's lease is lost, so a long round
// must stop early — one still running 5 seconds after its lease was lost
// fails the process, which exits without waiting for it, since another
// replica may be running the next instant by then — and it carries the
// round's identity — the job name and a trace id of the round's own — and,
// with tracing on, the round's root span, so the statements and log lines
// the job produces are found again from any of them. Every run is logged
// under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00 UTC), or a descriptor such as "@hourly"
// or "@every 5m". Schedules are read in UTC — prefix the expression with
// CRON_TZ=<zone> for another zone — and "@every" runs on the multiples of
// its period from the Unix epoch, so every replica computes the same
// instants. An instant that passes while the previous run is still in flight
// is skipped.
//
// Each instant of a job is claimed once across every replica of the
// deployment: the replicas share the instant's lease through the primary
// database, the first to claim it runs the round, the others skip it. Work
// that belongs to the process itself — refreshing a process-local cache,
// cleaning a local directory — registers with cronjob.RegisterPerInstance and
// runs on every replica.
//
// A round counts once it has run to its end: fn returned nil or an error of
// its own, or panicked. A round cut short — fn returned ctx's ending,
// ctx.Err() or context.Cause(ctx), as the process shut down or the lease was
// lost, or never returned because the process crashed — runs a second time on
// whichever replica finds it first, within about 15 seconds when a shutdown
// cut it short and 30 when a crash did, and so does a round that ran to its
// end but whose end the database failed to record; a second time only, and
// not at all once the job's next instant has started, which gives it up with
// a warning. A failure of fn's own returned beside ctx's ending counts only
// joined with it by errors.Join. fn must therefore be idempotent: it may run
// twice for one instant. On startup a job's most recent instant is caught up
// once, on one replica, when the job has run before, that instant passed
// within the last day and no replica claimed it; a job that has never run, or
// whose most recent instant was claimed, starts with its next instant.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside a job blocks the lease renewal: keep each transaction
// under 8 seconds, and under 5 when transactions run back to back — a longer
// one may end the round with the lease counted as lost, one over 10 seconds
// always does — or register work that only ever runs in one process with
// cronjob.RegisterPerInstance.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/cronjob"
//	)
//
//	func cleanup(ctx context.Context) error { return nil }
//
//	func init() {
//		cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup")
//	}
package cronjob

func init() {
	// TODO: register your cron jobs here.
}
