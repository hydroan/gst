// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// all registered jobs on boot. fn is a func() error; every run is logged
// under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00), or a descriptor such as "@hourly" or
// "@every 5m". Pass cronjob.Config{RunImmediately: true} as the optional
// fourth argument to also run the job once at startup.
//
// Example:
//
//	import "github.com/hydroan/gst/cronjob"
//
//	func cleanup() error { return nil }
//
//	func init() {
//		cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup")
//	}
package cronjob

func init() {
	// TODO: register your cron jobs here.
}
