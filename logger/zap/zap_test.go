package zap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogWriterBuffersFileSink(t *testing.T) {
	withLogWriterConfig(t, t.TempDir(), "buffered.log")

	writer := newLogWriter()
	buffered, ok := writer.(*zapcore.BufferedWriteSyncer)
	if !ok {
		t.Fatalf("expected file sink to use *zapcore.BufferedWriteSyncer, got %T", writer)
	}
	t.Cleanup(func() { _ = buffered.Stop() })

	if buffered.Size != 256*1024 {
		t.Fatalf("expected buffer size 262144, got %d", buffered.Size)
	}
	if buffered.FlushInterval != time.Second {
		t.Fatalf("expected flush interval 1s, got %s", buffered.FlushInterval)
	}
}

func TestNewLogWriterLeavesStdStreamsUnbuffered(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "stdout", file: "/dev/stdout"},
		{name: "stderr", file: "/dev/stderr"},
		{name: "empty", file: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLogWriterConfig(t, t.TempDir(), tt.file)

			writer := newLogWriter()
			if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
				t.Cleanup(func() { _ = buffered.Stop() })
				t.Fatalf("expected %q sink to stay unbuffered", tt.file)
			}
		})
	}
}

func TestCleanFlushesBufferedFileSink(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "clean.log")

	log := New("clean.log")
	log.Infoz("flush through clean")
	Clean()

	data, err := os.ReadFile(filepath.Join(dir, "clean.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "flush through clean") {
		t.Fatalf("expected flushed log file to contain message, got %q", string(data))
	}
}

func TestWithRequestMetadataAddsControllerLogFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := types.NewRequestMetadataFromValues(types.RequestMetadataValues{
		Route:    "/api/users/:id",
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
		Params: map[string]string{
			"id": "42",
		},
		Query: map[string][]string{
			"tag": {"blue", "green"},
		},
	})

	log.WithRequestMetadata(meta, consts.PHASE_GET).Infoz("controller request")

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.Equal(t, string(consts.PHASE_GET), fields[consts.PHASE])
	require.Equal(t, "/api/users/:id", fields[consts.CTX_ROUTE])
	require.Equal(t, "admin", fields[consts.CTX_USERNAME])
	require.Equal(t, "user-1", fields[consts.CTX_USER_ID])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
	require.Equal(t, map[string]any{"id": "42"}, fields[consts.PARAMS])
	require.Equal(t, map[string]any{"tag": "blue,green"}, fields[consts.QUERY])
}

func TestWithContextAddsRequestMetadataFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := types.NewRequestMetadataFromValues(types.RequestMetadataValues{
		Route:    "/api/users/:id",
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
		Params: map[string]string{
			"id": "42",
		},
		Query: map[string][]string{
			"tag": {"blue", "green"},
		},
	})
	ctx := types.ContextWithRequestMetadata(context.Background(), meta)

	log.WithContext(ctx, consts.PHASE_LIST).Infoz("database request")

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.Equal(t, string(consts.PHASE_LIST), fields[consts.PHASE])
	require.Equal(t, "/api/users/:id", fields[consts.CTX_ROUTE])
	require.Equal(t, "admin", fields[consts.CTX_USERNAME])
	require.Equal(t, "user-1", fields[consts.CTX_USER_ID])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
	require.Equal(t, map[string]any{"id": "42"}, fields[consts.PARAMS])
	require.Equal(t, map[string]any{"tag": "blue,green"}, fields[consts.QUERY])
}

func TestGormTraceUsesRequestMetadataFromContext(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := types.NewRequestMetadataFromValues(types.RequestMetadataValues{
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
	})
	ctx := types.ContextWithRequestMetadata(context.Background(), meta)

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
	require.Equal(t, "admin", fields[consts.CTX_USERNAME])
	require.Equal(t, "user-1", fields[consts.CTX_USER_ID])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
	require.Equal(t, "select 1", fields["sql"])
	require.Equal(t, int64(1), fields["rows"])
}

func withLogWriterConfig(t *testing.T, dir, file string) {
	t.Helper()

	oldDir := config.App.Dir
	oldLogFile := logFile
	oldLogLevel := logLevel
	oldLogFormat := logFormat
	oldLogMaxAge := logMaxAge
	oldLogMaxSize := logMaxSize
	oldLogMaxBackups := logMaxBackups

	config.App.Dir = dir
	config.App.Logger.Dir = dir
	logFile = file
	logLevel = "info"
	logFormat = "json"
	logMaxAge = 30
	logMaxSize = 100
	logMaxBackups = 1

	t.Cleanup(func() {
		config.App.Dir = oldDir
		config.App.Logger.Dir = oldDir
		logFile = oldLogFile
		logLevel = oldLogLevel
		logFormat = oldLogFormat
		logMaxAge = oldLogMaxAge
		logMaxSize = oldLogMaxSize
		logMaxBackups = oldLogMaxBackups
	})
}
