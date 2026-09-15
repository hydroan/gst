package cronjob

import (
	"context"
	"encoding/json"
	"maps"
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
	"github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestMain gives the suite a primary database — the dialect under test, so
// the Makefile test target runs the multi-replica scenarios on every dialect
// the lease engine reads a clock from — prepared the way bootstrap's first
// phase would, without the lifecycle: the scheduler under test has to be
// started by the tests themselves.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run holds the body of TestMain so that the deferred release still happens:
// the os.Exit in TestMain would skip it.
func run(m *testing.M) int {
	release, _, err := testcontainer.SetupDatabase(testutil.DatabaseUnderTest())
	if err != nil {
		panic(err)
	}
	defer func() { _ = release() }()

	logger.Gorm = gormlogger.Discard
	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := errors.Join(sqlite.Init(), mysql.Init(), postgres.Init()); err != nil {
		panic(err)
	}
	dbruntime.Wait()
	return m.Run()
}

// TestSchedulesAreReadInUTC proves an expression is read in UTC — the zone
// the schedule computes in is UTC, not the process's, so every replica
// computes the same instants — and that an expression naming its own zone
// with a CRON_TZ= prefix keeps that zone. The zone is asserted on the
// schedule itself: on a host whose own zone is UTC the instants alone could
// not tell the two apart.
func TestSchedulesAreReadInUTC(t *testing.T) {
	resetCronjobState(t)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		spec string
		zone string
		next time.Time
	}{
		{spec: "0 0 2 * * *", zone: "UTC", next: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)},
		{spec: "@daily", zone: "UTC", next: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		// Midnight UTC is 08:00 in Shanghai, so the next 02:00 there is the
		// following day's: 18:00 UTC.
		{spec: "CRON_TZ=Asia/Shanghai 0 0 2 * * *", zone: "Asia/Shanghai", next: time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			j, err := newJob(noopJob, tc.spec, "zone-job")
			require.NoError(t, err)
			spec, ok := j.schedule.(*cron.SpecSchedule)
			require.True(t, ok, "an expression parses to a spec schedule")
			require.Equal(t, tc.zone, spec.Location.String(), "the zone the schedule is read in")
			got := j.schedule.Next(from)
			require.True(t, got.Equal(tc.next), "want %s, got %s", tc.next, got.UTC())
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

// TestPreviousInstantFindsTheMostRecentOne proves the bisection behind the
// catch-up: it finds the most recent instant at or before now, an instant
// falling on now itself included, and reports none when the schedule had no
// instant within the last day.
func TestPreviousInstantFindsTheMostRecentOne(t *testing.T) {
	resetCronjobState(t)

	every, err := newJob(noopJob, "@every 5m", "grid-job")
	require.NoError(t, err)
	daily, err := newJob(noopJob, "0 0 2 * * *", "daily-job")
	require.NoError(t, err)
	yearly, err := newJob(noopJob, "0 0 0 1 1 *", "yearly-job")
	require.NoError(t, err)

	prev, ok := previousInstant(every.schedule, time.Date(2026, 1, 1, 10, 3, 20, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), prev)

	prev, ok = previousInstant(every.schedule, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC), prev, "an instant falling on now counts as passed")

	prev, ok = previousInstant(daily.schedule, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), prev)

	_, ok = previousInstant(yearly.schedule, time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC))
	require.False(t, ok, "an instant older than a day is history, not a missed round")
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

// TestStartRefusesAClickhousePrimary proves a job under a lease cannot start
// on a primary database that carries no leases: the deployment could not
// coordinate, so the start fails instead of every replica running every
// instant. A job registered per instance is unaffected.
func TestStartRefusesAClickhousePrimary(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)

	Register(noopJob, "* * * * * *", "shared-job")
	err := startOnClickhouse()
	require.ErrorIs(t, err, lease.ErrUnsupportedDatabase)
	require.ErrorContains(t, err, `cronjob "shared-job" runs once per instant across the deployment`)

	resetCronjobState(t)
	RegisterPerInstance(noopJob, "0 0 * * * *", "local-job")
	require.NoError(t, startOnClickhouse(), "a per-instance job needs no lease")
}

// startOnClickhouse starts the scheduler with a ClickHouse primary database
// in place of the suite's, for the duration of the start alone: the lease
// engine reads only the database's name from it, and the suite's database
// is back before anything touches a table.
func startOnClickhouse() error {
	original := dbruntime.DB
	dbruntime.DB = &gorm.DB{Config: &gorm.Config{Dialector: clickhouseDialector{}}}
	defer func() { dbruntime.DB = original }()
	return start(context.Background())
}

// TestInstantsPassingDuringARunAreSkipped proves a slow round never has the
// instants it overran piled on top of it: the loop resumes with the first
// instant after the run ended, and logs how many instants the run cost. A
// round overrunning three instants is followed by one run, not four, and by
// a warning naming the three.
func TestInstantsPassingDuringARunAreSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
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

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob skipped instants")
	require.Equal(t, "overrun-job", entry["name"])
	require.EqualValues(t, 3, entry["skipped"], "the warning must count the instants the run cost")
	require.Equal(t, "2026-01-01T10:05:00Z", entry["next"])
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

// TestRoundThatLosesItsLeaseIsCutShortAndLogged proves a round outliving
// its lease ends with it: once another replica has taken the name — the
// lease ended behind the round's back — the round's context ends with
// ErrLost as the cause, the loss is logged as the round's outcome, and the
// scheduler goes on to the next instant.
func TestRoundThatLosesItsLeaseIsCutShortAndLogged(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
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

	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob lost its lease during the round")
	require.Equal(t, "lost-job", entry["name"])
	require.EqualValues(t, 1, entry["term"], "the entry names the term the round ran in")
	interrupted := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "lease lost", interrupted["reason"], "the round returning its context's cancellation is an interruption, not a failure")
}

// TestRoundInterruptedAtShutdownIsAWarning proves a round that stops because
// its context ended is not logged as a failure: the job returned the
// context's own cancellation, which is what it is asked to do when the
// process shuts down, and every rolling deployment ends a long round this
// way.
func TestRoundInterruptedAtShutdownIsAWarning(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
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
			// The parser would read Local as the process's own zone — the one
			// zone replicas need not share, and the one the UTC default is
			// there to rule out — so naming it is refused, with either prefix.
			name:     "Local zone",
			register: func() { Register(noopJob, "CRON_TZ=Local 0 0 2 * * *", "sample-job") },
			want:     `names the Local zone`,
		},
		{
			name:     "Local zone with the short prefix",
			register: func() { Register(noopJob, "TZ=Local 0 0 2 * * *", "sample-job") },
			want:     `names the Local zone`,
		},
		{
			name: "duplicate name",
			register: func() {
				Register(noopJob, "* * * * * *", "sample-job")
				RegisterPerInstance(noopJob, "@hourly", " sample-job ")
			},
			want: `cronjob "sample-job": registered twice`,
		},
		{
			// The lease table holds 191 characters, "cron:" included; a job
			// that could never claim its instants must not start at all.
			name:     "name the lease table cannot hold",
			register: func() { Register(noopJob, "* * * * * *", strings.Repeat("n", 187)) },
			want:     "longer than the 191 the name column holds",
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

// TestSchedulerIsALifecycleComponent proves importing the package is what
// enables scheduling: init registered the scheduler as a lifecycle component
// whose Start and Stop are this package's, so bootstrap starts it once the
// tables are ready and stops it as the process drains. The component is
// driven directly — a lifecycle stage starts once per process, which would
// make the test unrepeatable.
func TestSchedulerIsALifecycleComponent(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	var component lifecycle.Component
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		if c.Name == "cronjob" {
			component = c
		}
	}
	require.NotNil(t, component.Start, "the scheduler must register itself as a lifecycle component")

	entered := make(chan struct{}, 1)
	Register(func(context.Context) error {
		entered <- struct{}{}
		return nil
	}, "* * * * * *", "component-job")

	require.NoError(t, component.Start(context.Background()))
	clock.Advance(time.Second)
	awaitSignal(t, entered, "the round")
	require.NoError(t, component.Stop(context.Background()))
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
// round's identity and its lease, and that the outcome entry carries the
// same trace id, the instant the round ran for and the lease's term, so the
// round is found again from either side.
func TestRunStampsRoundIdentity(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
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

// roundObservation is what a job sees of its round: the identity on its
// context and the span it runs under.
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

type roundObservation struct {
	identity execctx.Identity
	span     oteltrace.SpanContext
}

// clickhouseDialector stands in for a ClickHouse primary database: only its
// name is ever read, by the lease engine deciding whether it can run.
type clickhouseDialector struct {
	gorm.Dialector
}

func (clickhouseDialector) Name() string { return "clickhouse" }

// noopJob is a job that does nothing, for tests about scheduling rather
// than running.
func noopJob(context.Context) error {
	return nil
}

// runLog counts the runs of a job per instant, across every instance
// running it, and signals each run.
type runLog struct {
	mu         sync.Mutex
	perInstant map[time.Time]int
	ran        chan struct{}
}

func newRunLog() *runLog {
	return &runLog{perInstant: make(map[time.Time]int), ran: make(chan struct{}, 64)}
}

// record counts one run at the instant at.
func (l *runLog) record(at time.Time) {
	l.mu.Lock()
	l.perInstant[at]++
	l.mu.Unlock()
	l.ran <- struct{}{}
}

// await waits for one run, failing the test when none comes in time.
func (l *runLog) await(t *testing.T) {
	t.Helper()
	awaitSignal(t, l.ran, "a round")
}

// counts returns a copy of the runs per instant.
func (l *runLog) counts() map[time.Time]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.perInstant)
}

// lastRun puts an earlier run of a job on record: the lease row exists and
// names at as the last instant claimed, the way a job that ran before looks
// to a starting scheduler.
func lastRun(t *testing.T, leaseName string, at time.Time) {
	t.Helper()

	h, claimed, err := lease.ClaimSlot(context.Background(), leaseName, at)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, h.Release(context.Background()))
}

// takeOver acts as another replica taking the name: the lease is ended in
// the table behind the holder's back and claimed anew. The claim is released
// once the test ends.
func takeOver(t *testing.T, name string) {
	t.Helper()

	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", name).Error)
	taken, claimed, err := lease.Claim(context.Background(), name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = taken.Release(context.Background()) })
}

// withFastLease shrinks the lease protocol's timings so a loss and the grace
// after it play out in milliseconds, and restores them afterwards.
func withFastLease(t *testing.T) {
	t.Helper()

	t.Cleanup(lease.SetTimings(300*time.Millisecond, 50*time.Millisecond, 150*time.Millisecond, 100*time.Millisecond))
}

// withRecordedFailures records the process failures the lease engine reports
// instead of ending the test process, and restores the real one afterwards.
func withRecordedFailures(t *testing.T) <-chan error {
	t.Helper()

	failures := make(chan error, 4)
	t.Cleanup(lease.SetFail(func(err error) { failures <- err }))
	return failures
}

// startInstances builds and starts n schedulers over the registered jobs —
// n replicas sharing the primary database — and stops them once the test
// ends.
func startInstances(t *testing.T, n int) []*scheduler {
	t.Helper()

	if log == nil {
		log = pkgzap.New("cronjob.log")
	}
	instances := make([]*scheduler, 0, n)
	for range n {
		s := newScheduler(jobs)
		s.start(context.Background())
		instances = append(instances, s)
	}
	t.Cleanup(func() {
		for _, s := range instances {
			_ = s.stop(context.Background())
		}
	})
	return instances
}

// fakeClock is a clock the test drives by hand: time stands still until the
// test moves it, and a wait ends the moment the test moves past its instant.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
	// waiting is signaled whenever a wait is registered, so a test can hold
	// the clock still until the loops are waiting again.
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
	c.untilWaiters(t, 1)
}

// untilWaiters blocks until at least n waits are pending on the clock — n
// loops waiting for their next instant.
func (c *fakeClock) untilWaiters(t *testing.T, n int) {
	t.Helper()

	for {
		c.mu.Lock()
		pending := len(c.waiters)
		c.mu.Unlock()
		if pending >= n {
			return
		}
		select {
		case <-c.waiting:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d loops waited on the clock", pending, n)
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
// for its loops, rewinds the package-level scheduler state and empties the
// lease table, so each test exercises start from scratch. The loops this
// test starts are drained again once it ends, so none of them runs on into
// the next test.
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
	require.NoError(t, dbruntime.DB.Exec("DELETE FROM gst_leases").Error)
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
