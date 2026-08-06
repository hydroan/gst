package zap

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	gorml "gorm.io/gorm/logger"
)

// newObservedGormLogger builds a GormLogger over an observer core so tests
// can assert on the entries Trace emits.
func newObservedGormLogger() (*GormLogger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return &GormLogger{l: &Logger{zlog: zap.New(core)}}, logs
}

// stubSlowQueryThreshold pins the slow-query threshold for one test so the
// level decision in Trace is deterministic regardless of config state.
func stubSlowQueryThreshold(t *testing.T, threshold time.Duration) {
	t.Helper()
	old := config.App.Database.SlowQueryThreshold
	config.App.Database.SlowQueryThreshold = threshold
	t.Cleanup(func() { config.App.Database.SlowQueryThreshold = old })
}

func requireSingleEntry(t *testing.T, logs *observer.ObservedLogs) observer.LoggedEntry {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	return entries[0]
}

func TestFrameworkSQLFramePredicate(t *testing.T) {
	tests := []struct {
		function string
		skip     bool
	}{
		{"gorm.io/gorm.(*DB).Find", true},
		{"gorm.io/gorm/callbacks.query", true},
		{"gorm.io/driver/mysql.Migrator.CurrentDatabase", true},
		{"github.com/hydroan/gst/logger/zap.(*GormLogger).Trace", true},
		{"github.com/hydroan/gst/database.(*database[go.shape.*uint8]).List", true},
		{"github.com/hydroan/gst/database.withWriteTransaction.func1", true},
		{"github.com/hydroan/gst/database/sqlite.New", true},
		{"github.com/hydroan/gst/dao.QueryModelsMapWithOptions", true},
		{"github.com/hydroan/gst/internal/controller.ListFactory.func1", false},
		{"github.com/hydroan/gst/model.(*Sample).CreateAfter", false},
		{"example.com/app/service/report.latestEntry", false},
		{"testing.tRunner", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.skip, isFrameworkSQLFrame(tt.function), tt.function)
	}
}

func TestCallerOutsideReturnsFirstUnskippedFrame(t *testing.T) {
	caller, ok := callerOutside(func(string) bool { return false })
	require.True(t, ok)
	require.Contains(t, caller, "gorm_test.go:")
}

func TestCallerOutsideReportsMissWhenEveryFrameSkipped(t *testing.T) {
	_, ok := callerOutside(func(string) bool { return true })
	require.False(t, ok)
}

func TestGormLoggerTraceLogsSuccessAtInfoWithCaller(t *testing.T) {
	stubSlowQueryThreshold(t, time.Hour)
	g, logs := newObservedGormLogger()

	g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)

	entry := requireSingleEntry(t, logs)
	require.Equal(t, zapcore.InfoLevel, entry.Level)
	require.Equal(t, "sql executed", entry.Message)
	fields := entry.ContextMap()
	require.Equal(t, "SELECT 1", fields["sql"])
	require.Equal(t, int64(1), fields["rows"])
	require.Contains(t, fields, "route")
	require.Contains(t, fields, "username")
	require.Contains(t, fields, "user_id")
	require.Contains(t, fields, "trace_id")
	// The test stack always reaches the Go test runner, which no framework
	// prefix matches, so a caller must resolve here.
	require.Contains(t, fields, "caller")
	require.NotContains(t, fields, "record_not_found")
}

func TestGormLoggerTraceKeepsRecordNotFoundAtInfo(t *testing.T) {
	stubSlowQueryThreshold(t, time.Hour)
	g, logs := newObservedGormLogger()

	g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, gorml.ErrRecordNotFound)

	entry := requireSingleEntry(t, logs)
	require.Equal(t, zapcore.InfoLevel, entry.Level)
	require.Equal(t, "sql executed", entry.Message)
	fields := entry.ContextMap()
	require.Equal(t, true, fields["record_not_found"])
	require.NotContains(t, fields, "error")
}

func TestGormLoggerTraceLogsFailureWithRequestFields(t *testing.T) {
	stubSlowQueryThreshold(t, time.Hour)
	g, logs := newObservedGormLogger()

	g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, errors.New("boom"))

	entry := requireSingleEntry(t, logs)
	require.Equal(t, zapcore.ErrorLevel, entry.Level)
	require.Equal(t, "sql failed", entry.Message)
	fields := entry.ContextMap()
	require.Contains(t, fields, "error")
	require.Contains(t, fields, "trace_id")
	require.Contains(t, fields, "caller")
	require.Equal(t, "SELECT 1", fields["sql"])
}

func TestGormLoggerTraceFlagsSlowQuery(t *testing.T) {
	stubSlowQueryThreshold(t, time.Nanosecond)
	g, logs := newObservedGormLogger()

	g.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) { return "SELECT 1", 0 }, nil)

	entry := requireSingleEntry(t, logs)
	require.Equal(t, zapcore.WarnLevel, entry.Level)
	require.Equal(t, "slow sql detected", entry.Message)
	require.Contains(t, entry.ContextMap(), "threshold")
}

func TestGormLoggerInfoFormatsArgs(t *testing.T) {
	g, logs := newObservedGormLogger()

	g.Info(context.Background(), "migrating %s", "samples")

	entry := requireSingleEntry(t, logs)
	require.Equal(t, "migrating samples", entry.Message)
}
