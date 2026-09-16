package cronjob

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

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

// TestInstantRunsOnceAcrossInstances proves the cluster contract: two
// scheduler instances sharing one primary database — two replicas — run
// each instant of a job exactly once between them.
func TestInstantRunsOnceAcrossInstances(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "shared-job")
	startInstances(t, 2)

	for range 3 {
		clock.untilWaiters(t, 2)
		clock.Advance(time.Minute)
		runs.await(t)
	}
	clock.untilWaiters(t, 2)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC): 1,
	}, runs.counts(), "each instant runs exactly once across the instances")
}

// TestPerInstanceJobRunsOnEveryInstance proves RegisterPerInstance opts a
// job out of the cluster contract: every instance runs every instant.
func TestPerInstanceJobRunsOnEveryInstance(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	RegisterPerInstance(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "local-job")
	startInstances(t, 2)

	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	runs.await(t)
	runs.await(t)
	clock.untilWaiters(t, 2)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 2,
	}, runs.counts(), "a per-instance job runs on every instance")
}

// TestHeldInstantKeepsTheNextFromEveryone proves a round still holding its
// lease keeps the following instants from the other instances too: the
// instant is skipped across the deployment, not run elsewhere, and the
// instants after the round are shared again.
func TestHeldInstantKeepsTheNextFromEveryone(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	var first atomic.Bool
	release := make(chan struct{})
	Register(func(context.Context) error {
		runs.record(clk.Now())
		if first.CompareAndSwap(false, true) {
			<-release
		}
		return nil
	}, "@every 1m", "shared-job")
	startInstances(t, 2)

	// 10:01: one instance wins and holds the lease for the whole round.
	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	runs.await(t)
	// 10:02: the other instance is refused — the lease is held — and the
	// winner is still busy, so no one runs it.
	clock.untilWaiters(t, 1)
	clock.Advance(time.Minute)
	clock.untilWaiters(t, 1)
	close(release)
	// 10:03: the round is over and the lease released; one instance runs.
	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	runs.await(t)
	clock.untilWaiters(t, 2)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC): 1,
	}, runs.counts(), "the instant under a held lease runs nowhere")
}

// TestCatchUpRunsTheMostRecentInstantNoReplicaRan proves the start-up
// catch-up: a job that has run before, whose most recent instant passed
// within the last day without any replica claiming it, runs that instant
// once at start — marked as a catch-up — and then goes on with its next.
func TestCatchUpRunsTheMostRecentInstantNoReplicaRan(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	// The job ran before: an earlier instant is on record.
	lastRun(t, "cron:catch-up-job", time.Date(2026, 1, 1, 9, 58, 0, 0, time.UTC))

	runs := make(chan time.Time, 8)
	Register(func(context.Context) error {
		runs <- clk.Now()
		return nil
	}, "@every 1m", "catch-up-job")
	require.NoError(t, start(context.Background()))

	require.Equal(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC), awaitRun(t, runs), "the catch-up runs at start")
	clock.untilWaiting(t)
	clock.Advance(30 * time.Second)
	require.Equal(t, time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), awaitRun(t, runs), "the schedule goes on with the next instant")
	require.NoError(t, stop(context.Background()))

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, "2026-01-01T10:00:00Z", entry["at"], "the catch-up runs for the instant it caught up")
	require.Equal(t, true, entry["catch_up"])
	require.EqualValues(t, 2, entry["term"], "the catch-up starts the next term of the job's lease")
}

// TestInstantsPassingDuringACatchUpAreSkipped proves the instants that pass
// while the catch-up round is in flight are skipped like any that pass
// during a round: the loop resumes with the first instant after the
// catch-up ended, not with the one computed before it began.
func TestInstantsPassingDuringACatchUpAreSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	lastRun(t, "cron:slow-catch-up-job", time.Date(2026, 1, 1, 9, 58, 0, 0, time.UTC))

	runs := newRunLog()
	var first atomic.Bool
	release := make(chan struct{})
	Register(func(context.Context) error {
		runs.record(clk.Now())
		if first.CompareAndSwap(false, true) {
			<-release
		}
		return nil
	}, "@every 1m", "slow-catch-up-job")
	require.NoError(t, start(context.Background()))

	// The catch-up for 10:00 runs at once and lasts past 10:01, the instant
	// the scheduler computed before it began.
	runs.await(t)
	clock.Advance(time.Minute)
	close(release)
	clock.untilWaiting(t)
	clock.Advance(30 * time.Second)
	runs.await(t)
	require.NoError(t, stop(context.Background()))

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC):  1,
	}, runs.counts(), "the instant overrun by the catch-up must be skipped, not run late")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob skipped instants")
	require.EqualValues(t, 1, entry["skipped"])
	require.Equal(t, "2026-01-01T10:00:00Z", entry["after"], "the skip is counted from the instant the catch-up ran for")
}

// TestCatchUpSkipsAJobThatNeverRan proves a job appearing for the first
// time starts with its next instant: the instants before its first
// deployment were never its to run.
func TestCatchUpSkipsAJobThatNeverRan(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "new-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	require.Empty(t, runs.counts(), "a job never run catches nothing up")
}

// TestCatchUpSkipsAnInstantAlreadyRun proves the catch-up never repeats an
// instant: when the most recent instant is the one on record, nothing runs
// at start.
func TestCatchUpSkipsAnInstantAlreadyRun(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	lastRun(t, "cron:settled-job", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "settled-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	require.Empty(t, runs.counts(), "an instant already run is never caught up")
}

// TestInstantsPassingDuringARunAreSkipped proves a slow round never has the
// instants it overran piled on top of it: the loop resumes with the first
// instant after the run ended, and logs how many instants the run cost. A
// round overrunning three instants is followed by one run, not four, and by
// a warning naming the three.
func TestInstantsPassingDuringARunAreSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
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

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob skipped instants")
	require.Equal(t, "overrun-job", entry["name"])
	require.EqualValues(t, 3, entry["skipped"], "the warning must count the instants the run cost")
	require.Equal(t, "2026-01-01T10:05:00Z", entry["next"])
}

// TestRoundThatLosesItsLeaseIsCutShortAndLogged proves a round outliving
// its lease ends with it: once another replica has taken the name — the
// lease ended behind the round's back — the round's context ends with
// ErrLost as the cause, the round returning that ending is logged as an
// interruption naming the loss, once, and the scheduler goes on to the next
// instant.
func TestRoundThatLosesItsLeaseIsCutShortAndLogged(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)

	entered := make(chan struct{}, 1)
	ended := make(chan error, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		ended <- context.Cause(ctx)
		return ctx.Err()
	}, "@every 1m", "lost-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")

	takeOver(t, "cron:lost-job")
	select {
	case cause := <-ended:
		require.ErrorIs(t, cause, lease.ErrLost)
	case <-time.After(5 * time.Second):
		t.Fatal("the round must end once its lease is lost")
	}
	clock.untilWaiting(t)
	pkgzap.Clean()

	interrupted := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "lost-job", interrupted["name"])
	require.Equal(t, "lease lost", interrupted["reason"], "the round returning its context's cancellation is an interruption, not a failure")
	require.EqualValues(t, 1, interrupted["term"], "the entry names the term the round ran in")
	require.Empty(t, readLogEntries(t, filepath.Join(dir, "cronjob.log"), "cronjob lost its lease during the round"),
		"the interruption entry is the record of the loss; a second entry would say the same thing")
}

// TestRoundThatReturnsNothingAfterLosingItsLeaseIsLogged proves the loss
// is recorded even when the round's own entry does not carry it: a job that
// returns nothing once its context ended finishes as far as its own entry
// goes, so the scheduler records the lost lease beside it.
func TestRoundThatReturnsNothingAfterLosingItsLeaseIsLogged(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)

	entered := make(chan struct{}, 1)
	ended := make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		ended <- struct{}{}
		return nil
	}, "@every 1m", "quiet-lost-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")

	takeOver(t, "cron:quiet-lost-job")
	awaitSignal(t, ended, "the round's end")
	clock.untilWaiting(t)
	pkgzap.Clean()

	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob lost its lease during the round")
	require.Equal(t, "quiet-lost-job", entry["name"])
	require.EqualValues(t, 1, entry["term"], "the entry names the term the round ran in")
	finished := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, "quiet-lost-job", finished["name"], "the round's own entry says it finished: it returned nothing")
}

// TestRoundInterruptedAtShutdownIsAWarning proves a round that stops because
// its context ended is not recorded as a failure, in the log or in the
// trace: the job returned the context's own cancellation, which is what it
// is asked to do when the process shuts down, and every rolling deployment
// ends a long round this way. The round's span carries the interruption as
// an event and no error status, so a search for failed rounds skips it.
func TestRoundInterruptedAtShutdownIsAWarning(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	entered := make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}, "@every 1m", "interrupted-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")
	require.NoError(t, stop(context.Background()))

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "interrupted-job", entry["name"])
	require.Equal(t, "shutting down", entry["reason"])
	require.Equal(t, "WARN", entry["level"])

	span := oteltest.EndedNamed(t, recorder, "cronjob.InterruptedJob")
	require.Equal(t, codes.Unset, span.Status().Code, "an interrupted round is neither a success nor a failure")
	require.Len(t, span.Events(), 1)
	require.Equal(t, "interrupted", span.Events()[0].Name)
	require.Equal(t, "shutting down", spanEventAttribute(span.Events()[0], "reason"))
}

// spanEventAttribute reads one string attribute of a span event.
func spanEventAttribute(event sdktrace.Event, key string) string {
	for _, kv := range event.Attributes {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

// TestNeverMatchingScheduleEndsItsLoop proves a schedule with no instant
// left — a day that never comes — ends its loop with a warning instead of
// spinning on a zero instant.
func TestNeverMatchingScheduleEndsItsLoop(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	Register(noopJob, "0 0 0 30 2 *", "never-job")
	require.NoError(t, start(context.Background()))

	<-current.done
	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob has no further instant")
	require.Equal(t, "never-job", entry["name"])
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
			withBoundCronjobLogger(t)
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
			require.Contains(t, entry["error_stack"], "job_test.go",
				"error_stack must point at the line inside the job that failed")
		})
	}
}

// TestRunStampsRoundIdentity proves the context a job runs on carries the
// round's identity and its lease, and that the outcome entry carries the
// same trace id, the instant the round ran for and the lease's term, so the
// round is found again from either side.
func TestRunStampsRoundIdentity(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	type observation struct {
		identity execctx.Identity
		term     uint64
		leased   bool
	}
	seen := make(chan observation, 1)
	Register(func(ctx context.Context) error {
		term, leased := lease.TermFromContext(ctx)
		seen <- observation{identity: execctx.FromContext(ctx), term: term, leased: leased}
		return nil
	}, "* * * * * *", "identity-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	var got observation
	select {
	case got = <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
	}
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	require.Equal(t, "identity-job", got.identity.Cronjob)
	require.NotEmpty(t, got.identity.TraceID)
	require.True(t, got.leased, "the round runs under the instant's lease")
	require.EqualValues(t, 1, got.term)
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, got.identity.TraceID, entry[consts.TRACE_ID])
	require.Equal(t, "2026-01-01T10:00:01Z", entry["at"], "the outcome entry names the instant the round ran for")
	require.EqualValues(t, 1, entry["term"], "the outcome entry names the lease's term")
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

// TestRoundThatIgnoresTheLossFailsTheProcess proves the last line behind the
// lease is the scheduler's too: a round still running once its lease is lost
// and the grace has passed would run beside the next instant's round on
// another replica, so the process is failed. The failure is recorded instead
// of tripping the process-wide one, which is one-way and would keep the test
// from running twice.
func TestRoundThatIgnoresTheLossFailsTheProcess(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)
	failures := withRecordedFailures(t)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	Register(func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}, "@every 1m", "stubborn-job")
	require.NoError(t, start(context.Background()))
	// Released before the loops are drained, so the round ends within this
	// test instead of running on into the next one's loggers.
	t.Cleanup(func() { close(release) })

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")

	takeOver(t, "cron:stubborn-job")
	select {
	case err := <-failures:
		require.ErrorContains(t, err, `lease "cron:stubborn-job" was lost`)
	case <-time.After(5 * time.Second):
		t.Fatal("a round ignoring the loss must fail the process")
	}
}
