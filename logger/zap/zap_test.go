package zap

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
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

func TestNewLogWriterConsoleOptionTeesFileSinkToStdout(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "console.log")

	var writer zapcore.WriteSyncer
	output := captureStdout(t, func() {
		writer = newLogWriter(Option{Console: true})
		_, err := writer.Write([]byte("teed to stdout"))
		require.NoError(t, err)
		// Syncing a pipe (stdout stand-in here) fails on some platforms since
		// pipes don't support fsync; production code discards this error too
		// (see Clean()), so the file-sink assertion below is what matters.
		_ = writer.Sync()
	})
	t.Cleanup(func() {
		if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
			_ = buffered.Stop()
		}
	})
	require.Contains(t, output, "teed to stdout")

	data, err := os.ReadFile(filepath.Join(dir, "console.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "teed to stdout")
}

func TestNewLogWriterWithoutConsoleOptionStaysFileOnly(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "file_only.log")

	var writer zapcore.WriteSyncer
	output := captureStdout(t, func() {
		writer = newLogWriter()
		_, err := writer.Write([]byte("file only"))
		require.NoError(t, err)
		require.NoError(t, writer.Sync())
	})
	t.Cleanup(func() {
		if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
			_ = buffered.Stop()
		}
	})
	require.Empty(t, output)

	data, err := os.ReadFile(filepath.Join(dir, "file_only.log"))
	require.NoError(t, err)
	require.Contains(t, string(data), "file only")
}

func TestNewLogWriterConsoleOptionIgnoredForStdStreams(t *testing.T) {
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

			writer := newLogWriter(Option{Console: true})
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

func TestNewLogEncoderTimestampIsUTCAndOrdersWithinASecond(t *testing.T) {
	encoder := newLogEncoder()
	at := time.Date(2026, 7, 29, 14, 3, 8, 243834831, time.FixedZone("", 8*60*60))

	encode := func(at time.Time) string {
		buf, err := encoder.EncodeEntry(zapcore.Entry{Time: at}, nil)
		require.NoError(t, err)
		var entry struct {
			TS string `json:"ts"`
		}
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		return entry.TS
	}

	first := encode(at)
	require.Equal(t, "2026-07-29T06:03:08.243834831Z", first,
		"the host zone must not leak into the timestamp")

	// The entries of one request land in the same second, so a whole-second
	// timestamp would collapse them into a single value and lose their order.
	next := encode(at.Add(time.Microsecond))
	require.Less(t, first, next)

	// The timestamp is rendered in UTC no matter what zone the host runs in,
	// so an entry reads on the same clock as the stored rows it describes, and
	// entries from any two hosts order lexicographically. The layout is one a
	// log store reads as a date unassisted.
	parsed, err := time.Parse(time.RFC3339Nano, first)
	require.NoError(t, err)
	require.True(t, parsed.Equal(at))
	_, offset := parsed.Zone()
	require.Equal(t, 0, offset)
}

func TestNewLogEncoderReflectedValuesCollapseToOneStringField(t *testing.T) {
	encoder := newLogEncoder()

	encode := func(t *testing.T, fields ...zapcore.Field) map[string]any {
		t.Helper()
		buf, err := encoder.EncodeEntry(zapcore.Entry{Time: time.Now()}, fields)
		require.NoError(t, err)
		var entry map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry),
			"every entry must stay one valid JSON document")
		return entry
	}

	type sampleRecord struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	// The struct, map, and slice shapes below all reach the encoder through
	// zap.Any's reflection fallback. Each must land as a single string field
	// so the log store's field mapping stays bounded; the string still parses
	// as the value's JSON.
	t.Run("struct", func(t *testing.T) {
		entry := encode(t, zap.Any("record", &sampleRecord{Name: "sample", Count: 3}))
		collapsed, ok := entry["record"].(string)
		require.True(t, ok, "reflected struct must encode as one string field, got %T", entry["record"])
		var record sampleRecord
		require.NoError(t, json.Unmarshal([]byte(collapsed), &record))
		require.Equal(t, sampleRecord{Name: "sample", Count: 3}, record)
	})

	t.Run("map", func(t *testing.T) {
		entry := encode(t, zap.Any("attributes", map[string]int{"total": 7}))
		collapsed, ok := entry["attributes"].(string)
		require.True(t, ok, "reflected map must encode as one string field, got %T", entry["attributes"])
		require.JSONEq(t, `{"total":7}`, collapsed)
	})

	t.Run("slice of structs", func(t *testing.T) {
		entry := encode(t, zap.Any("records", []sampleRecord{{Name: "first", Count: 1}}))
		collapsed, ok := entry["records"].(string)
		require.True(t, ok, "reflected slice must encode as one string field, got %T", entry["records"])
		require.JSONEq(t, `[{"name":"first","count":1}]`, collapsed)
	})

	t.Run("unmarshalable value falls back to Go syntax", func(t *testing.T) {
		entry := encode(t, zap.Any("stream", map[string]chan int{"items": make(chan int)}))
		collapsed, ok := entry["stream"].(string)
		require.True(t, ok, "fallback must still encode as one string field, got %T", entry["stream"])
		require.Contains(t, collapsed, "chan int")
	})

	// Typed fields never touch the reflected encoder: scalar key-value pairs
	// keep their native JSON types and stay individually indexable.
	t.Run("typed fields keep their native JSON types", func(t *testing.T) {
		entry := encode(t, zap.String("kind", "sample"), zap.Int("count", 3))
		require.Equal(t, "sample", entry["kind"])
		require.IsType(t, float64(0), entry["count"], "count must stay a native JSON number")
		require.InDelta(t, 3, entry["count"], 0)
	})
}

func TestWithContextAddsMetadataFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := requestctx.New(requestctx.Fields{
		Route:    "/api/users/:id",
		Path:     "/api/users/42",
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
	ctx := requestctx.WithMetadata(context.Background(), meta)

	log.WithContext(ctx, consts.PHASE_LIST).Infoz("database request")

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.Equal(t, string(consts.PHASE_LIST), fields[consts.PHASE])
	require.Equal(t, "/api/users/:id", fields[consts.CTX_ROUTE])
	require.Equal(t, "/api/users/42", fields[consts.CTX_PATH])
	require.Equal(t, "admin", fields[consts.CTX_USERNAME])
	require.Equal(t, "user-1", fields[consts.CTX_USER_ID])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
	require.Equal(t, map[string]any{"id": "42"}, fields[consts.PARAMS])
	// Fields carries no RawQuery, so the logged query is re-encoded from Query.
	require.Equal(t, "tag=blue&tag=green", fields[consts.QUERY])
}

func TestGormTraceUsesMetadata(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log := &Logger{zlog: zap.New(core)}
	meta := requestctx.New(requestctx.Fields{
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
	})
	ctx := requestctx.WithMetadata(context.Background(), meta)

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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writePipe

	fn()

	require.NoError(t, writePipe.Close())
	os.Stdout = oldStdout

	output, err := io.ReadAll(readPipe)
	require.NoError(t, err)
	require.NoError(t, readPipe.Close())
	return string(output)
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
