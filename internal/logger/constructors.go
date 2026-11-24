package logger

import (
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/instance"
	"github.com/hydroan/gst/internal/logfield"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gormlogger "gorm.io/gorm/logger"
)

// logConfig is what a constructor builds one logger from: the [logger]
// section as it stands when the constructor runs, with the stream's own file
// in place of the section's. Each constructor reads its own, so constructors
// running at the same time never build from one another's file.
type logConfig struct {
	output     config.LoggerOutput
	file       string
	level      config.LogLevel
	format     config.LogFormat
	maxAge     int
	maxSize    int
	maxBackups int
}

// readConf reads the logger configuration as it stands, for the stream of
// filename; an empty filename keeps the section's own file.
func readConf(filename string) logConfig {
	cfg := logConfig{
		output:     config.App.Logger.Output,
		file:       config.App.Logger.File,
		level:      config.App.Logger.Level,
		format:     config.App.Logger.Format,
		maxAge:     config.App.Logger.MaxAge,
		maxSize:    config.App.Logger.MaxSize,
		maxBackups: config.App.Logger.MaxBackups,
	}
	if len(filename) > 0 {
		cfg.file = filename
	}
	return cfg
}

// Option configures encoder and writer behavior for constructors.
// DisableMsg/DisableLevel hide "msg" and "level" fields.
// Console additionally mirrors a file sink to os.Stdout; see newLogWriter.
//
// Timestamp layout is deliberately not an option: logfield.TimeLayout
// applies to every entry, so entries from different files stay orderable
// against one another and a log store types the field the same way everywhere.
type Option struct {
	DisableMsg    bool
	DisableLevel  bool
	DisableCaller bool
	Console       bool
}

// New builds the logger of a stream of its own, a gst.Logger backed by a
// *zap.Logger. filename is the stream's log file, whose name without ".log"
// names the stream in stdout mode, "/dev/stdout" writing to the console; opts
// adjust the encoder.
func New(filename string, opts ...Option) *Logger {
	cfg := readConf(filename)
	built := named(cfg, zap.New(
		newLogCore(cfg, opts...),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
	return &Logger{zlog: built}
}

// newGorm builds the gorm logger.Interface of the stream filename names,
// the way New reads it.
//
// The logger deliberately has no zap caller annotation: the wrapper depth
// between business code and the log call varies per operation, so a fixed
// AddCallerSkip would misattribute most statements. GormLogger.Trace walks
// the stack itself and attaches the business caller as a plain field.
func newGorm(filename string) gormlogger.Interface {
	cfg := readConf(filename)
	built := named(cfg, zap.New(
		newLogCore(cfg),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
	return &GormLogger{l: &Logger{zlog: built}}
}

// newAccess builds the *zap.Logger of a stream whose entries are their fields
// alone, with no message and no level: the access logs of both listeners and
// the HTTP body log. filename names the stream the way New reads it.
func newAccess(filename string) *zap.Logger {
	cfg := readConf(filename)
	return named(cfg, zap.New(newLogCore(cfg, Option{DisableMsg: true, DisableLevel: true})))
}

// newZap builds the *zap.Logger of the stream filename names, the way New
// reads it.
func newZap(filename string) *zap.Logger {
	cfg := readConf(filename)
	return named(cfg, zap.New(
		newLogCore(cfg),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
}

// named names a logger after the stream it writes in stdout mode, where the
// stream has no file of its own to tell it apart: zap writes the name as the
// entry's logger field. In file mode the logger stays unnamed, and its entries
// unchanged.
func named(cfg logConfig, l *zap.Logger) *zap.Logger {
	if cfg.output != config.LoggerOutputStdout {
		return l
	}
	return l.Named(streamName(cfg.file))
}

// streamName returns the name a stream goes by in stdout mode: its file name
// without the ".log" extension, or "global" for a stream that names no file.
func streamName(file string) string {
	switch file = strings.TrimSpace(file); file {
	case "", "/dev/stdout", "/dev/stderr":
		return "global"
	}
	return strings.TrimSuffix(filepath.Base(file), ".log")
}

// newLogCore builds the core every logger here writes through: the encoder,
// sink and level cfg and opts select, with the process identity stamped on
// every entry so the entries of replicas — on one host, or restarts of one
// pod — can be told apart wherever they end up collected together, and the
// error fields written as errorFieldCore says. The identity field is encoded
// once, here, not once per entry.
func newLogCore(cfg logConfig, opts ...Option) zapcore.Core {
	core := zapcore.NewCore(newLogEncoder(cfg, opts...), newLogWriter(cfg, opts...), newLogLevel(cfg))
	return &errorFieldCore{Core: core.With([]zapcore.Field{zap.String(logfield.KeyInstance, instance.ID())})}
}
