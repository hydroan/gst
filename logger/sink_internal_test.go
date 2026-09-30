package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/instance"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewLogWriterBuffersFileSink(t *testing.T) {
	withLogWriterConfig(t, t.TempDir(), "buffered.log")

	writer := newLogWriter(readConf(""))
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

			writer := newLogWriter(readConf(""))
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
		writer = newLogWriter(readConf(""), Option{Console: true})
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
		writer = newLogWriter(readConf(""))
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

			writer := newLogWriter(readConf(""), Option{Console: true})
			if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
				t.Cleanup(func() { _ = buffered.Stop() })
				t.Fatalf("expected %q sink to stay unbuffered", tt.file)
			}
		})
	}
}

func TestNewLogWriterPrecreatesEmptyLogFile(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "precreated.log")

	writer := newLogWriter(readConf(""))
	t.Cleanup(func() {
		if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
			_ = buffered.Stop()
		}
	})

	// The file must exist before the first entry is written: log collectors
	// discover files by path, and a config error (bad dir, bad permissions)
	// must surface at startup, not at the first log write.
	info, err := os.Stat(filepath.Join(dir, "precreated.log"))
	require.NoError(t, err, "constructing a file sink must create the log file")
	require.Zero(t, info.Size())
}

func TestNewLogWriterPrecreationKeepsExistingContent(t *testing.T) {
	dir := t.TempDir()
	withLogWriterConfig(t, dir, "existing.log")
	path := filepath.Join(dir, "existing.log")
	require.NoError(t, os.WriteFile(path, []byte("entry before restart\n"), 0o600))

	writer := newLogWriter(readConf(""))
	t.Cleanup(func() {
		if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
			_ = buffered.Stop()
		}
	})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "entry before restart\n", string(data),
		"precreation must not truncate a log file that survived a restart")
}

func TestNewLogWriterPrecreationFailureWarnsAndKeepsSink(t *testing.T) {
	// A plain file where the log directory should be makes both MkdirAll and
	// the file open fail deterministically.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	withLogWriterConfig(t, filepath.Join(blocker, "sub"), "warn.log")

	var writer zapcore.WriteSyncer
	output := captureStderr(t, func() {
		writer = newLogWriter(readConf(""))
	})
	t.Cleanup(func() {
		if buffered, ok := writer.(*zapcore.BufferedWriteSyncer); ok {
			_ = buffered.Stop()
		}
	})

	// Logging is observability, not the business itself: a sink that cannot
	// be precreated still returns a writer and only warns, so the process
	// starts and every other sink keeps working.
	require.NotNil(t, writer)
	require.Contains(t, output, "precreate log file")
	require.Contains(t, output, "warn.log")
}

// TestStdoutOutputWritesEveryStreamToStdoutUnderItsName proves stdout mode
// sends every stream, whichever constructor built its logger, to stdout with
// the process identity and a logger field naming the stream — a fallback
// component logger writing into the global stream — and creates no file.
func TestStdoutOutputWritesEveryStreamToStdoutUnderItsName(t *testing.T) {
	dir := t.TempDir()
	withLoggerInitConfig(t, dir, "sample.log")
	config.App.Logger.Output = config.LoggerOutputStdout
	restoreGlobalLoggers(t)

	output := captureStdout(t, func() {
		require.NoError(t, Init())
		zap.S().Info("global")
		App.Infoz("app")
		Gorm.Info(context.Background(), "gorm")
		New("typed.log").Infoz("typed")
		NewGin("access.log").Info("access")
		NewZap("plain.log").Info("plain")
		NewSugared("sugared.log").Info("sugared")
		Fallback("sample_component").Infoz("fallback")
		Clean()
	})

	var names []string
	for line := range strings.Lines(output) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
		require.Equal(t, instance.ID(), entry["instance"], line)
		name, ok := entry["logger"].(string)
		require.True(t, ok, "every stdout entry names its stream: %s", line)
		names = append(names, name)
	}
	require.ElementsMatch(t, []string{"sample", "app", "gorm", "typed", "access", "plain", "sugared", "sample"}, names)

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files, "stdout mode creates no log file")
}

// TestStdoutOutputKeepsConcurrentEntriesWhole proves streams logging at the
// same time through the shared stdout sink never interleave: every stream
// writes through the one sink, and every line stdout receives is one whole
// entry, entries far over a pipe's atomic write size and over the sink's
// buffer included.
func TestStdoutOutputKeepsConcurrentEntriesWhole(t *testing.T) {
	withLoggerInitConfig(t, t.TempDir(), "")
	config.App.Logger.Output = config.LoggerOutputStdout
	payload := strings.Repeat("x", 16*1024)
	oversized := strings.Repeat("y", defaultLogBufferSize+1024)
	const streams, entries, oversizedEntries = 8, 50, 4

	output := captureStdout(t, func() {
		first, second := stdoutLogWriter(), stdoutLogWriter()
		require.Same(t, first, second, "every stream writes through one sink")
		var wg sync.WaitGroup
		for i := range streams {
			log := New(fmt.Sprintf("stream%d.log", i))
			wg.Go(func() {
				for j := range entries {
					log.Infoz("entry", zap.String("payload", payload))
					if i < 2 && j < oversizedEntries {
						log.Infoz("entry", zap.String("payload", oversized))
					}
				}
			})
		}
		wg.Wait()
		Clean()
	})

	lines := 0
	for line := range strings.Lines(output) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "a line is not one whole entry")
		require.Contains(t, []any{payload, oversized}, entry["payload"])
		lines++
	}
	require.Equal(t, streams*entries+2*oversizedEntries, lines)
}

// captureStdout runs fn with os.Stdout redirected into a pipe and returns what
// was written. The pipe is drained while fn runs, so fn may write more than the
// pipe holds.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writePipe
	type drained struct {
		output []byte
		err    error
	}
	done := make(chan drained, 1)
	go func() {
		output, err := io.ReadAll(readPipe)
		done <- drained{output, err}
	}()

	fn()

	require.NoError(t, writePipe.Close())
	os.Stdout = oldStdout

	result := <-done
	require.NoError(t, result.err)
	require.NoError(t, readPipe.Close())
	return string(result.output)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writePipe

	fn()

	require.NoError(t, writePipe.Close())
	os.Stderr = oldStderr

	output, err := io.ReadAll(readPipe)
	require.NoError(t, err)
	require.NoError(t, readPipe.Close())
	return string(output)
}

func withLogWriterConfig(t *testing.T, dir, file string) {
	t.Helper()

	oldDir := config.App.Dir
	oldLogger := config.App.Logger

	config.App.Dir = dir
	config.App.Logger.Dir = dir
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.File = file
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"
	config.App.Logger.MaxAge = 30
	config.App.Logger.MaxSize = 100
	config.App.Logger.MaxBackups = 1

	t.Cleanup(func() {
		config.App.Dir = oldDir
		config.App.Logger = oldLogger
	})
}
