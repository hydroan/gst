package logger

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestFrameworkSQLFramePredicate(t *testing.T) {
	tests := []struct {
		function string
		skip     bool
	}{
		{"gorm.io/gorm.(*DB).Find", true},
		{"gorm.io/gorm/callbacks.query", true},
		{"gorm.io/driver/mysql.Migrator.CurrentDatabase", true},
		{"github.com/hydroan/gst/logger.(*GormLogger).Trace", true},
		{"github.com/hydroan/gst/database.(*database[go.shape.*uint8]).List", true},
		{"github.com/hydroan/gst/database.withWriteTransaction.func1", true},
		{"github.com/hydroan/gst/database/sqlite.New", true},
		{"github.com/hydroan/gst/dao.QueryModelsMapWithOptions", true},
		{"github.com/hydroan/gst/internal/controller.ListHandler.func1", false},
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
		// Ending a prefix in "." is how a project skips one package without
		// its subpackages: the subpackage separator is not the "." the prefix
		// demands.
		{"example.com/app/dao/sub.Helper", "example.com/app/dao.", false},
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
func stubSQLCallerSkipPrefixes(tb testing.TB, prefixes []string) {
	tb.Helper()
	old := config.App.Logger.SQLCallerSkipPrefixes
	config.App.Logger.SQLCallerSkipPrefixes = prefixes
	tb.Cleanup(func() { config.App.Logger.SQLCallerSkipPrefixes = old })
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
	require.Contains(t, caller, "sqlcaller_internal_test.go:")
}

func TestCallerOutsideReportsMissWhenEveryFrameSkipped(t *testing.T) {
	_, ok := callerOutside(func(string) bool { return true })
	require.False(t, ok)
}

// TestCallerOutsideFollowsConfiguredPrefixesAcrossCalls pins the boundary of the
// caller path cache: only the formatting of a resolved position is remembered,
// never which frame the walk stops at. Configuring another prefix must move the
// answer to the next frame out on the very next call, so an operator adding a
// helper package to logger.sql_caller_skip_prefixes still sees the caller they
// asked for rather than a position cached before the change.
func TestCallerOutsideFollowsConfiguredPrefixesAcrossCalls(t *testing.T) {
	// Every frame from here up to the test runner belongs to the framework, so
	// the walk lands on testing while nothing extra is configured.
	stubSQLCallerSkipPrefixes(t, nil)
	unconfigured, ok := callerOutside(isSkippedSQLFrame)
	require.True(t, ok)
	require.Contains(t, unconfigured, "testing.go:")

	// Skipping the runner too has to push the walk one frame further out; a
	// cache keyed on anything but the position would replay the answer above.
	stubSQLCallerSkipPrefixes(t, []string{"testing"})
	configured, ok := callerOutside(isSkippedSQLFrame)
	require.True(t, ok)
	require.NotEqual(t, unconfigured, configured)
	require.NotContains(t, configured, "testing.go:")

	// Removing the prefix restores the earlier caller, proving the first answer
	// was not pinned by having been cached.
	stubSQLCallerSkipPrefixes(t, nil)
	restored, ok := callerOutside(isSkippedSQLFrame)
	require.True(t, ok)
	require.Equal(t, unconfigured, restored)
}
