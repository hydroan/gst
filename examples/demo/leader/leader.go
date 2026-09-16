// Package leader registers the application's leader work: loops that run on
// exactly one replica of the deployment at a time.
//
// Call leader.Register(fn, name) in init below; every replica campaigns for
// the name once the process is ready to serve, the winner runs fn, and the
// others take over within seconds if it dies. fn is a func(ctx
// context.Context) error expected to run until ctx ends: ctx ends when the
// process begins shutting down or the lease behind the leadership is lost,
// and fn must stop then — one still running 5 seconds after its lease was
// lost fails the process, which exits without waiting for it, since another
// replica may be leading by then. fn runs again from scratch on the replica
// that takes over, so what it must not repeat it keeps in the database. fn
// that returns hands the leadership back, and the campaign resumes after a
// few seconds. Panics are recovered and logged, and every tenure is logged
// under name in leader.log.
//
// Work that runs on a schedule belongs in cronjob instead: a job registered
// there already runs once per instant across the deployment.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside fn blocks the lease renewal: keep each transaction
// under 8 seconds — a longer one may end the work with the lease counted as
// lost, one over 10 seconds always does.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/leader"
//	)
//
//	func relayOutbox(ctx context.Context) error {
//		for {
//			select {
//			case <-ctx.Done():
//				return nil
//			default:
//				// forward the next batch, then wait; return on a failure
//			}
//		}
//	}
//
//	func init() {
//		leader.Register(relayOutbox, "outbox-relay")
//	}
package leader

func init() {
	// TODO: register your leader work here.
}
