package cronjob

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
