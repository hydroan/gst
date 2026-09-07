// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// all registered jobs on boot. fn is a func(ctx context.Context) error: ctx
// carries the round's identity — the job name and a trace id of the round's
// own — and, with tracing on, the round's root span, so the statements and
// log lines the job produces are found again from any of them. Every run is
// logged under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00), or a descriptor such as "@hourly" or
// "@every 5m". Pass cronjob.Config{RunImmediately: true} as the optional
// fourth argument to also run the job once at startup.
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
