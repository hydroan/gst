package component

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// withRecordedFailures records the process failures the work reports
// instead of ending the process, and restores the real reporting afterwards.
func withRecordedFailures(t *testing.T) <-chan error {
	t.Helper()

	failures := make(chan error, 4)
	original := fail
	fail = func(err error) { failures <- err }
	t.Cleanup(func() { fail = original })
	return failures
}

// withObservedGlobalLogger routes the global logger into an observer for the
// test and restores the previous one afterwards.
func withObservedGlobalLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()

	core, logs := observer.New(zapcore.WarnLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(restore)
	return logs
}

// TestRegisterDeclaresTheWorkAsAComponent proves Register puts the work in
// the component stage of the lifecycle under its name, prefixed so that it
// cannot collide with a framework component's, and refuses a nil function.
func TestRegisterDeclaresTheWorkAsAComponent(t *testing.T) {
	Register(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}, "sample")

	var names []string
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		names = append(names, c.Name)
	}
	require.Contains(t, names, "component:sample")
	require.Panics(t, func() { Register(nil, "sample-nil") })
}

// TestWorkRunsUntilTheProcessStopsAndIsWaitedFor proves the work runs on the
// context it is started on until that context ends, and stop returns once
// the work has — with nothing reported as a failure.
func TestWorkRunsUntilTheProcessStopsAndIsWaitedFor(t *testing.T) {
	failures := withRecordedFailures(t)
	returned := make(chan struct{})
	w := &work{name: "sample", fn: func(ctx context.Context) error {
		<-ctx.Done()
		close(returned)
		return ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.start(ctx))

	select {
	case <-returned:
		t.Fatal("the work must run until the process stops")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	require.NoError(t, w.stop(context.Background()))
	<-returned
	require.Empty(t, failures, "returning once the process stops is not a failure")
}

// TestWorkThatEndsBeforeTheProcessFailsIt proves a return before the context
// has ended — nil, an error or a panic — is reported as a failure that names
// the work and carries the reason.
func TestWorkThatEndsBeforeTheProcessFailsIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(context.Context) error
		want string
	}{
		{name: "returns nothing", fn: func(context.Context) error { return nil }, want: "returned before the process began to stop"},
		{name: "returns an error", fn: func(context.Context) error { return errors.New("sample failure") }, want: "sample failure"},
		{name: "panics", fn: func(context.Context) error { panic("sample panic") }, want: "sample panic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := withRecordedFailures(t)
			w := &work{name: "sample", fn: tc.fn}
			require.NoError(t, w.start(context.Background()))

			select {
			case err := <-failures:
				require.ErrorContains(t, err, `component "sample"`)
				require.ErrorContains(t, err, tc.want)
			case <-time.After(5 * time.Second):
				t.Fatal("the failure must be reported")
			}
			require.NoError(t, w.stop(context.Background()))
		})
	}
}

// TestWorkFailingWhileStoppingIsLoggedNotFatal proves a failure of the work's
// own after the process began to stop is logged, not reported as a failure
// — the process ends either way — while the cancellation it was asked to
// return is not even logged.
func TestWorkFailingWhileStoppingIsLoggedNotFatal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    func(ctx context.Context) error
		logged bool
	}{
		{name: "the cancellation", err: func(ctx context.Context) error { return errors.Wrap(ctx.Err(), "poll") }, logged: false},
		{name: "a failure of its own", err: func(context.Context) error { return errors.New("sample failure") }, logged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := withRecordedFailures(t)
			logs := withObservedGlobalLogger(t)
			w := &work{name: "sample", fn: func(ctx context.Context) error {
				<-ctx.Done()
				return tc.err(ctx)
			}}
			ctx, cancel := context.WithCancel(context.Background())
			require.NoError(t, w.start(ctx))
			cancel()
			require.NoError(t, w.stop(context.Background()))

			require.Empty(t, failures)
			entries := logs.FilterMessage("component ended with an error while stopping").All()
			if tc.logged {
				require.Len(t, entries, 1)
			} else {
				require.Empty(t, entries)
			}
		})
	}
}

// TestStopGivesUpOnWorkThatWillNotReturn proves stop returns with an error
// once its own context ends while the work runs on, so shutdown goes on
// without it.
func TestStopGivesUpOnWorkThatWillNotReturn(t *testing.T) {
	failures := withRecordedFailures(t)
	release := make(chan struct{})
	w := &work{name: "sample", fn: func(context.Context) error {
		<-release
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.start(ctx))
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopCancel()
	require.ErrorContains(t, w.stop(stopCtx), "has not returned")

	close(release)
	require.NoError(t, w.stop(context.Background()))
	require.Empty(t, failures, "returning once the process stops is not a failure, however late")
}
