package cronjob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
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

// TestStopReportsNothingOnceTheRoundsReturned proves stop tells rounds that
// returned from rounds it gave up on even when its window has already ended —
// used up by the components stopped before it, or never given because the
// process must not wait: rounds that returned are never reported as given up
// on.
func TestStopReportsNothingOnceTheRoundsReturned(t *testing.T) {
	s := newScheduler(nil)
	s.cancel = func() {}
	close(s.done)
	ended, cancel := context.WithCancel(context.Background())
	cancel()

	for range 100 {
		require.NoError(t, s.stop(ended), "rounds that returned must not be reported as given up on")
	}
}

// TestStopWithoutStartIsNoop keeps stop safe in processes that never started
// the scheduler.
func TestStopWithoutStartIsNoop(t *testing.T) {
	resetCronjobState(t)

	require.NoError(t, stop(context.Background()))
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

// TestStartFallsBackToTheGlobalStreamWithoutABoundLogger proves what a
// process that never ran the lifecycle — a unit test — logs through: the
// entries go to the global log stream, tagged with the component, and the
// scheduler opens no file of its own, which would put a second rotation
// instance on the file the lifecycle's own logger owns.
func TestStartFallsBackToTheGlobalStreamWithoutABoundLogger(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	entries := withObservedGlobalLogger(t)

	Register(noopJob, "0 0 * * * *", "fallback-job")
	require.NoError(t, start(context.Background()))
	pkgzap.Clean()

	scheduled := entries.FilterMessage("scheduled cronjob").All()
	require.Len(t, scheduled, 1, "scheduling must log through the fallback logger")
	require.Equal(t, "cronjob", scheduled[0].ContextMap()["component"])
	require.NoFileExists(t, filepath.Join(dir, "cronjob.log"),
		"the fallback must not open the file the lifecycle's logger owns")
}

// TestSchedulerIsALifecycleComponent proves importing the package is what
// enables scheduling: init registered the scheduler as a lifecycle component
// whose Start, Stop and logger binding are this package's, so bootstrap
// starts it once the tables are ready, stops it as the process drains, and
// hands it the dedicated cronjob.log. The component is driven directly — a
// lifecycle stage starts once per process, which would make the test
// unrepeatable.
func TestSchedulerIsALifecycleComponent(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	var component lifecycle.Component
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		if c.Name == "cronjob" {
			component = c
		}
	}
	require.NotNil(t, component.Start, "the scheduler must register itself as a lifecycle component")
	require.NotNil(t, component.SetLogger, "the scheduler must take the dedicated logger the lifecycle binds")

	entered := make(chan struct{}, 1)
	Register(func(context.Context) error {
		entered <- struct{}{}
		return nil
	}, "* * * * * *", "component-job")

	component.SetLogger(pkgzap.New("bound_cronjob.log"))
	require.NoError(t, component.Start(context.Background()))
	clock.Advance(time.Second)
	awaitSignal(t, entered, "the round")
	require.NoError(t, component.Stop(context.Background()))
	require.NotNil(t, current, "the scheduler must have been started through the component")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "bound_cronjob.log"), "scheduled cronjob")
	require.Equal(t, "component-job", entry["name"])
	require.NoFileExists(t, filepath.Join(dir, "cronjob.log"), "the bound logger replaces the package's own")
}
