package cronjob

import (
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
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// TestInitAdoptsSharedCronjobLogger proves scheduling logs flow through the
// shared logger.Cronjob instance instead of a second package-local logger on
// the same file, which would race lumberjack rotation against it.
func TestInitAdoptsSharedCronjobLogger(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)

	shared := pkgzap.New("shared_cronjob.log")
	original := logger.Cronjob
	logger.Cronjob = shared
	t.Cleanup(func() { logger.Cronjob = original })

	Register(func() error { return nil }, "0 0 * * * *", "sample-job")
	require.NoError(t, Init())
	pkgzap.Clean()

	data, err := os.ReadFile(filepath.Join(dir, "shared_cronjob.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "successfully add cronjob",
		"scheduling must log through the shared cronjob logger")
	require.NoFileExists(t, filepath.Join(dir, "cronjob.log"),
		"no package-local logger may open the shared log file")
}

// TestInitFallsBackToLocalLoggerWithoutShared keeps the pre-existing
// behavior for processes that never ran the logging setup (unit tests):
// scheduling still logs through a package-local logger.
func TestInitFallsBackToLocalLoggerWithoutShared(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)

	original := logger.Cronjob
	logger.Cronjob = nil
	t.Cleanup(func() { logger.Cronjob = original })

	Register(func() error { return nil }, "0 0 * * * *", "fallback-job")
	require.NoError(t, Init())
	pkgzap.Clean()

	data, err := os.ReadFile(filepath.Join(dir, "cronjob.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "successfully add cronjob")
}

// TestStopWaitsForInFlightJob proves Stop halts scheduling and blocks until
// a job that is already running finishes, so shutdown cannot tear the
// connections out from under a half-done job.
func TestStopWaitsForInFlightJob(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)

	var startOnce, doneOnce sync.Once
	jobStarted := make(chan struct{})
	jobDone := make(chan struct{})
	Register(func() error {
		startOnce.Do(func() { close(jobStarted) })
		time.Sleep(300 * time.Millisecond)
		doneOnce.Do(func() { close(jobDone) })
		return nil
	}, "* * * * * *", "inflight-job")
	require.NoError(t, Init())

	select {
	case <-jobStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("the scheduled job never started")
	}

	Stop()

	select {
	case <-jobDone:
	default:
		t.Fatal("Stop returned before the in-flight job finished")
	}
}

// TestStopGivesUpOnStuckJob proves a job that never finishes cannot hold the
// shutdown hostage: Stop returns once the bounded wait elapses.
func TestStopGivesUpOnStuckJob(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)

	originalTimeout := stopTimeout
	stopTimeout = 100 * time.Millisecond
	t.Cleanup(func() { stopTimeout = originalTimeout })

	var startOnce sync.Once
	jobStarted := make(chan struct{})
	Register(func() error {
		startOnce.Do(func() { close(jobStarted) })
		time.Sleep(10 * time.Second)
		return nil
	}, "* * * * * *", "stuck-job")
	require.NoError(t, Init())

	select {
	case <-jobStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("the scheduled job never started")
	}

	begin := time.Now()
	Stop()
	require.Less(t, time.Since(begin), 2*time.Second,
		"Stop must return once the bounded wait elapses")
}

// TestStopWithoutInitIsNoop keeps Stop safe in processes that never started
// the scheduler.
func TestStopWithoutInitIsNoop(t *testing.T) {
	resetCronjobState(t)

	Stop()
}

// TestScheduledRunsSkipWhileStillRunning proves overlapping ticks are dropped
// while a run is still in flight: a slow job on a fast schedule must never run
// concurrently with itself. Without the guard a slow round piles new rounds on
// top of it, multiplying its downstream calls and interleaving its logs.
func TestScheduledRunsSkipWhileStillRunning(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)

	var running, maxRunning atomic.Int32
	var startOnce sync.Once
	started := make(chan struct{})
	block := make(chan struct{})
	Register(func() error {
		observeConcurrentRuns(&running, &maxRunning)
		startOnce.Do(func() { close(started) })
		<-block
		running.Add(-1)
		return nil
	}, "* * * * * *", "overlap-job")
	require.NoError(t, Init())

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("the scheduled job never started")
	}
	// Hold the first run across at least two more scheduling ticks, then let
	// everything finish so Stop does not have to wait out its timeout.
	time.Sleep(2200 * time.Millisecond)
	close(block)
	Stop()

	require.EqualValues(t, 1, maxRunning.Load(),
		"ticks firing while a run is in flight must be skipped, not piled on top of it")
}

// TestImmediateRunSharesSkipMutex proves the immediate run and the scheduled
// runs hold the same in-flight guard: a slow immediate run on a fast schedule
// must not race the first tick.
func TestImmediateRunSharesSkipMutex(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)

	var running, maxRunning atomic.Int32
	var startOnce sync.Once
	started := make(chan struct{})
	Register(func() error {
		observeConcurrentRuns(&running, &maxRunning)
		startOnce.Do(func() { close(started) })
		time.Sleep(1500 * time.Millisecond)
		running.Add(-1)
		return nil
	}, "* * * * * *", "immediate-overlap-job", Config{RunImmediately: true})
	require.NoError(t, Init())

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("the immediate run never started")
	}
	time.Sleep(1800 * time.Millisecond)
	Stop()

	require.EqualValues(t, 1, maxRunning.Load(),
		"the immediate run must hold the same guard as scheduled runs")
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

			var startOnce sync.Once
			started := make(chan struct{})
			Register(func() error {
				startOnce.Do(func() { close(started) })
				return tc.job()
			}, "* * * * * *", "failing-job")
			require.NoError(t, Init())

			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("the scheduled job never started")
			}
			// Stop waits for the in-flight round, whose outcome entry is
			// written before the round returns.
			Stop()
			pkgzap.Clean()

			entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), tc.msg)
			require.Equal(t, tc.err, entry["error"])
			require.Contains(t, entry["error_stack"], "cronjob_test.go",
				"error_stack must point at the line inside the job that failed")
		})
	}
}

// observeConcurrentRuns bumps the number of in-flight runs and records the
// highest concurrency seen across the test.
func observeConcurrentRuns(running, maxRunning *atomic.Int32) {
	cur := running.Add(1)
	for {
		seen := maxRunning.Load()
		if cur <= seen || maxRunning.CompareAndSwap(seen, cur) {
			return
		}
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

// resetCronjobState rewinds the package-level scheduler state so each test
// exercises Init from scratch.
func resetCronjobState(t *testing.T) {
	t.Helper()

	if c != nil {
		c.Stop()
	}
	c = nil
	log = nil
	cronjobs = nil
	inited = false
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
