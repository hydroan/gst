package component_test

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/component"
	"github.com/hydroan/gst/leader"
	"github.com/hydroan/gst/logger"
)

// ExampleRegister registers a consumer loop the way a project does, from the
// init function of its component package. The loop runs on every replica for
// the life of the process and returns once ctx ends, which happens when the
// process begins shutting down; shutdown waits for it, for a bounded time,
// then leaves it behind.
//
// Registration happens once, in init: a registration made after the process
// started its components panics, and one with no name, a nil function or a
// name already registered fails the process at startup. The loop passes ctx
// to everything it calls, the database included: a context.Background() in
// its place is invisible to shutdown, and gg check rejects one handed to the
// database in the component directory.
func ExampleRegister() {
	// component/component.go:
	//
	//	func init() {
	//		component.Register(consumeRecordEvents, "record-events")
	//	}
	component.Register(func(ctx context.Context) error {
		for {
			events, err := nextRecordEvents(ctx)
			if ctx.Err() != nil {
				// Shutdown began. Returning the ending, or nil, is how the
				// loop stops; neither is reported.
				return ctx.Err()
			}
			if err != nil {
				// Returned before shutdown, an error ends the process, and the
				// orchestrator restarts the replica.
				return errors.Wrap(err, "read record events")
			}
			for _, event := range events {
				if err := handleRecordEvent(ctx, event); err != nil && ctx.Err() == nil {
					logger.App.Warnw("handling a record event failed", "err", err, "record", event.ID)
				}
			}
		}
	}, "record-events")
}

// ExampleRegister_transientFailure waits out a failure the loop can recover
// from instead of returning it. Returning before the process stops — an
// error or nil alike — ends the process, and every request the replica
// serves goes with it; keep that for a failure the loop cannot get past, and
// retry the rest in place, with a wait that watches ctx so that shutdown is
// not held up by the backoff.
func ExampleRegister_transientFailure() {
	component.Register(func(ctx context.Context) error {
		backoff := time.Second
		for {
			events, err := nextRecordEvents(ctx)
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case errors.Is(err, errRecordStreamClosed):
				// No retry brings a closed stream back: end the process.
				return err
			case err != nil:
				logger.App.Warnw("reading record events failed, retrying", "err", err, "backoff", backoff)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				backoff = min(2*backoff, time.Minute)
				continue
			}
			backoff = time.Second
			for _, event := range events {
				if err := handleRecordEvent(ctx, event); err != nil && ctx.Err() == nil {
					logger.App.Warnw("handling a record event failed", "err", err, "record", event.ID)
				}
			}
		}
	}, "record-events")
}

// ExampleRegister_disabled keeps a loop the configuration switches off
// registered and idle. A loop with nothing to do still runs until the process
// stops: returning nil at once reads as a loop that quietly ended, which ends
// the process.
func ExampleRegister_disabled() {
	component.Register(func(ctx context.Context) error {
		if !recordEventsEnabled() {
			<-ctx.Done()
			return ctx.Err()
		}
		for {
			events, err := nextRecordEvents(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return errors.Wrap(err, "read record events")
			}
			for _, event := range events {
				if err := handleRecordEvent(ctx, event); err != nil && ctx.Err() == nil {
					logger.App.Warnw("handling a record event failed", "err", err, "record", event.ID)
				}
			}
		}
	}, "record-events")
}

// ExampleRegister_watch follows a change stream every replica applies to
// itself. Each change is waited for on ctx, so the watch returns the moment
// the process begins shutting down. Work that only has to come round on a
// schedule on every replica — refreshing a process-local cache every minute
// — is simpler as cronjob.RegisterPerInstance.
func ExampleRegister_watch() {
	component.Register(func(ctx context.Context) error {
		for {
			change, err := waitForSettingsChange(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return errors.Wrap(err, "watch settings")
			}
			applySettings(change)
		}
	}, "settings-watch")
}

// ExampleRegister_blockingCall stops a read that takes no context. The read
// would keep the loop blocked past the moment shutdown begins, and shutdown
// waits for the loop only so long before leaving it behind; closing the
// stream when ctx ends unblocks the read, and the loop returns in time.
func ExampleRegister_blockingCall() {
	component.Register(func(ctx context.Context) error {
		stream, err := dialRecordStream()
		if err != nil {
			return errors.Wrap(err, "dial record stream")
		}
		defer func() { _ = stream.Close() }()
		stop := context.AfterFunc(ctx, func() { _ = stream.Close() })
		defer stop()

		for {
			event, err := stream.Read()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return errors.Wrap(err, "read record stream")
			}
			if err := handleRecordEvent(ctx, event); err != nil && ctx.Err() == nil {
				logger.App.Warnw("handling a record event failed", "err", err, "record", event.ID)
			}
		}
	}, "record-stream")
}

// ExampleRegister_onceAcrossTheDeployment splits a flow that is part per
// replica, part once for the deployment. Every replica consumes the record
// events the clients connected to it need; forwarding them to an outside
// system must happen on one replica only. component runs its work on every
// replica and coordinates nothing — two replicas would forward every event
// twice — so the forwarding is leader work.
func ExampleRegister_onceAcrossTheDeployment() {
	// Every replica, for the clients connected to it.
	component.Register(func(ctx context.Context) error {
		for {
			events, err := nextRecordEvents(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return errors.Wrap(err, "read record events")
			}
			publishToConnectedClients(events)
		}
	}, "record-events")

	// One replica at a time, for the outside system.
	leader.Register(forwardRecordEvents, "record-events-forwarder")
}

// The declarations below stand in for the project's own code.

// recordEvent is one event about a record.
type recordEvent struct {
	ID int64
}

// errRecordStreamClosed reports a record stream its source closed for good.
var errRecordStreamClosed = errors.New("record stream closed")

// nextRecordEvents waits for the next batch of record events.
func nextRecordEvents(context.Context) ([]recordEvent, error) { return nil, nil }

// handleRecordEvent applies one record event.
func handleRecordEvent(context.Context, recordEvent) error { return nil }

// publishToConnectedClients pushes record events to the clients connected to
// this replica.
func publishToConnectedClients([]recordEvent) {}

// forwardRecordEvents forwards record events to an outside system until ctx
// ends.
func forwardRecordEvents(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// recordEventsEnabled reports whether the configuration switches record
// events on.
func recordEventsEnabled() bool { return true }

// settingsChange is one change to the settings every replica applies.
type settingsChange struct {
	Key, Value string
}

// waitForSettingsChange waits for the next settings change.
func waitForSettingsChange(context.Context) (settingsChange, error) { return settingsChange{}, nil }

// applySettings applies a settings change to this replica.
func applySettings(settingsChange) {}

// recordStream is a stream of record events whose reads take no context.
type recordStream struct{}

// dialRecordStream opens a record stream.
func dialRecordStream() (*recordStream, error) { return &recordStream{}, nil }

// Read blocks until the next record event arrives or the stream is closed.
func (*recordStream) Read() (recordEvent, error) { return recordEvent{}, nil }

// Close closes the stream, ending a Read in flight.
func (*recordStream) Close() error { return nil }
