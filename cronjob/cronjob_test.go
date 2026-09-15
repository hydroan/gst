package cronjob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TestSchedulesAreReadInUTC proves an expression names UTC wall-clock
// instants whatever zone the process runs in — every replica must compute
// the same instants — and that an expression naming its own zone with a
// CRON_TZ= prefix keeps that zone.
func TestSchedulesAreReadInUTC(t *testing.T) {
	withLocalZone(t, time.FixedZone("sample+08", 8*3600))
	resetCronjobState(t)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		spec string
		next time.Time
	}{
		{spec: "0 0 2 * * *", next: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)},
		{spec: "@daily", next: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		// Midnight UTC is 08:00 in Shanghai, so the next 02:00 there is the
		// following day's.
		{spec: "CRON_TZ=Asia/Shanghai 0 0 2 * * *", next: time.Date(2026, 1, 2, 2, 0, 0, 0, shanghai)},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			j, err := newJob(noopJob, tc.spec, "zone-job")
			require.NoError(t, err)
			got := j.schedule.Next(from)
			require.True(t, got.Equal(tc.next), "want %s, got %s", tc.next.UTC(), got.UTC())
		})
	}
}

// TestEveryRunsOnTheEpochGrid proves "@every" instants are the multiples of
// the period from the Unix epoch, not counted from the process start: the
// next instant after 10:03:20 for "@every 5m" is 10:05:00 on every replica,
// and after 10:05:00 exactly it is 10:10:00.
func TestEveryRunsOnTheEpochGrid(t *testing.T) {
	resetCronjobState(t)

	j, err := newJob(noopJob, "@every 5m", "grid-job")
	require.NoError(t, err)

	got := j.schedule.Next(time.Date(2026, 1, 1, 10, 3, 20, 0, time.UTC))
	require.Equal(t, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC), got)
	got = j.schedule.Next(time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC))
	require.Equal(t, time.Date(2026, 1, 1, 10, 10, 0, 0, time.UTC), got)
}

// TestLoopRunsAtEachInstant proves a started job runs once at every instant
// of its schedule and not before: the loop waits for the instant, runs, and
// waits for the next.
func TestLoopRunsAtEachInstant(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	runs := make(chan time.Time, 8)
	Register(func(context.Context) error {
		runs <- clk.Now()
		return nil
	}, "@every 1m", "grid-loop-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(30 * time.Second)
	require.Equal(t, time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), awaitRun(t, runs))
	// The loop computes the next instant from the clock once the run ended;
	// the clock only moves on once it is waiting for that instant.
	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	require.Equal(t, time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC), awaitRun(t, runs))
	require.NoError(t, stop(context.Background()))
	require.Empty(t, runs, "no instant ran twice and none ran early")
}

// TestInstantsPassingDuringARunAreSkipped proves a slow round never has the
// instants it overran piled on top of it: the loop resumes with the first
// instant after the run ended. A round overrunning three instants is
// followed by one run, not four.
func TestInstantsPassingDuringARunAreSkipped(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	var runs atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	Register(func(context.Context) error {
		entered <- struct{}{}
		if runs.Add(1) == 1 {
			<-release
		}
		return nil
	}, "@every 1m", "overrun-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the first round")
	// Three instants pass while the first round is still in flight.
	clock.Advance(3 * time.Minute)
	close(release)
	// The next instant after the run ended is 10:05, not the 10:02 it
	// overran: once the loop waits for it, one more minute brings it.
	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round after the overrun")
	require.NoError(t, stop(context.Background()))
	require.EqualValues(t, 2, runs.Load(), "the instants overrun by the first round must be skipped")
}

// TestStopEndsTheRoundAndWaitsForIt proves stop ends the context of a
// round in flight and returns once the round has returned, so a job that
// honors its context stops early and shutdown never tears the connections
// out from under a half-done round.
func TestStopEndsTheRoundAndWaitsForIt(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	entered := make(chan struct{}, 1)
	ended := make(chan error, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		ended <- ctx.Err()
		return nil
	}, "* * * * * *", "cancelable-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	awaitSignal(t, entered, "the round")
	require.NoError(t, stop(context.Background()))
	select {
	case err := <-ended:
		require.ErrorIs(t, err, context.Canceled, "the round's context must end with the shutdown")
	default:
		t.Fatal("stop returned before the in-flight round finished")
	}
}

// TestStopGivesUpOnAJobThatIgnoresItsContext proves a job that ignores its
// context cannot hold the shutdown hostage: stop returns once the context it
// was given expires, reporting the round it gave up on.
func TestStopGivesUpOnAJobThatIgnoresItsContext(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	// Released at the end so the round, logging included, ends within this
	// test instead of running on into the next one's loggers.
	t.Cleanup(func() { close(release) })
	Register(func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}, "* * * * * *", "stuck-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	awaitSignal(t, entered, "the round")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := stop(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"giving up on the in-flight round must be reported, not swallowed")
}

// TestStopWithoutStartIsNoop keeps stop safe in processes that never started
// the scheduler.
func TestStopWithoutStartIsNoop(t *testing.T) {
	resetCronjobState(t)

	require.NoError(t, stop(context.Background()))
}

// TestNeverMatchingScheduleEndsItsLoop proves a schedule with no instant
// left — a day that never comes — ends its loop with a warning instead of
// spinning on a zero instant.
func TestNeverMatchingScheduleEndsItsLoop(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	Register(noopJob, "0 0 0 30 2 *", "never-job")
	require.NoError(t, start(context.Background()))

	<-current.done
	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob has no further instant")
	require.Equal(t, "never-job", entry["name"])
}

// TestRegistrationErrorsFailStartup proves a registration the scheduler
// cannot honor fails the process at startup instead of silently dropping
// the job: the name is the job's identity, so it has to be a trusted input.
func TestRegistrationErrorsFailStartup(t *testing.T) {
	cases := []struct {
		name     string
		register func()
		want     string
	}{
		{
			name:     "no name",
			register: func() { Register(noopJob, "* * * * * *", " ") },
			want:     "has no name",
		},
		{
			name:     "nil function",
			register: func() { Register(nil, "* * * * * *", "sample-job") },
			want:     `cronjob "sample-job": nil function`,
		},
		{
			name:     "empty schedule",
			register: func() { Register(noopJob, " ", "sample-job") },
			want:     `cronjob "sample-job": empty schedule`,
		},
		{
			name:     "invalid schedule",
			register: func() { Register(noopJob, "not a schedule", "sample-job") },
			want:     `cronjob "sample-job": invalid schedule "not a schedule"`,
		},
		{
			name: "duplicate name",
			register: func() {
				Register(noopJob, "* * * * * *", "sample-job")
				Register(noopJob, "@hourly", " sample-job ")
			},
			want: `cronjob "sample-job": registered twice`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withCronjobLoggerConfig(t)
			resetCronjobState(t)

			tc.register()
			require.ErrorContains(t, start(context.Background()), tc.want)
		})
	}
}

// TestRegisterAfterStartPanics proves registration belongs in package init
// functions: a job registered once the scheduler runs would never be
// scheduled, so it fails fast instead.
func TestRegisterAfterStartPanics(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	require.NoError(t, start(context.Background()))
	require.PanicsWithValue(t,
		`cronjob: "late-job" registered after the scheduler started; register jobs in package init functions`,
		func() { Register(noopJob, "* * * * * *", "late-job") })
}

// TestStartAdoptsSharedCronjobLogger proves scheduling logs flow through the
// shared logger.Cronjob instance instead of a second package-local logger on
// the same file, which would race lumberjack rotation against it.
func TestStartAdoptsSharedCronjobLogger(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	shared := pkgzap.New("shared_cronjob.log")
	original := logger.Cronjob
	logger.Cronjob = shared
	t.Cleanup(func() { logger.Cronjob = original })

	Register(noopJob, "0 0 * * * *", "sample-job")
	require.NoError(t, start(context.Background()))
	pkgzap.Clean()

	data, err := os.ReadFile(filepath.Join(dir, "shared_cronjob.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "scheduled cronjob",
		"scheduling must log through the shared cronjob logger")
	require.NoFileExists(t, filepath.Join(dir, "cronjob.log"),
		"no package-local logger may open the shared log file")
}

// TestStartFallsBackToLocalLoggerWithoutShared keeps the pre-existing
// behavior for processes that never ran the logging setup (unit tests):
// scheduling still logs through a package-local logger.
func TestStartFallsBackToLocalLoggerWithoutShared(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	original := logger.Cronjob
	logger.Cronjob = nil
	t.Cleanup(func() { logger.Cronjob = original })

	Register(noopJob, "0 0 * * * *", "fallback-job")
	require.NoError(t, start(context.Background()))
	pkgzap.Clean()

	data, err := os.ReadFile(filepath.Join(dir, "cronjob.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "scheduled cronjob")
}

// TestSchedulerRunsAsLifecycleComponent proves importing the package is what
// enables scheduling: the component registered from init starts the
// scheduler when bootstrap starts the lifecycle components, and stops it
// when bootstrap stops them.
func TestSchedulerRunsAsLifecycleComponent(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	entered := make(chan struct{}, 1)
	Register(func(context.Context) error {
		entered <- struct{}{}
		return nil
	}, "* * * * * *", "component-job")

	require.NoError(t, lifecycle.Start(context.Background(), lifecycle.StageComponent))
	clock.Advance(time.Second)
	awaitSignal(t, entered, "the round")

	lifecycle.Stop(context.Background())
	require.NotNil(t, current, "the scheduler must have been started through the component")
}

// TestRunLogsFailureWithErrorStack proves a failed round leaves an entry the
// error_stack field can locate. For a job the cronjob log is the only record
// of its failure, so the entry has to point at the failing line, not just
// name the job: the returned error goes out as a typed error field, and a
// panic becomes an error whose stack still points at the line that panicked.
func TestRunLogsFailureWithErrorStack(t *testing.T) {
	cases := []struct {
		name string
		job  func() error
		msg  string
		err  string
	}{
		{
			name: "returned error",
			job:  func() error { return errors.New("sample failure") },
			msg:  "finished cronjob with error",
			err:  "sample failure",
		},
		{
			name: "panic",
			job:  func() error { panic("sample panic") },
			msg:  "cronjob panicked",
			err:  "sample panic",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withCronjobLoggerConfig(t)
			resetCronjobState(t)
			clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

			entered := make(chan struct{}, 1)
			Register(func(context.Context) error {
				entered <- struct{}{}
				return tc.job()
			}, "* * * * * *", "failing-job")
			require.NoError(t, start(context.Background()))

			clock.Advance(time.Second)
			awaitSignal(t, entered, "the round")
			// Stop waits for the in-flight round, whose outcome entry is
			// written before the round returns.
			require.NoError(t, stop(context.Background()))
			pkgzap.Clean()

			entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), tc.msg)
			require.Equal(t, tc.err, entry["error"])
			require.NotEmpty(t, entry[consts.TRACE_ID], "the outcome entry must carry the round's trace id")
			require.Contains(t, entry["error_stack"], "cronjob_test.go",
				"error_stack must point at the line inside the job that failed")
		})
	}
}

// TestRunStampsRoundIdentity proves the context a job runs on carries the
// round's identity, and that the outcome entry carries the same trace id and
// the instant the round ran for, so the round is found again from either
// side.
func TestRunStampsRoundIdentity(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	seen := make(chan execctx.Identity, 1)
	Register(func(ctx context.Context) error {
		seen <- execctx.FromContext(ctx)
		return nil
	}, "* * * * * *", "identity-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	var id execctx.Identity
	select {
	case id = <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
	}
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	require.Equal(t, "identity-job", id.Cronjob)
	require.NotEmpty(t, id.TraceID)
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, id.TraceID, entry[consts.TRACE_ID])
	require.Equal(t, "2026-01-01T10:00:01Z", entry["at"], "the outcome entry names the instant the round ran for")
}

// TestRunOpensRoundSpanWhenTracingIsOn proves a round gets a root span of its
// own when tracing is on: the job runs under it, and the round's trace id is
// the span's, so the trace in the tracing backend and the id in the logs and
// statement comments are one trail.
func TestRunOpensRoundSpanWhenTracingIsOn(t *testing.T) {
	withCronjobLoggerConfig(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	seen := make(chan roundObservation, 1)
	Register(func(ctx context.Context) error {
		seen <- roundObservation{
			identity: execctx.FromContext(ctx),
			span:     oteltrace.SpanFromContext(ctx).SpanContext(),
		}
		return nil
	}, "* * * * * *", "traced-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	var got roundObservation
	select {
	case got = <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
	}
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	require.True(t, got.span.HasTraceID(), "the job must run under the round's span")
	require.Equal(t, got.span.TraceID().String(), got.identity.TraceID)
	span := oteltest.EndedNamed(t, recorder, "cronjob.TracedJob")
	require.Equal(t, got.identity.TraceID, span.SpanContext().TraceID().String())
}

// roundObservation is what a job sees of its round: the identity on its
// context and the span it runs under.
type roundObservation struct {
	identity execctx.Identity
	span     oteltrace.SpanContext
}

// noopJob is a job that does nothing, for tests about scheduling rather
// than running.
func noopJob(context.Context) error {
	return nil
}

// fakeClock is a clock the test drives by hand: time stands still until the
// test moves it, and a wait ends the moment the test moves past its instant.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
	// waiting is signaled whenever a wait is registered, so a test can hold
	// the clock still until the loop is waiting again.
	waiting chan struct{}
}

// fakeWaiter is one pending Wait: the instant it waits for and the channel
// closed once the clock passes it.
type fakeWaiter struct {
	at   time.Time
	wake chan struct{}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Wait(ctx context.Context, t time.Time) bool {
	c.mu.Lock()
	if !t.After(c.now) {
		c.mu.Unlock()
		return true
	}
	wake := make(chan struct{})
	c.waiters = append(c.waiters, fakeWaiter{at: t, wake: wake})
	c.mu.Unlock()
	select {
	case c.waiting <- struct{}{}:
	default:
	}

	select {
	case <-wake:
		return true
	case <-ctx.Done():
		return false
	}
}

// untilWaiting blocks until a wait is pending on the clock: the loop has
// computed its next instant and is waiting for it, so the clock can move on
// without the two racing over what "now" is.
func (c *fakeClock) untilWaiting(t *testing.T) {
	t.Helper()

	for {
		c.mu.Lock()
		pending := len(c.waiters)
		c.mu.Unlock()
		if pending > 0 {
			return
		}
		select {
		case <-c.waiting:
		case <-time.After(5 * time.Second):
			t.Fatal("nothing waited on the clock")
		}
	}
}

// Advance moves the clock forward by d and wakes every wait whose instant
// has passed.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
	pending := c.waiters[:0]
	for _, w := range c.waiters {
		if w.at.After(c.now) {
			pending = append(pending, w)
			continue
		}
		close(w.wake)
	}
	c.waiters = pending
}

// withFakeClock hands the scheduler a clock standing at now, which the test
// drives by hand, and restores the system clock afterwards.
func withFakeClock(t *testing.T, now time.Time) *fakeClock {
	t.Helper()

	clock := &fakeClock{now: now, waiting: make(chan struct{}, 1)}
	original := clk
	clk = clock
	// The loops read the clock; they must be gone before it changes hands.
	t.Cleanup(func() {
		drainLoops()
		clk = original
	})
	return clock
}

// withLocalZone runs the test with the process zone set to loc, the way a
// process deployed in that zone sees time.Local.
func withLocalZone(t *testing.T, loc *time.Location) {
	t.Helper()

	original := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = original })
}

// awaitRun receives the next run from runs, failing the test when none
// comes in time.
func awaitRun(t *testing.T, runs <-chan time.Time) time.Time {
	t.Helper()

	select {
	case at := <-runs:
		return at
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
		return time.Time{}
	}
}

// awaitSignal waits for one signal on ch, failing the test when none comes
// in time.
func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not run", what)
	}
}

// withCronjobLoggerConfig points config.App at a scratch logger setup so the
// loggers built during the test write under a temporary directory.
func withCronjobLoggerConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Dir = dir
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"

	t.Cleanup(func() { config.App = original })
	return dir
}

// resetCronjobState stops whatever the previous test left running, waits
// for its loops, and rewinds the package-level scheduler state so each test
// exercises start from scratch. The loops this test starts are drained
// again once it ends, so none of them runs on into the next test.
func resetCronjobState(t *testing.T) {
	t.Helper()

	drainLoops()
	t.Cleanup(drainLoops)

	mu.Lock()
	defer mu.Unlock()
	jobs = nil
	errRegister = nil
	log = nil
	current = nil
}

// drainLoops ends the running loops, if any, and waits for them to return.
// A job that ignores its context has to be released by its test first.
func drainLoops() {
	mu.Lock()
	s := current
	mu.Unlock()
	if s != nil {
		s.cancel()
		<-s.done
	}
}

// readLogEntry returns the first JSON entry of the log file whose msg field
// equals msg, failing the test when the file has none.
func readLogEntry(t *testing.T, path, msg string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		entry := make(map[string]any)
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "log line must be JSON: %s", line)
		if entry["msg"] == msg {
			return entry
		}
	}
	require.Failf(t, "missing log entry", "no entry with msg %q in %s", msg, path)
	return nil
}
