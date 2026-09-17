package cronjob

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// roundObservation is what a job sees of its round: the identity on its
// context and the span it runs under.
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
// names at as the last instant claimed, its round run to its end, the way a
// job that ran before looks to a starting scheduler.
func lastRun(t *testing.T, leaseName string, at time.Time) {
	t.Helper()

	h, claimed, err := lease.ClaimSlot(context.Background(), leaseName, at)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, h.Finish(context.Background(), time.Time{}))
}

// cutShortRun puts on record a round of a job cut short, the way a replica
// leaves it: at is the last instant claimed, its round never ran to its end,
// and the lease is given back — as a shutdown gives it back — or, when
// crashed, left to expire, as the lease of a process that died is.
func cutShortRun(t *testing.T, leaseName string, at time.Time, crashed bool) {
	t.Helper()

	h, claimed, err := lease.ClaimSlot(context.Background(), leaseName, at)
	require.NoError(t, err)
	require.True(t, claimed)
	if !crashed {
		require.NoError(t, h.Release(context.Background()))
	}
}

// withUnreachableDatabase hands the scheduler a primary database whose
// connections are closed, so that every statement fails, and puts the suite's
// database back once the test ends. The loops read the database, so they must
// be drained before it changes back: call it before the helpers whose
// cleanups drain them, withFakeClock and withFastSweep.
func withUnreachableDatabase(t *testing.T) {
	t.Helper()

	closed, err := gorm.Open(gormsqlite.Open("file::memory:?cache=private"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	pool, err := closed.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())

	original := dbruntime.DB
	dbruntime.DB = closed
	t.Cleanup(func() { dbruntime.DB = original })
}

// withFirstFinishFailing has the first statement recording a round under
// leaseName as finished fail, the way a database dropping it would, and takes
// the failure out once the test ends. The loops run their statements through
// what it changes, so call it before the helpers whose cleanups drain them,
// withFakeClock and withFastSweep.
func withFirstFinishFailing(t *testing.T, leaseName string) {
	t.Helper()

	var failed atomic.Bool
	const callback = "test:fail_first_finish"
	require.NoError(t, dbruntime.DB.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE gst_leases SET expires_at_ms = 0, unfinished_slot_ms = 0") &&
			slices.Contains(tx.Statement.Vars, any(leaseName)) && failed.CompareAndSwap(false, true) {
			_ = tx.AddError(errors.New("sample failure"))
		}
	}))
	t.Cleanup(func() { _ = dbruntime.DB.Callback().Raw().Remove(callback) })
}

// withFastSweep has the schedulers the test starts look for rounds cut short
// every few milliseconds, and restores the sweep's timings afterwards.
func withFastSweep(t *testing.T) {
	t.Helper()

	interval, jitter := sweepInterval, sweepJitter
	sweepInterval, sweepJitter = 10*time.Millisecond, 0
	// The loops read the timings; they must be gone before they change back.
	t.Cleanup(func() {
		drainLoops()
		sweepInterval, sweepJitter = interval, jitter
	})
}

// unfinishedInstants returns the instants under the job's lease name that a
// sweep would hand to a loop to run a second time.
func unfinishedInstants(t *testing.T, leaseName string) []time.Time {
	t.Helper()

	found, err := lease.UnfinishedSlots(context.Background(), []string{leaseName})
	require.NoError(t, err)
	instants := make([]time.Time, 0, len(found))
	for _, u := range found {
		instants = append(instants, u.Slot)
	}
	return instants
}

// leaseRow is what a test reads of a job's lease row: the last instant
// claimed, the instant whose round has not run to its end, and when the lease
// expires — 0 once it was given back.
type leaseRow struct {
	SlotMs           int64
	UnfinishedSlotMs int64
	ExpiresAtMs      int64
}

// readLeaseRow reads the lease row of a job under its lease name.
func readLeaseRow(t *testing.T, leaseName string) leaseRow {
	t.Helper()

	var row leaseRow
	require.NoError(t, dbruntime.DB.Raw("SELECT slot_ms, unfinished_slot_ms, expires_at_ms FROM gst_leases WHERE name = ?", leaseName).Scan(&row).Error)
	return row
}

// takeOver acts as another replica taking the name: the lease is ended in
// the table behind the holder's back and claimed anew. The claim is released
// once the test ends.
func takeOver(t *testing.T, name string) {
	t.Helper()

	endLease(t, name)
	taken, claimed, err := lease.Claim(context.Background(), name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = taken.Release(context.Background()) })
}

// endLease ends the lease of name in the table, whoever holds it: an
// operator's hand, or the replica that took the name over giving it back.
func endLease(t *testing.T, name string) {
	t.Helper()

	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", name).Error)
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

	withBoundCronjobLogger(t)
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
// test moves it, and a timer fires the moment the test moves past its
// instant.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
	// waiting is signaled whenever a timer is set, so a test can hold the
	// clock still until the loops are waiting again.
	waiting chan struct{}
}

// fakeWaiter is one pending timer: the instant it waits for and the channel
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

func (c *fakeClock) Timer(t time.Time) (<-chan struct{}, func()) {
	wake := make(chan struct{})
	c.mu.Lock()
	if !t.After(c.now) {
		c.mu.Unlock()
		close(wake)
		return wake, func() {}
	}
	c.waiters = append(c.waiters, fakeWaiter{at: t, wake: wake})
	c.mu.Unlock()
	select {
	case c.waiting <- struct{}{}:
	default:
	}

	// A timer stopped leaves the clock, so that untilWaiters counts only
	// waits that are still pending.
	return wake, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.waiters = slices.DeleteFunc(c.waiters, func(w fakeWaiter) bool { return w.wake == wake })
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

// withBoundCronjobLogger stands in for the lifecycle, which binds the
// dedicated cronjob.log before it starts the component, so the scheduler's
// entries land in a file the test can read back. A test that already bound
// one keeps it: a second logger on the file would open a second rotation
// instance on it.
func withBoundCronjobLogger(t *testing.T) {
	t.Helper()

	mu.Lock()
	defer mu.Unlock()
	if log == nil {
		log = pkgzap.New("cronjob.log")
	}
}

// withObservedGlobalLogger routes the global logger — the stream the
// package's fallback logger writes to — into an observer for the test, and
// restores the previous one afterwards.
func withObservedGlobalLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()

	core, logs := observer.New(zapcore.InfoLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(restore)
	return logs
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

	entries := readLogEntries(t, path, msg)
	if len(entries) == 0 {
		require.Failf(t, "missing log entry", "no entry with msg %q in %s", msg, path)
		return nil
	}
	return entries[0]
}

// readLogEntries returns every JSON entry of the log file whose msg field
// equals msg, in the order they were written.
func readLogEntries(t *testing.T, path, msg string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		entry := make(map[string]any)
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "log line must be JSON: %s", line)
		if entry["msg"] == msg {
			entries = append(entries, entry)
		}
	}
	return entries
}
