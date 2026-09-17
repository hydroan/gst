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

// TestPackageIsOneComponentOfTheLifecycle proves importing the package
// registers the one component that starts and stops every registered work,
// and that Register lists the work under its name.
func TestPackageIsOneComponentOfTheLifecycle(t *testing.T) {
	withRegistry(t)

	var names []string
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		names = append(names, c.Name)
	}
	require.Contains(t, names, "component")

	Register(idle, " sample ")
	require.Len(t, works, 1)
	require.Equal(t, "sample", works[0].name, "the name is trimmed")
}

// TestRegisterRefusesWhatItCannotHonor proves a registration without a
// name, without a function or under a name already taken fails the start
// naming every one of them, and a registration after the start panics.
func TestRegisterRefusesWhatItCannotHonor(t *testing.T) {
	withRegistry(t)

	Register(nil, "sample-nil")
	Register(idle, "  ")
	Register(idle, "sample")
	Register(idle, "sample")
	err := start(context.Background())
	require.ErrorContains(t, err, `component "sample-nil": nil function`)
	require.ErrorContains(t, err, "registered work has no name")
	require.ErrorContains(t, err, `component "sample": registered twice`)

	withRegistry(t)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, start(ctx))
	require.Panics(t, func() { Register(idle, "late") }, "a registration after the start would never run")
	cancel()
	require.NoError(t, stop(context.Background()))
}

// TestStartRunsEveryWorkAndStopWaitsForAll proves the start runs every
// registered work on the context it is given and stop returns once all of
// them have — with nothing reported as a failure.
func TestStartRunsEveryWorkAndStopWaitsForAll(t *testing.T) {
	withRegistry(t)
	failures := withRecordedFailures(t)
	returned := make(chan string, 2)
	for _, name := range []string{"first", "second"} {
		Register(func(ctx context.Context) error {
			<-ctx.Done()
			returned <- name
			return ctx.Err()
		}, name)
	}
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, start(ctx))

	select {
	case name := <-returned:
		t.Fatalf("%q must run until the process stops", name)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	require.NoError(t, stop(context.Background()))
	require.Len(t, returned, 2, "stop returns once every work has")
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
			w.start(context.Background())

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
			w.start(ctx)
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
	w.start(ctx)
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopCancel()
	require.ErrorContains(t, w.stop(stopCtx), "has not returned")

	close(release)
	require.NoError(t, w.stop(context.Background()))
	require.Empty(t, failures, "returning once the process stops is not a failure, however late")
}

// TestStopReportsNothingOnceTheWorkReturned proves stop tells work that
// returned from work it gave up on even when its window has already ended —
// used up by the components stopped before it, or never given because the
// process must not wait: work that returned is never reported as given up
// on.
func TestStopReportsNothingOnceTheWorkReturned(t *testing.T) {
	w := &work{name: "sample", done: make(chan struct{})}
	close(w.done)
	ended, cancel := context.WithCancel(context.Background())
	cancel()

	for range 100 {
		require.NoError(t, w.stop(ended), "work that returned must not be reported as given up on")
	}
}

// idle is work that runs until the process stops.
func idle(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// withRegistry gives the test a registry of its own and restores the
// process's afterwards.
func withRegistry(t *testing.T) {
	t.Helper()

	mu.Lock()
	savedWorks, savedErr, savedStarted := works, errRegister, started
	works, errRegister, started = nil, nil, false
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		works, errRegister, started = savedWorks, savedErr, savedStarted
		mu.Unlock()
	})
}

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
