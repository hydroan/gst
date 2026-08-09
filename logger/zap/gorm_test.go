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
		// Prefixes match at a package boundary: a sibling package sharing the
		// prefix as a name prefix is not a framework frame.
		{"github.com/hydroan/gst/daox.Migrate", false},
		{"github.com/hydroan/gst/databases.New", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.skip, isFrameworkSQLFrame(tt.function), tt.function)
	}
}

func TestHasPackagePrefix(t *testing.T) {
	tests := []struct {
		function string
		prefix   string
		match    bool
	}{
		// Package boundary: "." starts a symbol, "/" starts a subpackage.
		{"example.com/app/dao.Query", "example.com/app/dao", true},
		{"example.com/app/dao.(*Query).List", "example.com/app/dao", true},
		{"example.com/app/dao/sub.Helper", "example.com/app/dao", true},
		{"example.com/app/daox.Query", "example.com/app/dao", false},
		{"example.com/app/service.Load", "example.com/app/dao", false},
		// A prefix already ending in "/" or "." matches by plain prefix.
		{"gorm.io/gorm.(*DB).Find", "gorm.io/", true},
		{"example.com/app/dao.Query", "example.com/app/dao.", true},
		// Exact equality counts as a match.
		{"example.com/app/dao", "example.com/app/dao", true},
		// An empty prefix never matches: a stray empty configuration entry
		// must not swallow every frame and erase the caller field.
		{"example.com/app/dao.Query", "", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.match, hasPackagePrefix(tt.function, tt.prefix), "%s vs %s", tt.function, tt.prefix)
	}
}

// stubSQLCallerSkipPrefixes pins the configured caller-skip prefixes for one
// test or benchmark so the combined skip predicate is deterministic
// regardless of config state.
func stubSQLCallerSkipPrefixes(t testing.TB, prefixes []string) {
	t.Helper()
	old := config.App.Logger.SQLCallerSkipPrefixes
	config.App.Logger.SQLCallerSkipPrefixes = prefixes
	t.Cleanup(func() { config.App.Logger.SQLCallerSkipPrefixes = old })
}

func TestSkippedSQLFramePredicateHonorsConfiguredPrefixes(t *testing.T) {
	// The second entry keeps the surrounding whitespace a comma-separated
	// environment value carries after splitting; matching must not depend on
	// the operator remembering to avoid spaces.
	stubSQLCallerSkipPrefixes(t, []string{"example.com/app/dao", " example.com/app/repo "})

	tests := []struct {
		function string
		skip     bool
	}{
		// Configured project helper packages are skipped like framework ones.
		{"example.com/app/dao.QueryHelper", true},
		{"example.com/app/dao.(*Query).List", true},
		{"example.com/app/dao/sub.Helper", true},
		{"example.com/app/repo.Load", true},
		// Package-boundary matching also applies to configured prefixes.
		{"example.com/app/daox.Query", false},
		{"example.com/app/service.Load", false},
		// Built-in prefixes stay in force and cannot be configured away.
		{"gorm.io/gorm.(*DB).Find", true},
		{"github.com/hydroan/gst/database.(*database[go.shape.*uint8]).List", true},
	}
	for _, tt := range tests {
		require.Equal(t, tt.skip, isSkippedSQLFrame(tt.function), tt.function)
	}
}

func TestSkippedSQLFramePredicateWithoutConfigMatchesFrameworkPredicate(t *testing.T) {
	stubSQLCallerSkipPrefixes(t, nil)

	for _, function := range []string{
		"gorm.io/gorm.(*DB).Find",
		"github.com/hydroan/gst/dao.QueryModelsMapWithOptions",
		"example.com/app/dao.QueryHelper",
		"example.com/app/service/report.latestEntry",
	} {
		require.Equal(t, isFrameworkSQLFrame(function), isSkippedSQLFrame(function), function)
	}
}

// nestedCallerOutside pads the stack with skipped frames before resolving the
// caller: every level lives in this package, which the framework prefix list
// skips, so a depth of N reproduces the N wrapper frames sitting between a
// real statement log and the business caller.
//
//go:noinline
func nestedCallerOutside(depth int, skip func(function string) bool) (string, bool) {
	if depth == 0 {
		return callerOutside(skip)
	}
	return nestedCallerOutside(depth-1, skip)
}

// BenchmarkCallerOutsideSkippedSQLFrames measures the per-statement cost of
// resolving the business caller through a realistic stack of framework
// wrapper frames, without configured prefixes.
func BenchmarkCallerOutsideSkippedSQLFrames(b *testing.B) {
	stubSQLCallerSkipPrefixes(b, nil)

	b.ReportAllocs()
	for b.Loop() {
		nestedCallerOutside(12, isSkippedSQLFrame)
	}
}

// BenchmarkCallerOutsideSkippedSQLFramesConfigured is the same walk with two
// configured project prefixes, isolating the cost the configuration adds.
func BenchmarkCallerOutsideSkippedSQLFramesConfigured(b *testing.B) {
	stubSQLCallerSkipPrefixes(b, []string{"example.com/app/dao", "example.com/app/repo"})

	b.ReportAllocs()
	for b.Loop() {
		nestedCallerOutside(12, isSkippedSQLFrame)
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
