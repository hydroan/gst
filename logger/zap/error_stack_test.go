package zap

import (
	"io"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	logger, logs := newObservedLogger()

	logger.Error(newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "stack traced failure")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
	require.Contains(t, stackTrace, "error_stack_test.go")
}

func TestErrorfAttachesErrorStackFieldFromErrorOrigin(t *testing.T) {
	logger, logs := newObservedLogger()

	logger.Errorf("operation failed: %v", newStackTracedError())

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "operation failed: stack traced failure")

	stackTrace := errorStackField(t, entries[0])
	require.Contains(t, stackTrace, "newStackTracedError")
}

func TestErrorSkipsErrorStackFieldWhenErrorHasNoStackTrace(t *testing.T) {
	logger, logs := newObservedLogger()

	logger.Error(plainError{})

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "plain failure")

	for _, field := range entries[0].Context {
		require.NotEqual(t, errorStackKey, field.Key)
	}
}

func TestErrorSkipsErrorStackFieldWhenNoErrorArgs(t *testing.T) {
	logger, logs := newObservedLogger()

	logger.Error("plain message")

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Empty(t, entries[0].Context)
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
	return &Logger{zlog: zap.New(core)}, logs
}

func BenchmarkErrorWithStackTracedError(b *testing.B) {
	logger := newDiscardLogger()
	err := errors.Wrap(newStackTracedError(), "wrapped")

	b.ReportAllocs()
	for b.Loop() {
		logger.Error(err)
	}
}

func BenchmarkErrorWithPlainError(b *testing.B) {
	logger := newDiscardLogger()

	b.ReportAllocs()
	for b.Loop() {
		logger.Error(plainError{})
	}
}

func newDiscardLogger() *Logger {
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard),
		zapcore.ErrorLevel,
	)
	return &Logger{zlog: zap.New(core)}
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
