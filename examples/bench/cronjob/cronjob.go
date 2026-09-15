// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// the scheduler once the process is ready to serve and stops it first at
// shutdown. fn is a func(ctx context.Context) error: ctx ends when the
// process begins shutting down, so a long round can stop early, and it
// carries the round's identity — the job name and a trace id of the round's
// own — and, with tracing on, the round's root span, so the statements and
// log lines the job produces are found again from any of them. Every run is
// logged under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00 UTC), or a descriptor such as "@hourly"
// or "@every 5m". Schedules are read in UTC — prefix the expression with
// CRON_TZ=<zone> for another zone — and "@every" runs on the multiples of
// its period from the Unix epoch, so every replica computes the same
// instants. An instant that passes while the previous run is still in flight
// is skipped.
//
// A job runs once per instant across every replica of the deployment: the
// replicas share the instant's lease through the primary database, the first
// to claim it runs the round, the others skip it. Work that belongs to the
// process itself — refreshing a process-local cache, cleaning a local
// directory — registers with cronjob.RegisterPerInstance and runs on every
// replica. On startup a job's most recent instant is caught up once, on one
// replica, when the job has run before, that instant passed within the last
// day and no replica ran it; a job that has never run, or whose most recent
// instant was run, starts with its next instant.
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
