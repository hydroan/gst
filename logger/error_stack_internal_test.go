package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/errorstack"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// errSentinel is a package-level error: the stack it carries was captured
// while the package initialized, the one errorstack treats as absent.
var errSentinel = errors.New("sentinel failure")

func TestErrorAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Error(newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "stack traced failure")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
	require.Contains(t, stackTrace, "error_stack_internal_test.go")
}

func TestErrorfAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Errorf("operation failed: %v", newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "operation failed: stack traced failure")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
}

func TestErrorwAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Errorw("operation failed", "error", newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "operation failed")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
}

func TestErrorzAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Errorz("operation failed", zap.Error(newStackTracedError()))

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "operation failed")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
}

func TestErrorzSkipsErrorStackFieldWhenNoErrorField(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Errorz("operation failed", zap.String("key", "value"))

	entries := logs.All()
	require.Len(t, entries, 1)

	for _, field := range entries[0].Context {
		require.NotEqual(t, errorStackKey, field.Key)
	}
}

func TestErrorSkipsErrorStackFieldWhenErrorHasNoStackTrace(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Error(plainError{})

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "plain failure")

	for _, field := range entries[0].Context {
		require.NotEqual(t, errorStackKey, field.Key)
	}
}

func TestErrorSkipsErrorStackFieldWhenDisabledByConfig(t *testing.T) {
	old := config.App.Logger.ErrorStackDisabled
	config.App.Logger.ErrorStackDisabled = true
	t.Cleanup(func() { config.App.Logger.ErrorStackDisabled = old })

	lg, logs := newObservedLogger()

	lg.Error(newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "stack traced failure")

	for _, field := range entries[0].Context {
		require.NotEqual(t, errorStackKey, field.Key)
	}
}

func TestErrorSkipsErrorStackFieldWhenNoErrorArgs(t *testing.T) {
	lg, logs := newObservedLogger()

	lg.Error("plain message")

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Empty(t, entries[0].Context)
}

// TestErrorFieldsAreWrittenAsTextWithTheErrorStack pins what an error field
// becomes on the wire through the loggers the constructors build: the error's
// text under the field's key, the error_stack errorstack derives for it, and
// no Verbose field — zapcore's rendering of the error's %+v, the stack of
// every wrap of the chain — whichever way the error reaches the core: the
// typed field of Errorz or of a zap logger, or the loose key-value of Errorw,
// typed or sugared.
func TestErrorFieldsAreWrittenAsTextWithTheErrorStack(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "")
	err := errors.Wrap(newStackTracedError(), "load account")

	New("errorz.log").Errorz("operation failed", zap.Error(err))
	New("errorw.log").Errorw("operation failed", "error", err)
	NewZap("zap.log").Error("operation failed", zap.Error(err))
	NewSugared("sugar.log").Errorw("operation failed", "error", err)
	Clean()

	for _, file := range []string{"errorz.log", "errorw.log", "zap.log", "sugar.log"} {
		line, entry := readLogEntry(t, filepath.Join(dir, file))
		require.Equal(t, "load account: stack traced failure", entry["error"], file)
		require.Equal(t, errorstack.Origin(err), entry[errorStackKey], file)
		requireNoVerboseField(t, entry, file)
		require.NotContains(t, line, "doInit", file)
	}
}

// TestASentinelErrorFieldCarriesNoStack pins the rule of errorstack at the
// core: the stack a package-level error captured while the package
// initialized is no origin, so the entry has no error_stack, and no Verbose
// field bringing that init stack back.
func TestASentinelErrorFieldCarriesNoStack(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "")

	New("sentinel.log").Errorz("operation failed", zap.Error(errSentinel))
	Clean()

	line, entry := readLogEntry(t, filepath.Join(dir, "sentinel.log"))
	require.Equal(t, "sentinel failure", entry["error"])
	require.NotContains(t, entry, errorStackKey)
	requireNoVerboseField(t, entry, "sentinel.log")
	require.NotContains(t, line, "doInit")
}

// TestTheErrorStackSwitchLeavesAnErrorFieldEntryWithoutAStack pins
// logger.error_stack_disabled at the core: an entry with an error field
// carries no error_stack and no Verbose field, the error's text alone.
func TestTheErrorStackSwitchLeavesAnErrorFieldEntryWithoutAStack(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "")
	config.App.Logger.ErrorStackDisabled = true

	New("switch.log").Errorz("operation failed", zap.Error(newStackTracedError()))
	Clean()

	_, entry := readLogEntry(t, filepath.Join(dir, "switch.log"))
	require.Equal(t, "stack traced failure", entry["error"])
	require.NotContains(t, entry, errorStackKey)
	requireNoVerboseField(t, entry, "switch.log")
}

// TestAnErrorGroupFieldKeepsItsCausesAsText pins the causes zapcore lists for
// an error group, the errors go.uber.org/multierr combines: each is written as
// its text alone, no Verbose field inside any of them.
func TestAnErrorGroupFieldKeepsItsCausesAsText(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "")
	err := multierr.Combine(errors.New("first failure"), errors.New("second failure"))

	New("group.log").Errorz("operation failed", zap.Error(err))
	Clean()

	_, entry := readLogEntry(t, filepath.Join(dir, "group.log"))
	require.Equal(t, "first failure; second failure", entry["error"])
	causes, ok := entry["errorCauses"].([]any)
	require.True(t, ok, "errorCauses = %v", entry["errorCauses"])
	require.Len(t, causes, 2)
	for _, cause := range causes {
		fields, ok := cause.(map[string]any)
		require.True(t, ok)
		require.Equal(t, []string{"error"}, mapKeys(fields))
	}
	requireNoVerboseField(t, entry, "group.log")
}

// TestAnEntryCarriesOneErrorStack pins error_stack written once per entry:
// the first error carrying a stack sets it, a context field counted, and the
// entry's own error field adds no second one.
func TestAnEntryCarriesOneErrorStack(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Clean)
	withLoggerInitConfig(t, dir, "")
	first := newStackTracedError()

	NewZap("once.log").With(zap.Error(first)).Error("operation failed", zap.NamedError("cause", errors.New("second failure")))
	Clean()

	line, entry := readLogEntry(t, filepath.Join(dir, "once.log"))
	require.Equal(t, 1, strings.Count(line, `"`+errorStackKey+`":`), line)
	require.Equal(t, errorstack.Origin(first), entry[errorStackKey])
	require.Equal(t, "second failure", entry["cause"])
	requireNoVerboseField(t, entry, "once.log")
}

// plainError is an error without any embedded stack trace.
type plainError struct{}

func (plainError) Error() string { return "plain failure" }

// newStackTracedError creates an error whose stack trace points at this
// helper, so tests can assert the error origin is recorded.
func newStackTracedError() error {
	return errors.New("stack traced failure")
}

func newObservedLogger() (*Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return &Logger{zlog: zap.New(&errorFieldCore{Core: core})}, logs
}

func BenchmarkErrorWithStackTracedError(b *testing.B) {
	lg := newDiscardLogger()
	err := errors.Wrap(newStackTracedError(), "wrapped")

	b.ReportAllocs()
	for b.Loop() {
		lg.Error(err)
	}
}

func BenchmarkErrorWithPlainError(b *testing.B) {
	lg := newDiscardLogger()

	b.ReportAllocs()
	for b.Loop() {
		lg.Error(plainError{})
	}
}

func errorStackField(t *testing.T, entry observer.LoggedEntry) string {
	t.Helper()

	for _, field := range entry.Context {
		if field.Key == errorStackKey {
			return field.String
		}
	}
	require.Failf(t, "missing field", "log entry has no %s field", errorStackKey)
	return ""
}

// readLogEntry reads the one entry file holds, as the line written and as the
// object it decodes to.
func readLogEntry(t *testing.T, path string) (string, map[string]any) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(data, &entry), "%q", data)
	return string(data), entry
}

// requireNoVerboseField fails on a key ending in Verbose, the field zapcore
// adds beside an error field whose error formats itself.
func requireNoVerboseField(t *testing.T, entry map[string]any, file string) {
	t.Helper()

	for key := range entry {
		require.False(t, strings.HasSuffix(key, "Verbose"), "%s: field %s: %v", file, key, entry[key])
	}
}

// mapKeys returns the keys of fields, sorted.
func mapKeys(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
