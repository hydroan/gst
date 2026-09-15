package lock

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	logpkg "github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestMain gives the suite a primary database — the dialect under test, so
// the Makefile test target runs the scenarios on every dialect the lease
// engine reads a clock from — prepared the way bootstrap's first phase would,
// without the lifecycle: the locks under test are checked by the tests
// themselves.
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

	logpkg.Gorm = gormlogger.Discard
	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := errors.Join(sqlite.Init(), mysql.Init(), postgres.Init()); err != nil {
		panic(err)
	}
	dbruntime.Wait()
	return m.Run()
}

// TestTryRunRunsTheWorkUnderTheLock proves the ordinary try: the work runs
// under the lease — its term readable from the context, the caller's
// identity kept — and the name is free again the moment the work returns.
func TestTryRunRunsTheWorkUnderTheLock(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	l := New("sample-work")

	ctx := execctx.WithTraceID(context.Background(), "trace-lock")
	var seen tryObservation
	require.NoError(t, l.TryRun(ctx, func(ctx context.Context) error {
		term, leased := lease.TermFromContext(ctx)
		seen = tryObservation{ran: true, term: term, leased: leased, identity: execctx.FromContext(ctx)}
		return nil
	}))

	require.True(t, seen.ran)
	require.True(t, seen.leased, "the work runs under its lease")
	require.EqualValues(t, 1, seen.term)
	require.Equal(t, "trace-lock", seen.identity.TraceID, "the work keeps the caller's identity")

	h, claimed, err := lease.Claim(context.Background(), "lock:sample-work")
	require.NoError(t, err)
	require.True(t, claimed, "the name must be free the moment the work returned")
	require.NoError(t, h.Release(context.Background()))
}

// TestTryRunRefusesAHeldLock proves the try never waits: while the work runs
// under the lock, another try is refused at once with ErrHeld, and once the
// work has returned the next try runs.
func TestTryRunRefusesAHeldLock(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	l := New("held-work")

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- l.TryRun(context.Background(), func(context.Context) error {
			entered <- struct{}{}
			<-release
			return nil
		})
	}()
	awaitSignal(t, entered, "the work")

	begin := time.Now()
	require.ErrorIs(t, l.TryRun(context.Background(), noopWork), ErrHeld)
	require.Less(t, time.Since(begin), time.Second, "a held lock must be refused at once")

	close(release)
	require.NoError(t, awaitError(t, first))
	require.NoError(t, l.TryRun(context.Background(), noopWork), "the next try runs once the work has returned")
}

// TestTryRunReturnsTheWorkErrorAndRecoversAPanic proves the work's outcome
// is the try's — its error as is, a panic as an error carrying its stack.
func TestTryRunReturnsTheWorkErrorAndRecoversAPanic(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	l := New("failing-work")

	require.ErrorContains(t, l.TryRun(context.Background(), func(context.Context) error { return errors.New("sample failure") }), "sample failure")
	err := l.TryRun(context.Background(), func(context.Context) error { panic("sample panic") })
	require.ErrorContains(t, err, "sample panic")
	require.Contains(t, fmt.Sprintf("%+v", err), "lock_test.go", "the error must carry the stack of the panic site")
}

// TestTryRunReportsLostEvenWhenTheWorkSucceeded proves a try ends with its
// lease: once another holder has taken the name — the lease ended behind the
// work's back — the work's context ends with ErrLost as the cause, and the
// try reports ErrLost even though the work returned nothing, since another
// holder may have started the same work since.
func TestTryRunReportsLostEvenWhenTheWorkSucceeded(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	withFastLease(t)
	l := New("lost-work")

	entered := make(chan struct{}, 1)
	causes := make(chan error, 1)
	tried := make(chan error, 1)
	go func() {
		tried <- l.TryRun(context.Background(), func(ctx context.Context) error {
			entered <- struct{}{}
			<-ctx.Done()
			causes <- context.Cause(ctx)
			return nil
		})
	}()
	awaitSignal(t, entered, "the work")

	takeOver(t, "lock:lost-work")
	require.ErrorIs(t, awaitError(t, causes), lease.ErrLost)
	require.ErrorIs(t, awaitError(t, tried), ErrLost, "work cut short by a lost lease is reported lost, whatever it returned")
}

// TestDeclarationErrorsFailStartup proves a declaration that cannot be
// honored is neither dropped nor panicked on at declaration time, but
// reported by start, all of them at once, so the process fails at startup.
func TestDeclarationErrorsFailStartup(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)

	New("  ")
	New("twice-work")
	New("twice-work")
	// The lease table holds 191 characters, "lock:" included.
	New(strings.Repeat("n", 187))

	err := start(context.Background())
	require.ErrorContains(t, err, "lock: declared lock has no name")
	require.ErrorContains(t, err, `lock "twice-work": declared twice`)
	require.ErrorContains(t, err, "longer than the 191 the name column holds")
	require.False(t, started, "a failed start must not mark the locks checked")
}

// TestNewAfterStartPanics proves a declaration the start could no longer
// check fails fast instead of being silently accepted.
func TestNewAfterStartPanics(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)

	require.NoError(t, start(context.Background()))
	require.PanicsWithValue(t,
		`lock: "late-work" declared after the locks were checked; declare locks in package variables`,
		func() { New("late-work") })
}

// TestStartRefusesAClickhousePrimary proves a deployment that cannot
// coordinate fails at startup instead of running the work everywhere: on a
// ClickHouse primary database a declared lock fails the start, while a
// process declaring none starts anywhere.
func TestStartRefusesAClickhousePrimary(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	require.NoError(t, startOnClickhouse(), "a process declaring no lock needs no lease")

	resetLockState(t)
	New("clustered-work")
	err := startOnClickhouse()
	require.ErrorIs(t, err, lease.ErrUnsupportedDatabase)
	require.ErrorContains(t, err, `lock "clustered-work" runs its work once at a time across the deployment`)
}

// startOnClickhouse checks the locks on a ClickHouse primary database — a
// connection handle that only names its dialect — and puts the suite's
// database back.
func startOnClickhouse() error {
	original := dbruntime.DB
	dbruntime.DB = &gorm.DB{Config: &gorm.Config{Dialector: clickhouseDialector{}}}
	defer func() { dbruntime.DB = original }()
	return start(context.Background())
}

// TestLocksAreALifecycleComponent proves importing the package is what
// enables locks: init registered them as a lifecycle component whose Start
// and logger binding are this package's, so bootstrap checks the
// declarations once the tables are ready and hands the package the dedicated
// lock.log. The component is driven directly — a lifecycle stage starts once
// per process, which would make the test unrepeatable.
func TestLocksAreALifecycleComponent(t *testing.T) {
	dir := withLockLoggerConfig(t)
	resetLockState(t)

	var component lifecycle.Component
	for _, c := range lifecycle.Components(lifecycle.StageComponent) {
		if c.Name == "lock" {
			component = c
		}
	}
	require.NotNil(t, component.Start, "the locks must register themselves as a lifecycle component")
	require.NotNil(t, component.SetLogger, "the package must take the dedicated logger the lifecycle binds")
	require.Nil(t, component.Stop, "a lock holds nothing between tries, so there is nothing to stop")

	New("component-work")
	component.SetLogger(pkgzap.New("bound_lock.log"))
	require.NoError(t, component.Start(context.Background()))
	require.True(t, started, "the locks must have been checked through the component")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "bound_lock.log"), "declared lock")
	require.Equal(t, "component-work", entry["name"])
	require.NoFileExists(t, filepath.Join(dir, "lock.log"), "the bound logger replaces the package's own")
}

// TestWorkThatIgnoresTheLossFailsTheProcess proves the last line behind the
// lease reaches a lock's work too: work still running once its lease is lost
// and the grace has passed would run beside the new holder's, so the process
// is failed. The failure is recorded instead of tripping the process-wide
// one, which is one-way and would keep the test from running twice.
func TestWorkThatIgnoresTheLossFailsTheProcess(t *testing.T) {
	withLockLoggerConfig(t)
	resetLockState(t)
	withFastLease(t)
	failures := withRecordedFailures(t)
	l := New("stubborn-work")

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	tried := make(chan error, 1)
	go func() {
		tried <- l.TryRun(context.Background(), func(context.Context) error {
			entered <- struct{}{}
			<-release
			return nil
		})
	}()
	awaitSignal(t, entered, "the work")

	takeOver(t, "lock:stubborn-work")
	select {
	case err := <-failures:
		require.ErrorContains(t, err, `lease "lock:stubborn-work" was lost`)
	case <-time.After(5 * time.Second):
		t.Fatal("work ignoring the loss must fail the process")
	}
	close(release)
	require.ErrorIs(t, awaitError(t, tried), ErrLost)
}

// tryObservation is what the work saw on its context.
type tryObservation struct {
	ran      bool
	term     uint64
	leased   bool
	identity execctx.Identity
}

// clickhouseDialector stands in for a ClickHouse primary database: only its
// name is ever read, by the lease engine deciding whether it can run.
type clickhouseDialector struct {
	gorm.Dialector
}

func (clickhouseDialector) Name() string { return "clickhouse" }

// noopWork is work that returns at once, for tests about the try rather than
// the work.
func noopWork(context.Context) error {
	return nil
}

// takeOver acts as another holder taking the name: the lease is ended in the
// table behind the holder's back and claimed anew. The claim is released once
// the test ends.
func takeOver(t *testing.T, name string) {
	t.Helper()

	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", name).Error)
	h, claimed, err := lease.Claim(context.Background(), name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = h.Release(context.Background()) })
}

// awaitError receives the next error from errs, failing the test when none
// comes in time.
func awaitError(t *testing.T, errs <-chan error) error {
	t.Helper()

	select {
	case err := <-errs:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the try did not end")
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

// withLockLoggerConfig points config.App at a scratch logger setup so the
// loggers built during the test write under a temporary directory.
func withLockLoggerConfig(t *testing.T) string {
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

// resetLockState rewinds the package-level state and empties the lease
// table, so each test declares its locks from scratch.
func resetLockState(t *testing.T) {
	t.Helper()

	mu.Lock()
	defer mu.Unlock()
	locks = nil
	errDeclare = nil
	log = nil
	started = false
	require.NoError(t, dbruntime.DB.Exec("DELETE FROM gst_leases").Error)
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
