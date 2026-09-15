package leader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/execctx"
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
// phase would, without the lifecycle: the elector under test has to be
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

// TestOneReplicaLeadsAtATime proves the cluster contract: with several
// replicas campaigning for a name, the work runs on one of them while the
// others wait, and once the leader stops the work moves to another replica —
// never running on two at once.
func TestOneReplicaLeadsAtATime(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)
	withFastCampaign(t)

	tenures := newTenureLog()
	Register(func(ctx context.Context) error {
		tenures.begin()
		defer tenures.end()
		<-ctx.Done()
		return nil
	}, "shared-work")

	first := startInstances(t, 1)[0]
	tenures.awaitBegin(t)
	startInstances(t, 2)
	// Three campaign intervals later the name is still the first's.
	time.Sleep(3 * campaignInterval)
	require.Equal(t, 1, tenures.started(), "a held name is refused to the other replicas")

	require.NoError(t, first.stop(context.Background()))
	tenures.awaitBegin(t)
	require.Equal(t, 2, tenures.started(), "the work moves to another replica once its leader stops")
	require.Equal(t, 1, tenures.maxActive(), "the work never runs on two replicas at once")
}

// TestLostLeaseEndsTheTenure proves a tenure ends with its lease: once
// another replica has taken the name — the lease ended behind the leader's
// back — the work's context ends with ErrLost as the cause and the loss is
// the tenure's logged outcome; the replica goes on campaigning and wins the
// name back once the other lets it go.
func TestLostLeaseEndsTheTenure(t *testing.T) {
	dir := withLeaderLoggerConfig(t)
	resetLeaderState(t)
	withFastCampaign(t)
	withFastLease(t)

	tenures := newTenureLog()
	causes := make(chan error, 8)
	Register(func(ctx context.Context) error {
		tenures.begin()
		defer tenures.end()
		<-ctx.Done()
		causes <- context.Cause(ctx)
		return ctx.Err()
	}, "lost-work")
	startInstances(t, 1)
	tenures.awaitBegin(t)

	other := takeOver(t, "leader:lost-work")
	require.ErrorIs(t, awaitCause(t, causes), lease.ErrLost)
	require.NoError(t, other.Release(context.Background()))
	tenures.awaitBegin(t)
	require.Equal(t, 2, tenures.started(), "the replica campaigns again after losing the lease")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "leader.log"), "leader stepped down with error")
	require.Equal(t, "lost-work", entry["name"])
	require.Equal(t, "lease lost", entry["reason"])
	require.EqualValues(t, 1, entry["term"], "the entry names the term the tenure ran in")
}

// TestWorkThatIgnoresTheLossFailsTheProcess proves the last line behind the
// lease: work still running once its lease is lost and the grace has passed
// would run beside the new leader's, so the process is failed — through the
// lifecycle, which ends bootstrap's Run.
func TestWorkThatIgnoresTheLossFailsTheProcess(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)
	withFastCampaign(t)
	withFastLease(t)
	withShortGrace(t)

	failures := make(chan error, 1)
	fail = func(err error) { failures <- err }
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	Register(func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}, "stubborn-work")
	startInstances(t, 1)
	// Released before the instance is stopped, so the tenure ends within
	// this test instead of running on into the next one's loggers.
	t.Cleanup(func() { close(release) })
	awaitSignal(t, entered, "the tenure")

	takeOver(t, "leader:stubborn-work")
	select {
	case err := <-failures:
		require.ErrorContains(t, err, `leader "stubborn-work" lost its lease and its work has not stopped`)
	case <-time.After(5 * time.Second):
		t.Fatal("work ignoring the loss must fail the process")
	}
}

// TestWorkThatReturnsIsCampaignedForAgain proves work that returns while
// still the leader hands the name back and is started again after the
// campaign interval, with the return — with or without an error — as the
// tenure's logged outcome.
func TestWorkThatReturnsIsCampaignedForAgain(t *testing.T) {
	dir := withLeaderLoggerConfig(t)
	resetLeaderState(t)
	withFastCampaign(t)

	done := make(chan struct{}, 16)
	Register(func(context.Context) error {
		done <- struct{}{}
		return nil
	}, "done-work")
	failed := make(chan struct{}, 16)
	Register(func(context.Context) error {
		failed <- struct{}{}
		return errors.New("sample failure")
	}, "failing-work")
	startInstances(t, 1)

	for range 3 {
		awaitSignal(t, done, "the work that returns")
		awaitSignal(t, failed, "the work that fails")
	}

	pkgzap.Clean()
	path := filepath.Join(dir, "leader.log")
	entry := readLogEntry(t, path, "leader stepped down")
	require.Equal(t, "done-work", entry["name"])
	require.Equal(t, "work returned", entry["reason"])
	entry = readLogEntry(t, path, "leader stepped down with error")
	require.Equal(t, "failing-work", entry["name"])
	require.Equal(t, "work returned", entry["reason"])
	require.Contains(t, entry["error"], "sample failure")
	require.Contains(t, entry["error_stack"], "leader_test.go", "the entry must locate the failing line")
}

// TestWorkPanicIsRecoveredAndLogged proves a panic in the work neither takes
// the process down nor ends the elections: it is logged with the stack of
// the panic site, and the work is started again.
func TestWorkPanicIsRecoveredAndLogged(t *testing.T) {
	dir := withLeaderLoggerConfig(t)
	resetLeaderState(t)
	withFastCampaign(t)

	runs := make(chan struct{}, 16)
	Register(func(context.Context) error {
		runs <- struct{}{}
		panic("sample panic")
	}, "panicking-work")
	startInstances(t, 1)

	awaitSignal(t, runs, "the work")
	awaitSignal(t, runs, "the work, again")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "leader.log"), "leader work panicked")
	require.Equal(t, "panicking-work", entry["name"])
	require.Contains(t, entry["error"], "sample panic")
	require.Contains(t, entry["error_stack"], "leader_test.go", "the stack must locate the line that panicked")
}

// TestStopEndsTheTenureAndReleasesTheName proves shutdown reaches the work —
// its context ends, without ErrLost — and hands the name back at once, so
// another replica takes it over without waiting the lease out.
func TestStopEndsTheTenureAndReleasesTheName(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)

	entered := make(chan struct{}, 1)
	causes := make(chan error, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		causes <- context.Cause(ctx)
		return nil
	}, "stopped-work")
	require.NoError(t, start(context.Background()))
	awaitSignal(t, entered, "the tenure")

	require.NoError(t, stop(context.Background()))
	require.ErrorIs(t, awaitCause(t, causes), context.Canceled)

	h, claimed, err := lease.Claim(context.Background(), "leader:stopped-work")
	require.NoError(t, err)
	require.True(t, claimed, "the name must be free the moment the leader stopped")
	require.NoError(t, h.Release(context.Background()))
}

// TestStopGivesUpOnWorkThatIgnoresItsContext proves stop is bounded by its
// context: work that never returns cannot hold the shutdown hostage, and
// giving up on it is reported.
func TestStopGivesUpOnWorkThatIgnoresItsContext(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	// Released at the end so the tenure ends within this test instead of
	// running on into the next one's loggers.
	t.Cleanup(func() { close(release) })
	Register(func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}, "stuck-work")
	require.NoError(t, start(context.Background()))
	awaitSignal(t, entered, "the tenure")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, stop(ctx), context.DeadlineExceeded,
		"giving up on the work must be reported, not swallowed")
}

// TestStopWithoutStartIsNoop keeps stop safe in processes that never started
// the elector.
func TestStopWithoutStartIsNoop(t *testing.T) {
	resetLeaderState(t)

	require.NoError(t, stop(context.Background()))
}

// TestTenureCarriesItsIdentity proves the work runs on a context that says
// what it is: the tenure's identity for every log line and statement, and
// the lease — its term readable, its transactions verified — for the world
// outside the database.
func TestTenureCarriesItsIdentity(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)

	observed := make(chan tenureObservation, 1)
	Register(func(ctx context.Context) error {
		term, leased := lease.TermFromContext(ctx)
		observed <- tenureObservation{identity: execctx.FromContext(ctx), term: term, leased: leased}
		<-ctx.Done()
		return nil
	}, "identified-work")
	startInstances(t, 1)

	select {
	case o := <-observed:
		require.Equal(t, "identified-work", o.identity.Leader)
		require.NotEmpty(t, o.identity.TraceID, "the tenure has a trace id of its own")
		require.True(t, o.leased, "the work runs under its lease")
		require.EqualValues(t, 1, o.term)
	case <-time.After(5 * time.Second):
		t.Fatal("the work did not run")
	}
}

// TestRegistrationErrorsFailStartup proves a registration that cannot be
// honored is neither dropped nor panicked on at registration time, but
// reported by start, all of them at once, so the process fails at startup.
func TestRegistrationErrorsFailStartup(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)

	Register(nil, "nil-work")
	Register(noopWork, "  ")
	Register(noopWork, "twice-work")
	Register(noopWork, "twice-work")

	err := start(context.Background())
	require.ErrorContains(t, err, `leader "nil-work": nil function`)
	require.ErrorContains(t, err, "leader: registered work has no name")
	require.ErrorContains(t, err, `leader "twice-work": registered twice`)
	require.Nil(t, current, "a failed start must leave no elector behind")
}

// TestRegisterAfterStartPanics proves a registration the elector can no
// longer campaign for fails fast instead of being silently dropped.
func TestRegisterAfterStartPanics(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)

	require.NoError(t, start(context.Background()))
	require.PanicsWithValue(t,
		`leader: "late-work" registered after the elector started; register leader work in package init functions`,
		func() { Register(noopWork, "late-work") })
}

// TestStartRefusesAClickhousePrimary proves a deployment that cannot
// coordinate fails at startup instead of running the work on every replica:
// on a ClickHouse primary database registered work fails the start, while a
// process registering none starts anywhere.
func TestStartRefusesAClickhousePrimary(t *testing.T) {
	withLeaderLoggerConfig(t)
	resetLeaderState(t)
	require.NoError(t, startOnClickhouse(), "a process registering no work needs no lease")

	resetLeaderState(t)
	Register(noopWork, "clustered-work")
	err := startOnClickhouse()
	require.ErrorIs(t, err, lease.ErrUnsupportedDatabase)
	require.ErrorContains(t, err, `leader "clustered-work" runs on one replica of the deployment`)
}

// startOnClickhouse starts the elector on a ClickHouse primary database — a
// connection handle that only names its dialect — and puts the suite's
// database back.
func startOnClickhouse() error {
	original := dbruntime.DB
	dbruntime.DB = &gorm.DB{Config: &gorm.Config{Dialector: clickhouseDialector{}}}
	defer func() { dbruntime.DB = original }()
	return start(context.Background())
}

// TestElectorIsALifecycleComponent proves importing the package is what
// enables the elections: init registered the elector as a lifecycle
// component whose Start, Stop and logger binding are this package's, so
// bootstrap starts it once the tables are ready, stops it as the process
// drains, and hands it the dedicated leader.log. The component is driven
// directly — a lifecycle stage starts once per process, which would make
// the test unrepeatable.
func TestElectorIsALifecycleComponent(t *testing.T) {
	dir := withLeaderLoggerConfig(t)
	resetLeaderState(t)

	var component lifecycle.Component
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		if c.Name == "leader" {
			component = c
		}
	}
	require.NotNil(t, component.Start, "the elector must register itself as a lifecycle component")
	require.NotNil(t, component.SetLogger, "the elector must take the dedicated logger the lifecycle binds")

	entered := make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		return nil
	}, "component-work")

	component.SetLogger(pkgzap.New("bound_leader.log"))
	require.NoError(t, component.Start(context.Background()))
	awaitSignal(t, entered, "the tenure")
	require.NoError(t, component.Stop(context.Background()))
	require.NotNil(t, current, "the elector must have been started through the component")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "bound_leader.log"), "elected leader")
	require.Equal(t, "component-work", entry["name"])
	require.NoFileExists(t, filepath.Join(dir, "leader.log"), "the bound logger replaces the package's own")
}

// tenureObservation is what the work saw on its context.
type tenureObservation struct {
	identity execctx.Identity
	term     uint64
	leased   bool
}

// clickhouseDialector stands in for a ClickHouse primary database: only its
// name is ever read, by the lease engine deciding whether it can run.
type clickhouseDialector struct {
	gorm.Dialector
}

func (clickhouseDialector) Name() string { return "clickhouse" }

// noopWork is work that returns at once, for tests about registration
// rather than running.
func noopWork(context.Context) error {
	return nil
}

// tenureLog counts the tenures of a work across every instance running it:
// how many began, how many run at once at most, and a signal per beginning.
type tenureLog struct {
	mu         sync.Mutex
	begun      int
	active     int
	mostAtOnce int
	began      chan struct{}
}

func newTenureLog() *tenureLog {
	return &tenureLog{began: make(chan struct{}, 64)}
}

// begin counts one tenure beginning.
func (l *tenureLog) begin() {
	l.mu.Lock()
	l.begun++
	l.active++
	l.mostAtOnce = max(l.mostAtOnce, l.active)
	l.mu.Unlock()
	l.began <- struct{}{}
}

// end counts one tenure ending.
func (l *tenureLog) end() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active--
}

// awaitBegin waits for one tenure to begin, failing the test when none does
// in time.
func (l *tenureLog) awaitBegin(t *testing.T) {
	t.Helper()
	awaitSignal(t, l.began, "a tenure")
}

// started returns how many tenures began.
func (l *tenureLog) started() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.begun
}

// maxActive returns the most tenures that ran at once.
func (l *tenureLog) maxActive() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.mostAtOnce
}

// startInstances builds and starts n electors over the registered work — n
// replicas sharing the primary database — and stops them once the test ends.
func startInstances(t *testing.T, n int) []*elector {
	t.Helper()

	if log == nil {
		log = pkgzap.New("leader.log")
	}
	instances := make([]*elector, 0, n)
	for range n {
		e := newElector(works)
		e.start(context.Background())
		instances = append(instances, e)
	}
	t.Cleanup(func() {
		for _, e := range instances {
			_ = e.stop(context.Background())
		}
	})
	return instances
}

// takeOver acts as another replica taking the name: the lease is ended in
// the table behind the holder's back and claimed anew. The claim is released
// once the test ends, unless the test released it first.
func takeOver(t *testing.T, name string) *lease.Handle {
	t.Helper()

	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", name).Error)
	h, claimed, err := lease.Claim(context.Background(), name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = h.Release(context.Background()) })
	return h
}

// awaitCause receives the next cause from causes, failing the test when none
// comes in time.
func awaitCause(t *testing.T, causes <-chan error) error {
	t.Helper()

	select {
	case cause := <-causes:
		return cause
	case <-time.After(5 * time.Second):
		t.Fatal("the work's context did not end")
		return nil
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

// withFastCampaign shrinks the campaign timings so a campaign plays out in
// milliseconds, and restores them afterwards.
func withFastCampaign(t *testing.T) {
	t.Helper()

	originalInterval, originalJitter := campaignInterval, campaignJitter
	campaignInterval, campaignJitter = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { campaignInterval, campaignJitter = originalInterval, originalJitter })
}

// withFastLease shrinks the lease protocol's timings so a loss is found in
// milliseconds, and restores them afterwards.
func withFastLease(t *testing.T) {
	t.Helper()

	t.Cleanup(lease.SetTimings(300*time.Millisecond, 50*time.Millisecond, 150*time.Millisecond))
}

// withShortGrace shrinks the grace work gets to return after losing its
// lease, and restores it afterwards.
func withShortGrace(t *testing.T) {
	t.Helper()

	original := stepDownGrace
	stepDownGrace = 100 * time.Millisecond
	t.Cleanup(func() { stepDownGrace = original })
}

// withLeaderLoggerConfig points config.App at a scratch logger setup so the
// loggers built during the test write under a temporary directory.
func withLeaderLoggerConfig(t *testing.T) string {
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

// resetLeaderState stops whatever the previous test left running, waits for
// its loops, rewinds the package-level elector state and empties the lease
// table, so each test exercises start from scratch. The loops this test
// starts are drained again once it ends, so none of them runs on into the
// next test; a process failure this test did not ask for fails it.
func resetLeaderState(t *testing.T) {
	t.Helper()

	drainLoops()
	t.Cleanup(drainLoops)

	mu.Lock()
	defer mu.Unlock()
	works = nil
	errRegister = nil
	log = nil
	current = nil
	fail = func(err error) { t.Errorf("unexpected process failure: %v", err) }
	require.NoError(t, dbruntime.DB.Exec("DELETE FROM gst_leases").Error)
}

// drainLoops ends the running loops, if any, and waits for them to return.
// Work that ignores its context has to be released by its test first.
func drainLoops() {
	mu.Lock()
	e := current
	mu.Unlock()
	if e != nil {
		e.cancel()
		<-e.done
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
