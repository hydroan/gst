package zap

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/instance"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestContextFieldsFitTheCapacityInTheWorstCase pins contextFieldCap to the
// fields WithContext binds when every optional one is present — the request
// metadata in full and a cron round's identity — so a field added to
// withContextFields without bumping the capacity fails here instead of
// regrowing the slice on every context-scoped logger.
func TestContextFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	l := &Logger{zlog: zap.New(core)}

	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{
		Route:    "/api/samples/:id",
		Path:     "/api/samples/1",
		Method:   "GET",
		Username: "sample",
		UserID:   "1",
		Params:   map[string]string{"id": "1"},
		RawQuery: "page=1",
	}))
	ctx = execctx.WithCronjob(ctx, "sample_job", "trace-worst")
	l.WithContext(ctx, consts.PHASE_LIST).Infoz("sample entry")

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Context, contextFieldCap,
		"the worst case must fill the capacity exactly: a new field bumps contextFieldCap, a dropped one lowers it")
	require.Contains(t, entries[0].ContextMap(), consts.CRONJOB, "the worst case must carry the optional identity field")
}

func BenchmarkLogger_Discard10(b *testing.B) {
	l := newDiscardLogger()
	msg := strings.Repeat("0", 10)

	for b.Loop() {
		l.Infoz(msg)
	}
}

func BenchmarkLogger_Discard100(b *testing.B) {
	l := newDiscardLogger()
	msg := strings.Repeat("0", 100)

	for b.Loop() {
		l.Infoz(msg)
	}
}

func BenchmarkLogger_Discard1000(b *testing.B) {
	l := newDiscardLogger()
	msg := strings.Repeat("0", 1000)

	for b.Loop() {
		l.Infoz(msg)
	}
}

func BenchmarkLogger_Discard10000(b *testing.B) {
	l := newDiscardLogger()
	msg := strings.Repeat("0", 10000)

	for b.Loop() {
		l.Infoz(msg)
	}
}

// newDiscardLogger builds a logger the way New does — the same encoder, level,
// process identity and options — over a sink that drops every entry, so a
// benchmark logging through it measures what a call costs the logger itself,
// with no write adding to the figure. The File benchmarks in logger_test.go
// measure the same calls through the buffered file sink.
func newDiscardLogger() *Logger {
	core := zapcore.NewCore(newLogEncoder(), zapcore.AddSync(io.Discard), newLogLevel())
	return &Logger{zlog: zap.New(
		core.With([]zapcore.Field{zap.String(consts.INSTANCE, instance.ID())}),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.FatalLevel),
	)}
}
