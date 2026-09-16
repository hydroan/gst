package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/types"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// TestStagesStartInTurnAndStopInReverse proves the two stages come up in the
// order bootstrap drives them — the providers first, then the components —
// and go down in the opposite one, the components first and the providers
// after their last user, whatever the names say.
func TestStagesStartInTurnAndStopInReverse(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("a-worker", StageComponent, &events, nil))
	Register(recordingComponent("z-client", StageProvider, &events, nil))

	require.NoError(t, Start(context.Background(), StageProvider))
	require.Equal(t, []string{"start z-client"}, events)
	require.NoError(t, Start(context.Background(), StageComponent))
	require.Equal(t, []string{"start z-client", "start a-worker"}, events)

	Stop(context.Background())
	require.Equal(t, []string{"start z-client", "start a-worker", "stop a-worker", "stop z-client"}, events)
}

// TestStartOrdersAStageByName proves the members of one stage start in name
// order regardless of registration order, so bootstrap is reproducible
// across builds.
func TestStartOrdersAStageByName(t *testing.T) {
	resetRegistry(t)

	var events []string
	for _, name := range []string{"second", "third", "first"} {
		Register(recordingComponent(name, StageComponent, &events, nil))
	}

	require.NoError(t, Start(context.Background(), StageComponent))
	require.Equal(t, []string{"start first", "start second", "start third"}, events)
}

// TestStartLeavesTheOtherStageAlone proves starting one stage neither starts
// nor seals the other: the components registered for it start when their
// own stage does.
func TestStartLeavesTheOtherStageAlone(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("client", StageProvider, &events, nil))

	require.NoError(t, Start(context.Background(), StageProvider))
	Register(recordingComponent("worker", StageComponent, &events, nil))
	require.NoError(t, Start(context.Background(), StageComponent))
	require.Equal(t, []string{"start client", "start worker"}, events)
}

// TestDisabledComponentIsLeftOutOfTheLifecycle proves a component whose
// Enabled reports false neither starts nor stops, while one without an
// Enabled function counts as enabled.
func TestDisabledComponentIsLeftOutOfTheLifecycle(t *testing.T) {
	resetRegistry(t)

	var events []string
	off := recordingComponent("off", StageComponent, &events, nil)
	off.Enabled = func() bool { return false }
	Register(off)
	Register(recordingComponent("on", StageComponent, &events, nil))

	require.NoError(t, Start(context.Background(), StageComponent))
	Stop(context.Background())
	require.Equal(t, []string{"start on", "stop on"}, events)
}

// TestSetLoggerReceivesADedicatedLoggerBeforeStart proves every registered
// component of a stage that declared SetLogger — enabled or not — is bound
// to a logger writing <Name>.log before the stage's first Start runs, and
// that the file exists.
func TestSetLoggerReceivesADedicatedLoggerBeforeStart(t *testing.T) {
	resetRegistry(t)
	dir := withLoggerConfig(t)

	var sampleLogger, offLogger types.Logger
	boundBeforeStart := false
	Register(Component{
		Name:      "sample",
		Stage:     StageProvider,
		SetLogger: func(l types.Logger) { sampleLogger = l },
		Start: func(context.Context) error {
			boundBeforeStart = sampleLogger != nil
			return nil
		},
	})
	Register(Component{
		Name:      "off",
		Stage:     StageProvider,
		Enabled:   func() bool { return false },
		SetLogger: func(l types.Logger) { offLogger = l },
		Start:     func(context.Context) error { return errors.New("must not start") },
	})

	require.NoError(t, Start(context.Background(), StageProvider))
	require.True(t, boundBeforeStart, "the dedicated logger must be bound before Start runs")
	require.NotNil(t, offLogger, "a disabled component keeps its dedicated logger binding")
	// Sink construction precreates the file, so its existence proves the
	// binding points at the component's own file.
	require.FileExists(t, filepath.Join(dir, "sample.log"))
	require.FileExists(t, filepath.Join(dir, "off.log"))
}

// TestProviderStartContextEndsWithStart proves the two stages get the
// contexts they are promised: a provider's is canceled as soon as its Start
// returns, a component keeps the process context for its lifetime.
func TestProviderStartContextEndsWithStart(t *testing.T) {
	resetRegistry(t)

	var providerDone, componentDone <-chan struct{}
	Register(Component{Name: "client", Stage: StageProvider, Start: func(ctx context.Context) error {
		providerDone = ctx.Done()
		return nil
	}})
	Register(Component{Name: "worker", Stage: StageComponent, Start: func(ctx context.Context) error {
		componentDone = ctx.Done()
		return nil
	}})

	processCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, Start(processCtx, StageProvider))
	require.NoError(t, Start(processCtx, StageComponent))
	select {
	case <-providerDone:
	default:
		t.Fatal("a provider's context must end with its Start")
	}
	select {
	case <-componentDone:
		t.Fatal("a component must keep the process context")
	default:
	}

	cancel()
	<-componentDone
}

// TestStartStopsAtTheFirstFailure proves a component that fails to start
// halts its stage: the ones after it never start, the ones before it keep
// running and are the only ones Stop stops.
func TestStartStopsAtTheFirstFailure(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("a", StageComponent, &events, nil))
	Register(recordingComponent("b-failing", StageComponent, &events, errors.New("sample failure")))
	Register(recordingComponent("c", StageComponent, &events, nil))

	err := Start(context.Background(), StageComponent)
	require.ErrorContains(t, err, `failed to start component "b-failing"`)
	require.ErrorContains(t, err, "sample failure")
	require.Equal(t, []string{"start a", "start b-failing"}, events)

	Stop(context.Background())
	require.Equal(t, []string{"start a", "start b-failing", "stop a"}, events)
}

// TestStopFailureDoesNotStopTheOthers proves a Stop that returns an error is
// logged and the remaining components still stop.
func TestStopFailureDoesNotStopTheOthers(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("a", StageComponent, &events, nil))
	failing := recordingComponent("b", StageComponent, &events, nil)
	failing.Stop = func(context.Context) error {
		events = append(events, "stop b")
		return errors.New("sample stop failure")
	}
	Register(failing)

	require.NoError(t, Start(context.Background(), StageComponent))
	Stop(context.Background())
	require.Equal(t, []string{"start a", "start b", "stop b", "stop a"}, events)
}

// TestStopDoesNotWaitForComponentsOnceTheProcessFailsNow proves the waits
// FailNow rules out are gone from Stop: a component stops on a context that
// has already ended, and one still waiting for its work when the process
// fails now stops waiting at once, while the providers get what is left of
// the window.
func TestStopDoesNotWaitForComponentsOnceTheProcessFailsNow(t *testing.T) {
	resetRegistry(t)

	errFailure := errors.New("sample failure")
	stuck := make(chan struct{})
	causes := make(chan error, 2)
	Register(Component{
		Name:  "sample-worker",
		Stage: StageComponent,
		Start: func(context.Context) error { return nil },
		Stop: func(ctx context.Context) error {
			close(stuck)
			<-ctx.Done()
			causes <- context.Cause(ctx)
			return nil
		},
	})
	Register(Component{
		Name:  "sample-client",
		Stage: StageProvider,
		Start: func(context.Context) error { return nil },
		Stop: func(ctx context.Context) error {
			causes <- ctx.Err()
			return nil
		},
	})
	require.NoError(t, Start(context.Background(), StageProvider))
	require.NoError(t, Start(context.Background(), StageComponent))

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		Stop(context.Background())
	}()
	<-stuck
	FailNow(errFailure)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop kept waiting for a component after the process failed now")
	}
	require.ErrorIs(t, <-causes, errFailure, "the component's stop context must end with the failure")
	require.NoError(t, <-causes, "the providers must get what is left of the window")
}

// TestAwaitReportsWorkThatReturnedOverAnEndedWindow proves Await answers for
// the work first: work that has returned is never reported as given up on,
// even when the window to wait for it has ended too, while work still
// running is given up on once the window ends.
func TestAwaitReportsWorkThatReturnedOverAnEndedWindow(t *testing.T) {
	ended, cancel := context.WithCancel(context.Background())
	cancel()

	returned := make(chan struct{})
	close(returned)
	for range 100 {
		require.True(t, Await(ended, returned), "work that returned must not be given up on")
	}
	require.False(t, Await(ended, make(chan struct{})), "work still running is given up on once the window ends")
}

// TestComponentWithoutStopIsSkippedAtShutdown proves Stop is optional: a
// component that declared none is simply left alone.
func TestComponentWithoutStopIsSkippedAtShutdown(t *testing.T) {
	resetRegistry(t)

	var events []string
	stopless := recordingComponent("stopless", StageComponent, &events, nil)
	stopless.Stop = nil
	Register(stopless)

	require.NoError(t, Start(context.Background(), StageComponent))
	Stop(context.Background())
	require.Equal(t, []string{"start stopless"}, events)
}

// TestStartRunsOncePerStageAndStopDrainsWhatStarted proves bootstrap cannot
// start a stage twice, and that Stop only ever stops what has started since
// the previous Stop.
func TestStartRunsOncePerStageAndStopDrainsWhatStarted(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("sample", StageComponent, &events, nil))

	require.NoError(t, Start(context.Background(), StageComponent))
	require.ErrorContains(t, Start(context.Background(), StageComponent), "component stage already started")

	Stop(context.Background())
	Stop(context.Background())
	require.Equal(t, []string{"start sample", "stop sample"}, events)
}

// TestComponentsListsAStageSortedByName proves the registry reports a stage's
// members in name order, enabled or not, and nothing from other stages.
func TestComponentsListsAStageSortedByName(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("zeta", StageProvider, &events, nil))
	off := recordingComponent("alpha", StageProvider, &events, nil)
	off.Enabled = func() bool { return false }
	Register(off)
	Register(recordingComponent("loop", StageComponent, &events, nil))

	var names []string
	for _, c := range Components(StageProvider) {
		names = append(names, c.Name)
	}
	require.Equal(t, []string{"alpha", "zeta"}, names)
}

// TestRegisterRejectsProgrammerErrors proves the registry refuses what it
// could only accept by silently dropping a component: an empty name, a
// missing Start, an unknown stage, a duplicate name across stages, and a
// registration into a stage that was already started.
func TestRegisterRejectsProgrammerErrors(t *testing.T) {
	noop := func(context.Context) error { return nil }

	cases := []struct {
		name     string
		register func()
		want     string
	}{
		{
			name:     "empty name",
			register: func() { Register(Component{Name: " ", Start: noop}) },
			want:     "non-empty name",
		},
		{
			name:     "missing start",
			register: func() { Register(Component{Name: "sample"}) },
			want:     "requires a non-nil Start",
		},
		{
			name:     "unknown stage",
			register: func() { Register(Component{Name: "sample", Stage: Stage(7), Start: noop}) },
			want:     "unknown stage Stage(7)",
		},
		{
			name: "duplicate name across stages",
			register: func() {
				Register(Component{Name: "sample", Stage: StageProvider, Start: noop})
				Register(Component{Name: " sample ", Stage: StageComponent, Start: noop})
			},
			want: "duplicate component registration",
		},
		{
			name: "after the stage started",
			register: func() {
				require.NoError(t, Start(context.Background(), StageComponent))
				Register(Component{Name: "late", Stage: StageComponent, Start: noop})
			},
			want: "registered after bootstrap started the component stage",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRegistry(t)
			require.Contains(t, capturePanic(t, tc.register), tc.want)
		})
	}
}

// TestFailEndsTheProcessWithTheFirstFailure proves a component's failure
// reaches bootstrap through the failure context, with the first failure as
// its cause: a later one changes nothing, and a failure reported without an
// error still names itself.
func TestFailEndsTheProcessWithTheFirstFailure(t *testing.T) {
	resetRegistry(t)

	require.NoError(t, Failure().Err(), "nothing has failed yet")

	errFirst := errors.New("sample component failure")
	Fail(errFirst)
	Fail(errors.New("a later failure"))
	require.ErrorIs(t, context.Cause(Failure()), errFirst)

	resetRegistry(t)
	Fail(nil)
	require.ErrorContains(t, context.Cause(Failure()), "a component failed without saying why")
	require.NoError(t, FailedNow().Err(), "Fail leaves the shutdown its waits")
}

// TestFailNowEndsTheProcessWithoutItsWaits proves FailNow reaches bootstrap
// through both contexts: the failure context, as Fail does, with the first
// failure as its cause, and the one telling the shutdown not to wait, with
// this failure as its cause.
func TestFailNowEndsTheProcessWithoutItsWaits(t *testing.T) {
	resetRegistry(t)

	errFailure := errors.New("sample failure")
	FailNow(errFailure)
	require.ErrorIs(t, context.Cause(Failure()), errFailure)
	require.ErrorIs(t, context.Cause(FailedNow()), errFailure)

	resetRegistry(t)
	errEarlier := errors.New("sample earlier failure")
	Fail(errEarlier)
	FailNow(errFailure)
	require.ErrorIs(t, context.Cause(Failure()), errEarlier, "the first failure stays the reason")
	require.ErrorIs(t, context.Cause(FailedNow()), errFailure, "a later FailNow still ends the waits")
}

// recordingComponent builds a component of stage that appends its start and
// stop to events, failing to start with startErr when that is non-nil.
func recordingComponent(name string, stage Stage, events *[]string, startErr error) Component {
	return Component{
		Name:  name,
		Stage: stage,
		Start: func(context.Context) error {
			*events = append(*events, "start "+name)
			return startErr
		},
		Stop: func(context.Context) error {
			*events = append(*events, "stop "+name)
			return nil
		},
	}
}

// withLoggerConfig points config.App at a scratch logger setup so the loggers
// built during the test write under a temporary directory, and returns it.
func withLoggerConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Dir = dir
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"
	t.Cleanup(func() {
		pkgzap.Clean()
		config.App = original
	})
	return dir
}

// capturePanic runs fn and returns the message it panics with, failing the
// test when it returns normally.
func capturePanic(t *testing.T, fn func()) (msg string) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			msg = fmt.Sprint(recovered)
		}
	}()
	fn()
	t.Fatal("expected a panic")
	return ""
}

// resetRegistry rewinds the package-level registry so each test starts from
// an empty one with no stage started.
func resetRegistry(t *testing.T) {
	t.Helper()

	mu.Lock()
	defer mu.Unlock()
	components = nil
	stages = [stageCount]stageState{}
	running = nil
	failure, fail = newFailure() //nolint:fatcontext // The failure contexts are process-wide by design; a test starts from fresh ones.
	failedNow, failNow = newFailure()
}
