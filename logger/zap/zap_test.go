package zap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"go.uber.org/zap/zapcore"
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
