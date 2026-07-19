package logrus

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/hydroan/gst/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logFile       string
	logLevel      string
	logFormat     string
	logEncoder    string //nolint: unused
	logMaxAge     int
	logMaxSize    int
	logMaxBackups int
)

// Init will init logrus global logger according to Server/Client configuration.
func Init() error {
	initVar()
	initLogFile()
	initLogLevel()
	initLogFormat()
	return nil
}

// New setup and new a *logrus.LOgger instance accounting to Server/Client configuration.
func New() *logrus.Logger {
	logger := logrus.New()
	initVar()
	initLogFile(logger)
	initLogLevel(logger)
	initLogFormat(logger)
	return logger
}

// NewEntry is the same as New function but returns a *logrus.Entry object.
func NewEntry() *logrus.Entry {
	initVar()
	return logrus.NewEntry(New())
}

// initLogFile setup log file for global logrus logger or the given *logrus.Logger.
func initLogFile(logger ...*logrus.Logger) {
	if len(logger) == 0 {
		switch logFile {
		case "/dev/stdout":
			logrus.SetOutput(os.Stdout)
		case "/dev/stderr":
			logrus.SetOutput(os.Stderr)
		case "":
			logrus.SetOutput(os.Stdout)
		default:
			writer := &lumberjack.Logger{
				Filename:   logFile,
				MaxAge:     logMaxAge,
				MaxSize:    logMaxSize,
				MaxBackups: logMaxBackups,
				LocalTime:  true,
				Compress:   false, // openwrt may not support to comporess.
			}
			logrus.SetOutput(writer)
			logrus.RegisterExitHandler(func() { writer.Close() })
		}
	} else {
		if logger[0] == nil {
			return
		}
		switch logFile {
		case "/dev/stdout":
			logger[0].SetOutput(os.Stdout)
		case "/dev/stderr":
			logger[0].SetOutput(os.Stderr)
		case "":
			logger[0].SetOutput(os.Stdout)
		default:
			writer := &lumberjack.Logger{
				Filename:   logFile,
				MaxAge:     logMaxAge,
				MaxSize:    logMaxSize,
				MaxBackups: logMaxBackups,
				LocalTime:  true,
				Compress:   false, // openwrt may not support to comporess.
			}
			logger[0].SetOutput(writer)
			logrus.RegisterExitHandler(func() { writer.Close() })
		}
	}
}

// initLogLevel setup log level for global logrus logger or the given *logrus.Logger.
func initLogLevel(logger ...*logrus.Logger) {
	if len(logger) == 0 {
		switch logLevel {
		case "debug":
			logrus.SetLevel(logrus.DebugLevel)
		case "info":
			logrus.SetLevel(logrus.InfoLevel)
		case "warn", "warning":
			logrus.SetLevel(logrus.WarnLevel)
		case "error":
			logrus.SetLevel(logrus.ErrorLevel)
		case "fatal":
			logrus.SetLevel(logrus.FatalLevel)
		default:
			logrus.SetLevel(logrus.InfoLevel)
		}
	} else {
		if logger[0] == nil {
			return
		}
		switch logLevel {
		case "debug":
			logger[0].SetLevel(logrus.DebugLevel)
		case "info":
			logger[0].SetLevel(logrus.InfoLevel)
		case "warn", "warning":
			logger[0].SetLevel(logrus.WarnLevel)
		case "error":
			logger[0].SetLevel(logrus.ErrorLevel)
		case "fatal":
			logger[0].SetLevel(logrus.FatalLevel)
		default:
			logger[0].SetLevel(logrus.InfoLevel)
		}
	}
}

// initLogFormat setup log format for global logrus logger or the given *logrus.Logger.
func initLogFormat(logger ...*logrus.Logger) {
	if len(logger) == 0 {
		logrus.SetReportCaller(true)
		switch logFormat {
		case "text", "console":
			logrus.SetFormatter(&logrus.TextFormatter{CallerPrettyfier: callerPrettyfier, DisableQuote: true})
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{CallerPrettyfier: callerPrettyfier})
		default:
			logrus.SetFormatter(&logrus.TextFormatter{CallerPrettyfier: callerPrettyfier, DisableColors: true})
		}
	} else {
		if logger[0] == nil {
			return
		}
		logger[0].SetReportCaller(true)
		switch logFormat {
		case "text", "console":
			logger[0].SetFormatter(&logrus.TextFormatter{CallerPrettyfier: callerPrettyfier, DisableQuote: true})
		case "json":
			logger[0].SetFormatter(&logrus.JSONFormatter{CallerPrettyfier: callerPrettyfier})
		default:
			logger[0].SetFormatter(&logrus.TextFormatter{CallerPrettyfier: callerPrettyfier, DisableColors: true})
		}
	}
}

func initVar() {
	logFile = config.App.Logger.File
	logLevel = config.App.Logger.Level
	logFormat = config.App.Logger.Format
	logEncoder = config.App.Logger.Encoder
	logMaxAge = config.App.Logger.MaxAge
	logMaxSize = config.App.Logger.MaxSize
	logMaxBackups = config.App.Logger.MaxBackups
}

func callerPrettyfier(frame *runtime.Frame) (function, file string) {
	// return frame.Function, filepath.Join(
	//    path.Base(filepath.Dir(frame.File)),
	//    path.Base(frame.File),
	// ) + ":" + strconv.Itoa(frame.Line)
	return "", filepath.Join(
		path.Base(filepath.Dir(frame.File)),
		path.Base(frame.File),
	) + ":" + strconv.Itoa(frame.Line)
}
