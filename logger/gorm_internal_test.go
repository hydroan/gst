package logger

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dbruntime/dbnode"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	gormlogger "gorm.io/gorm/logger"
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
	require.Contains(t, fields, "method")
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

	g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, gormlogger.ErrRecordNotFound)

	entry := requireSingleEntry(t, logs)
	require.Equal(t, zapcore.InfoLevel, entry.Level)
	require.Equal(t, "sql executed", entry.Message)
	fields := entry.ContextMap()
	require.Equal(t, true, fields["record_not_found"])
	require.NotContains(t, fields, "error")
}

// TestGormLoggerTraceLogsCanceledStatementAtInfo proves a statement its
// context canceled logs at info level as canceled, with no error: whoever
// canceled it — a client gone, a shutdown, a holder that stopped renewing its
// lease — ended it, and nothing failed in the database. A statement past its
// deadline still logs as a failure: the database took too long.
func TestGormLoggerTraceLogsCanceledStatementAtInfo(t *testing.T) {
	stubSlowQueryThreshold(t, time.Hour)

	t.Run("canceled", func(t *testing.T) {
		g, logs := newObservedGormLogger()

		g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, errors.Wrap(context.Canceled, "sample"))

		entry := requireSingleEntry(t, logs)
		require.Equal(t, zapcore.InfoLevel, entry.Level)
		require.Equal(t, "sql canceled", entry.Message)
		require.NotContains(t, entry.ContextMap(), "error")
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		g, logs := newObservedGormLogger()

		g.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, context.DeadlineExceeded)

		entry := requireSingleEntry(t, logs)
		require.Equal(t, zapcore.ErrorLevel, entry.Level)
		require.Equal(t, "sql failed", entry.Message)
	})
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
	require.Equal(t, time.Nanosecond, entry.ContextMap()["threshold"],
		"the threshold is a duration, rendered as integer nanoseconds like the duration it is measured against")
}

// TestTraceFieldsFitTheCapacityInTheWorstCase pins traceFieldCap to the entry
// Trace emits when every optional field is present — the caller, a cron
// round's identity, a replica role, record_not_found and the slow-query
// threshold — so a field added to Trace without bumping the capacity fails
// here instead of regrowing the slice on every statement.
func TestTraceFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	stubSlowQueryThreshold(t, time.Nanosecond)
	g, logs := newObservedGormLogger()

	ctx := execctx.WithCronjob(context.Background(), "sample_job", "trace-worst")
	ctx = dbnode.WithRole(ctx, "replica")
	g.Trace(ctx, time.Now().Add(-time.Second), func() (string, int64) { return "SELECT 1", 0 }, gormlogger.ErrRecordNotFound)

	entry := requireSingleEntry(t, logs)
	require.Len(t, entry.Context, traceFieldCap,
		"the worst case must fill the capacity exactly: a new field bumps traceFieldCap, a dropped one lowers it")
	fields := entry.ContextMap()
	for _, key := range []string{"caller", "cronjob", "db_role", "record_not_found", "threshold"} {
		require.Contains(t, fields, key, "the worst case must carry every optional field")
	}
}

func TestGormLoggerInfoFormatsArgs(t *testing.T) {
	g, logs := newObservedGormLogger()

	g.Info(context.Background(), "migrating %s", "samples")

	entry := requireSingleEntry(t, logs)
	require.Equal(t, "migrating samples", entry.Message)
}

func TestGormTraceUsesMetadata(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := requestctx.New(requestctx.Fields{
		Method:   http.MethodPost,
		Username: "admin",
		UserID:   "user-1",
	})
	ctx := execctx.WithTraceID(requestctx.WithMetadata(context.Background(), meta), "trace-1")

	oldThreshold := config.App.Database.SlowQueryThreshold
	config.App.Database.SlowQueryThreshold = time.Hour
	t.Cleanup(func() {
		config.App.Database.SlowQueryThreshold = oldThreshold
	})

	gormLog := &GormLogger{l: log}
	gormLog.Trace(ctx, time.Now(), func() (string, int64) {
		return "select 1", 1
	}, nil)

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.Equal(t, http.MethodPost, fields[consts.CTX_METHOD])
	require.Equal(t, "admin", fields[consts.CTX_USERNAME])
	require.Equal(t, "user-1", fields[consts.CTX_USER_ID])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
	require.Equal(t, "select 1", fields["sql"])
	require.Equal(t, int64(1), fields["rows"])
	require.NotContains(t, fields, consts.CRONJOB)
}

func TestGormTraceAddsCronjobField(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	ctx := execctx.WithCronjob(context.Background(), "sample_job", "trace-cron")

	oldThreshold := config.App.Database.SlowQueryThreshold
	config.App.Database.SlowQueryThreshold = time.Hour
	t.Cleanup(func() {
		config.App.Database.SlowQueryThreshold = oldThreshold
	})

	gormLog := &GormLogger{l: log}
	gormLog.Trace(ctx, time.Now(), func() (string, int64) {
		return "select 1", 1
	}, nil)

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.Equal(t, "trace-cron", fields[consts.TRACE_ID])
	require.Equal(t, "sample_job", fields[consts.CRONJOB])
}
