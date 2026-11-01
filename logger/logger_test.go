package logger_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

var (
	msg10    = "0000000000"
	msg100   = strings.Repeat(msg10, 10)
	msg1000  = strings.Repeat(msg10, 100)
	msg10000 = strings.Repeat(msg10, 1000)

	keyValues10  = []string{}
	keyValues100 = []string{}
)

func init() {
	// init keyValues10
	for i := range 10 {
		keyValues10 = append(keyValues10, "key"+strconv.Itoa(i), "value"+strconv.Itoa(i))
	}
	// init keyValues100
	for i := range 100 {
		keyValues100 = append(keyValues100, "key"+strconv.Itoa(i), "value"+strconv.Itoa(i))
	}
}

func createLogger(b *testing.B, filename string) types.Logger {
	b.Helper()
	b.Setenv(config.LOGGER_OUTPUT, string(config.LoggerOutputFile))
	b.Setenv(config.LOGGER_FILE, filename)
	b.Setenv(config.LOGGER_DIR, b.TempDir())
	if err := config.Init(); err != nil {
		b.Fatal(err)
	}
	l := logger.New("")
	return l
}

func BenchmarkLoggerFile10(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10)
	}
}

func BenchmarkLoggerFile100(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg100)
	}
}

func BenchmarkLoggerFile1000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg1000)
	}
}

func BenchmarkLoggerFile10000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10000)
	}
}

func BenchmarkLoggerWith10(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues10...).Info(msg10)
	}
}

func BenchmarkLoggerWith100(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues100...).Info(msg10)
	}
}

// benchmarkRequestContext is a context carrying the metadata and the identity
// of one request, what WithContext binds to a derived logger.
func benchmarkRequestContext() context.Context {
	ctx := execctx.WithTraceID(context.Background(), "trace-sample")
	return requestctx.WithMetadata(ctx, requestctx.New(requestctx.Fields{
		Route:    "/api/samples/:id",
		Path:     "/api/samples/1",
		Method:   http.MethodGet,
		Username: "alice",
		UserID:   "u1",
		Params:   map[string]string{"id": "1"},
		Query:    url.Values{"name": {"sample"}},
		RawQuery: "name=sample",
	}))
}

// BenchmarkWithContextDiscardedDebug measures deriving a request logger and
// writing a debug entry on it at the default info level: the shape of every
// database operation's outcome entry, which the level discards.
func BenchmarkWithContextDiscardedDebug(b *testing.B) {
	l := createLogger(b, "test.log")
	ctx := benchmarkRequestContext()

	b.ReportAllocs()
	for b.Loop() {
		l.WithContext(ctx, apiaction.List).Debugz("database operation completed", zap.String("model", "Sample"))
	}
}

// BenchmarkWithContextInfoz measures deriving a request logger and writing an
// info entry on it: the shape of a service's own log line.
func BenchmarkWithContextInfoz(b *testing.B) {
	l := createLogger(b, "test.log")
	ctx := benchmarkRequestContext()

	b.ReportAllocs()
	for b.Loop() {
		l.WithContext(ctx, apiaction.List).Infoz("sample listed", zap.Int("count", 3))
	}
}
