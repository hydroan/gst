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

// The logger configuration as the last constructor read it (see readConf):
// what every logger built since is made of.
var (
	logOutput     config.LoggerOutput
	logFile       string
	logLevel      string
	logFormat     string
	logMaxAge     int
	logMaxSize    int
	logMaxBackups int
)

// readConf takes the logger configuration as it stands, for the logger
// built next.
func readConf() {
	logOutput = config.App.Logger.Output
	logFile = config.App.Logger.File
	logLevel = config.App.Logger.Level
	logFormat = config.App.Logger.Format
	logMaxAge = config.App.Logger.MaxAge
	logMaxSize = config.App.Logger.MaxSize
	logMaxBackups = config.App.Logger.MaxBackups
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
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	built := named(zap.New(
		newLogCore(opts...),
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
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	built := named(zap.New(
		newLogCore(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
	return &GormLogger{l: &Logger{zlog: built}}
}

// NewGin builds a *zap.Logger for Gin access logs.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
func NewGin(filename string) *zap.Logger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return named(zap.New(newLogCore(Option{DisableMsg: true, DisableLevel: true})))
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
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return named(zap.New(
		newLogCore(opts...),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))
}

// NewSugared builds a *zap.SugaredLogger with optional filename and options.
// filename: the stream's log file, whose name without ".log" names the
// stream in stdout mode ("/dev/stdout" for console)
// opts: optional encoder options
func NewSugared(filename string, opts ...Option) *zap.SugaredLogger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return named(zap.New(
		newLogCore(opts...),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	)).Sugar()
}

// named names a logger after the stream it writes in stdout mode, where the
// stream has no file of its own to tell it apart: zap writes the name as the
// entry's logger field. In file mode the logger stays unnamed, and its entries
// unchanged.
func named(l *zap.Logger) *zap.Logger {
	if logOutput != config.LoggerOutputStdout {
		return l
	}
	return l.Named(streamName(logFile))
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
// sink and level opts select, with the process identity stamped on every
// entry so the entries of replicas — on one host, or restarts of one pod —
// can be told apart wherever they end up collected together. The field is
// encoded once, here, not once per entry.
func newLogCore(opts ...Option) zapcore.Core {
	core := zapcore.NewCore(newLogEncoder(opts...), newLogWriter(opts...), newLogLevel())
	return core.With([]zapcore.Field{zap.String(consts.INSTANCE, instance.ID())})
}
