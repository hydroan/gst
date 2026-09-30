package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hydroan/gst/config"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// The file sinks and the shared stdout sink buffer their entries and
	// write them out when the buffer fills, the flush interval elapses, or
	// Clean (or a Sync on the logger) runs. Until then the output stays
	// behind what was logged, which is all a reader inspecting it right after
	// the fact gets to see.
	defaultLogBufferSize    = 256 * 1024
	defaultLogFlushInterval = time.Second
)

// bufferedLogWriters are the buffered sinks the constructors built, for
// Clean to stop (see stopBufferedLogWriters).
var (
	bufferedLogWritersMu sync.Mutex
	bufferedLogWriters   []*zapcore.BufferedWriteSyncer
)

// newLogWriter selects log sink: the shared stdout sink in stdout mode, and in
// file mode stdout/stderr or a rolling file.
// opts: opts[0].Console additionally mirrors a file sink to os.Stdout.
func newLogWriter(opts ...Option) zapcore.WriteSyncer {
	if logOutput == config.LoggerOutputStdout {
		return stdoutLogWriter()
	}
	switch strings.TrimSpace(logFile) {
	case "/dev/stdout":
		return zapcore.AddSync(os.Stdout)
	case "/dev/stderr":
		return zapcore.AddSync(os.Stderr)
	case "":
		return zapcore.AddSync(os.Stdout)
	default:
		precreateLogFile(filepath.Join(config.App.Dir, logFile))
		writer := &zapcore.BufferedWriteSyncer{
			WS: zapcore.AddSync(&lumberjack.Logger{
				Filename:   filepath.Join(config.App.Dir, logFile),
				MaxAge:     logMaxAge,
				MaxSize:    logMaxSize,
				MaxBackups: logMaxBackups,
				LocalTime:  true,
				Compress:   false, // openwrt may not support compress.
			}),
			Size:          defaultLogBufferSize,
			FlushInterval: defaultLogFlushInterval,
		}
		registerBufferedLogWriter(writer)
		if len(opts) > 0 && opts[0].Console {
			return zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), writer)
		}
		return writer
	}
}

// precreateLogFile creates the log file and its directory at sink construction
// time instead of leaving both to lumberjack's lazy first-Write open. A sink
// that never logs would otherwise never touch disk, so log collectors find no
// file to tail after a quiet deploy, and a misconfigured directory or
// permission would stay silent until the first entry is dropped. Directory and
// file modes match what lumberjack itself uses, so which side creates them
// first makes no difference; an existing file is opened in append mode and
// kept as is.
//
// Failure only warns on stderr: logging is observability, not the business
// itself, and the constructors carry no error channel, so a sink that cannot
// be precreated must not stop the process or the other sinks.
func precreateLogFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "precreate log file %s: %v\n", path, err)
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- path is built from trusted logger config.
	if err != nil {
		fmt.Fprintf(os.Stderr, "precreate log file %s: %v\n", path, err)
		return
	}
	_ = file.Close()
}

// sharedStdoutWriter is the sink every stream writes through in stdout mode.
// One buffer behind one lock serves them all, so the entries of streams
// logging at the same time reach stdout whole, and a stream gets the same
// buffering a file sink does. Clean stops it with the file writers, and the
// next stdout-mode logger starts a new one.
var (
	sharedStdoutWriterMu sync.Mutex
	sharedStdoutWriter   *zapcore.BufferedWriteSyncer
)

// stdoutLogWriter returns the shared stdout sink, starting it on first use.
func stdoutLogWriter() zapcore.WriteSyncer {
	sharedStdoutWriterMu.Lock()
	defer sharedStdoutWriterMu.Unlock()

	if sharedStdoutWriter == nil {
		sharedStdoutWriter = &zapcore.BufferedWriteSyncer{
			WS:            zapcore.AddSync(os.Stdout),
			Size:          defaultLogBufferSize,
			FlushInterval: defaultLogFlushInterval,
		}
		registerBufferedLogWriter(sharedStdoutWriter)
	}
	return sharedStdoutWriter
}

func registerBufferedLogWriter(writer *zapcore.BufferedWriteSyncer) {
	if writer == nil {
		return
	}

	bufferedLogWritersMu.Lock()
	bufferedLogWriters = append(bufferedLogWriters, writer)
	bufferedLogWritersMu.Unlock()
}

func stopBufferedLogWriters() {
	sharedStdoutWriterMu.Lock()
	sharedStdoutWriter = nil
	sharedStdoutWriterMu.Unlock()

	bufferedLogWritersMu.Lock()
	writers := bufferedLogWriters
	bufferedLogWriters = nil
	bufferedLogWritersMu.Unlock()

	for _, writer := range writers {
		_ = writer.Stop()
	}
}
