package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hydroan/gst/config"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// defaultLogBufferSize and defaultLogFlushInterval bound the buffering of
	// the file sinks and the shared stdout sink, which write their entries out
	// when the buffer fills, the flush interval elapses, or Clean (or a Sync
	// on the logger) runs. Until then the output stays behind what was
	// logged, which is all a reader inspecting it right after the fact gets
	// to see.
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
// file mode stdout/stderr or a rolling file, one sink per path shared by
// every stream writing it (see fileLogWriter).
// opts: opts[0].Console additionally mirrors a file sink to os.Stdout.
func newLogWriter(cfg logConfig, opts ...Option) zapcore.WriteSyncer {
	if cfg.output == config.LoggerOutputStdout {
		return stdoutLogWriter()
	}
	switch strings.TrimSpace(cfg.file) {
	case "/dev/stdout":
		return zapcore.AddSync(os.Stdout)
	case "/dev/stderr":
		return zapcore.AddSync(os.Stderr)
	case "":
		return zapcore.AddSync(os.Stdout)
	default:
		writer := fileLogWriter(filepath.Join(config.App.Dir, cfg.file), cfg)
		if len(opts) > 0 && opts[0].Console {
			return zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), writer)
		}
		return writer
	}
}

// fileLogWriters are the file sinks by the path they write, so that every
// stream of one path writes through one sink: the global stream and a stream
// named after the same file, or a client's logger built on the global file,
// share one lumberjack. Lumberjack assumes it is the file's only writer; a
// second instance on the path would rotate the file under the first and
// carry its own entries off into backups of its own, where the backup
// limit then deletes them. The first constructor's rotation settings serve
// the path, which every stream reads from the one configuration. Clean
// stops the sinks with the stdout one, and the next file-mode logger starts
// afresh.
var (
	fileLogWritersMu sync.Mutex
	fileLogWriters   = map[string]*zapcore.BufferedWriteSyncer{}
)

// fileLogWriter returns the sink of the file at path, starting it on first
// use.
func fileLogWriter(path string, cfg logConfig) *zapcore.BufferedWriteSyncer {
	path = filepath.Clean(path)
	fileLogWritersMu.Lock()
	defer fileLogWritersMu.Unlock()

	if writer, ok := fileLogWriters[path]; ok {
		return writer
	}
	precreateLogFile(path)
	writer := &zapcore.BufferedWriteSyncer{
		WS: zapcore.AddSync(&lumberjack.Logger{
			Filename:   path,
			MaxAge:     cfg.maxAge,
			MaxSize:    cfg.maxSize,
			MaxBackups: cfg.maxBackups,
			LocalTime:  true,
			Compress:   false, // openwrt may not support compress.
		}),
		Size:          defaultLogBufferSize,
		FlushInterval: defaultLogFlushInterval,
	}
	fileLogWriters[path] = writer
	registerBufferedLogWriter(writer)
	return writer
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

	fileLogWritersMu.Lock()
	fileLogWriters = map[string]*zapcore.BufferedWriteSyncer{}
	fileLogWritersMu.Unlock()

	bufferedLogWritersMu.Lock()
	writers := bufferedLogWriters
	bufferedLogWriters = nil
	bufferedLogWritersMu.Unlock()

	for _, writer := range writers {
		_ = writer.Stop()
	}
}

// syncBufferedLogWriters flushes every buffered writer the constructors
// registered, the file writers and the shared stdout sink, and leaves them
// running: what the global logger's Sync does for the streams (see
// flushingCore).
func syncBufferedLogWriters() {
	bufferedLogWritersMu.Lock()
	writers := slices.Clone(bufferedLogWriters)
	bufferedLogWritersMu.Unlock()

	for _, writer := range writers {
		_ = writer.Sync()
	}
}

// flushingCore is the core of the global logger: its Sync flushes every
// buffered writer the constructors registered before syncing its own, so
// syncing the global logger alone writes out what every stream buffered. A
// process exiting on a failure syncs exactly that one logger (see
// util.RunOrDie), the only one it reaches without importing this package;
// zap's Logger.Sync reaches its own core alone, and a BufferedWriteSyncer
// flushes on its own interval or size otherwise, so the entries a component
// logged moments before the exit would stay in its buffer.
type flushingCore struct {
	zapcore.Core
}

// With keeps the fields' core flushing, as the loggers named and derived
// from the global one sync through it.
func (c flushingCore) With(fields []zapcore.Field) zapcore.Core {
	return flushingCore{Core: c.Core.With(fields)}
}

// Sync flushes every registered writer, then syncs the core's own, whose
// failure — syncing a terminal — must not stop the flush ahead of it.
func (c flushingCore) Sync() error {
	syncBufferedLogWriters()
	return c.Core.Sync()
}
