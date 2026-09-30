package logger

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/instance"
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
	level      string
	format     string
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
// Timestamp layout is deliberately not an option: consts.LayoutTimeEncoder
// applies to every entry, so entries from different files stay orderable
// against one another and a log store types the field the same way everywhere.
type Option struct {
	DisableMsg    bool
	DisableLevel  bool
	DisableCaller bool
	Console       bool
}

// New builds a types.Logger backed by *zap.Logger.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
// opts: optional encoder options
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

// NewGorm builds a gorm logger.Interface.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
//
// The logger deliberately has no zap caller annotation: the wrapper depth
// between business code and the log call varies per operation, so a fixed
// AddCallerSkip would misattribute most statements. GormLogger.Trace walks
// the stack itself and attaches the business caller as a plain field.
func NewGorm(filename string) gormlogger.Interface {
	cfg := readConf(filename)
	built := named(cfg, zap.New(
		newLogCore(cfg),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
	return &GormLogger{l: &Logger{zlog: built}}
}

// NewGin builds a *zap.Logger for Gin access logs.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
func NewGin(filename string) *zap.Logger {
	cfg := readConf(filename)
	return named(cfg, zap.New(newLogCore(cfg, Option{DisableMsg: true, DisableLevel: true})))
}

// NewStdLog builds a *log.Logger backed by *zap.Logger.
func NewStdLog() *log.Logger {
	return zap.NewStdLog(NewZap(""))
}

// NewZap builds a *zap.Logger with optional filename and options.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
// opts: optional encoder options
func NewZap(filename string, opts ...Option) *zap.Logger {
	cfg := readConf(filename)
	return named(cfg, zap.New(
		newLogCore(cfg, opts...),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
}

// NewSugared builds a *zap.SugaredLogger with optional filename and options:
// the logger NewZap builds, sugared.
func NewSugared(filename string, opts ...Option) *zap.SugaredLogger {
	return NewZap(filename, opts...).Sugar()
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
// pod — can be told apart wherever they end up collected together. The field
// is encoded once, here, not once per entry.
func newLogCore(cfg logConfig, opts ...Option) zapcore.Core {
	core := zapcore.NewCore(newLogEncoder(cfg, opts...), newLogWriter(cfg, opts...), newLogLevel(cfg))
	return core.With([]zapcore.Field{zap.String(consts.INSTANCE, instance.ID())})
}
