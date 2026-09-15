// Package component registers the application's long-running work: loops
// that run on every replica for the life of the process.
//
// Call component.Register(fn, name) in init below; the process starts fn
// once every table exists and is seeded, right before it starts serving,
// and stops it first at shutdown, before the clients it may use close. fn
// is a func(ctx context.Context) error expected to run until ctx ends: ctx
// ends when the process begins shutting down, and fn must return then —
// shutdown waits for it, for a bounded time. Returning before ctx ends is a
// failure, nil included, and so is a panic: each ends the process with the
// reason, for the orchestrator to restart it.
//
// Work the deployment must do once belongs elsewhere: on a schedule in
// cronjob, as a loop on one replica in leader, on demand under lock.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/component"
//	)
//
//	func consumeEvents(ctx context.Context) error {
//		for {
//			select {
//			case <-ctx.Done():
//				return nil
//			default:
//				// take the next batch and handle it; return on a failure
//			}
//		}
//	}
//
//	func init() {
//		component.Register(consumeEvents, "event-consumer")
//	}
package component

func init() {
	// TODO: register your long-running work here.
}
