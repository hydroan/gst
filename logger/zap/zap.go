package zap

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	casbinl "github.com/casbin/casbin/v3/log"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	gorml "gorm.io/gorm/logger"
)

const (
	defaultLogBufferSize    = 256 * 1024
	defaultLogFlushInterval = time.Second
)

var (
	mode          config.Mode //nolint:unused
	logFile       string
	logLevel      string
	logFormat     string
	logEncoder    string //nolint:unused
	logMaxAge     int
	logMaxSize    int
	logMaxBackups int

	bufferedLogWritersMu sync.Mutex
	bufferedLogWriters   []*zapcore.BufferedWriteSyncer
)

// Option configures encoder behavior for constructors.
// DisableMsg/DisableLevel hide "msg" and "level" fields; TSLayout sets time format.
type Option struct {
	DisableMsg    bool
	DisableLevel  bool
	DisableCaller bool
	TSLayout      string
}

// Init initializes global loggers from config and wires subsystem loggers.
// Returns error on configuration or initialization failure.
func Init() error {
	readConf()
	zap.ReplaceGlobals(zap.New(
		zapcore.NewCore(newLogEncoder(), newLogWriter(), newLogLevel()),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))

	logger.Runtime = New("runtime.log")
	logger.Cronjob = New("cronjob.log")
	logger.Task = New("task.log")

	logger.Controller = New("controller.log")
	logger.Service = New("service.log")
	logger.Database = New("database.log")
	logger.Cache = New("cache.log")
	logger.Dcache = New("dcache.log")
	logger.Redis = New("redis.log")

	logger.Authz = New("authz.log", Option{DisableMsg: true, DisableCaller: true})
	logger.OTEL = New("otel.log")
	logger.Cassandra = New("cassandra.log")
	logger.Elastic = New("elastic.log")
	logger.Etcd = New("etcd.log")
	logger.Feishu = New("feishu.log")
	logger.Influxdb = New("influxdb.log")
	logger.Kafka = New("kafka.log")
	logger.Ldap = New("ldap.log")
	logger.Memcached = New("memcached.log")
	logger.Minio = New("minio.log")
	logger.Mongo = New("mongo.log")
	logger.Mqtt = New("mqtt.log")
	logger.Nats = New("nats.log")
	logger.Scylla = New("scylla.log")
	logger.RethinkDB = New("rethinkdb.log")
	logger.RocketMQ = New("rocketmq.log")

	logger.Protocol = New("protocol.log")
	logger.Binary = New("binary.log")

	logger.Gin = NewGin("access.log")
	logger.Gorm = NewGorm("gorm.log")
	logger.Casbin = NewCasbin("casbin.log")

	return nil
}

func Clean() {
	// types.Logger
	_ = zap.L().Sync()
	logs := []types.Logger{
		logger.Runtime,
		logger.Cronjob,
		logger.Task,

		logger.Controller,
		logger.Service,
		logger.Database,
		logger.Cache,
		logger.Redis,

		logger.Authz,
		logger.Cassandra,
		logger.Elastic,
		logger.Etcd,
		logger.Feishu,
		logger.Influxdb,
		logger.Kafka,
		logger.Ldap,
		logger.Memcached,
		logger.Minio,
		logger.Mongo,
		logger.Mqtt,
		logger.Nats,
		logger.Scylla,
		logger.RethinkDB,
		logger.RocketMQ,

		logger.Protocol,
		logger.Binary,
	}
	for _, log := range logs {
		if l, ok := log.(*Logger); ok {
			_ = l.zlog.Sync()
		}
	}

	// Gin logger
	if logger.Gin != nil {
		_ = logger.Gin.Sync()
	}

	// gorm logger
	gormLogs := []gorml.Interface{
		logger.Gorm,
	}
	for _, glog := range gormLogs {
		if log, ok := glog.(*GormLogger); ok {
			if l, ok := log.l.(*Logger); ok {
				_ = l.zlog.Sync()
			}
		}
	}

	// casbin logger
	casbinLogs := []casbinl.Logger{
		logger.Casbin,
	}
	for _, clog := range casbinLogs {
		if log, ok := clog.(*CasbinLogger); ok {
			if l, ok := log.l.(*Logger); ok {
				_ = l.zlog.Sync()
			}
		}
	}

	stopBufferedLogWriters()
}

// New builds a types.Logger backed by *zap.Logger.
// filename: target log file name ("/dev/stdout" for console)
// opts: optional encoder options
func New(filename string, opts ...Option) *Logger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	logger := zap.New(
		zapcore.NewCore(newLogEncoder(opts...), newLogWriter(opts...), newLogLevel(opts...)),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.FatalLevel),
	)
	return &Logger{zlog: logger}
}

// NewGorm builds a gorm logger.Interface.
// filename: target log file name ("/dev/stdout" for console)
func NewGorm(filename string) gorml.Interface {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	logger := zap.New(
		zapcore.NewCore(newLogEncoder(), newLogWriter(), newLogLevel()),
		zap.AddCaller(),
		zap.AddCallerSkip(5),
		zap.AddStacktrace(zapcore.FatalLevel),
	)
	return &GormLogger{l: &Logger{zlog: logger}}
}

// NewCasbin builds a casbin Logger (no caller field).
// filename: target log file name ("/dev/stdout" for console)
func NewCasbin(filename string) casbinl.Logger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	logger := zap.New(
		zapcore.NewCore(newLogEncoder(Option{DisableMsg: true}), newLogWriter(), newLogLevel()),
		zap.AddStacktrace(zapcore.FatalLevel),
	)
	return &CasbinLogger{l: &Logger{zlog: logger}}
}

// NewGin builds a *zap.Logger for Gin access logs.
// filename: target log file name ("/dev/stdout" for console)
func NewGin(filename string) *zap.Logger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return zap.New(zapcore.NewCore(newLogEncoder(Option{DisableMsg: true, DisableLevel: true}), newLogWriter(), newLogLevel()))
}

// NewStdLog builds a *log.Logger backed by *zap.Logger.
func NewStdLog() *log.Logger {
	return zap.NewStdLog(NewZap(""))
}

// NewZap builds a *zap.Logger with optional filename and options.
// filename: target log file name ("/dev/stdout" for console)
// opts: optional encoder options
func NewZap(filename string, opts ...Option) *zap.Logger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return zap.New(
		zapcore.NewCore(newLogEncoder(opts...), newLogWriter(opts...), newLogLevel(opts...)),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	)
}

// NewSugared builds a *zap.SugaredLogger with optional filename and options.
// filename: target log file name ("/dev/stdout" for console)
// opts: optional encoder options
func NewSugared(filename string, opts ...Option) *zap.SugaredLogger {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	return zap.New(
		zapcore.NewCore(newLogEncoder(opts...), newLogWriter(opts...), newLogLevel(opts...)),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	).Sugar()
}

// newLogWriter selects log sink (stdout/stderr or rolling file).
// opts: reserved for future expansion
func newLogWriter(_ ...Option) zapcore.WriteSyncer {
	switch strings.TrimSpace(logFile) {
	case "/dev/stdout":
		return zapcore.AddSync(os.Stdout)
	case "/dev/stderr":
		return zapcore.AddSync(os.Stderr)
	case "":
		return zapcore.AddSync(os.Stdout)
	default:
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
		return writer
	}
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
	bufferedLogWritersMu.Lock()
	writers := bufferedLogWriters
	bufferedLogWriters = nil
	bufferedLogWritersMu.Unlock()

	for _, writer := range writers {
		_ = writer.Stop()
	}
}

// newLogLevel parses configured level; defaults to Info.
// opts: reserved for future expansion
func newLogLevel(_ ...Option) zapcore.Level {
	if len(logLevel) == 0 {
		return zapcore.InfoLevel
	}
	level := new(zapcore.Level)
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return zapcore.InfoLevel
	}
	return *level
}

// newLogEncoder builds JSON/console encoder with optional field suppression and time layout.
// opt: encoder options
func newLogEncoder(opt ...Option) zapcore.Encoder {
	encConfig := zap.NewProductionEncoderConfig()
	// encConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	// encConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	// encConfig.EncodeDuration = zapcore.MillisDurationEncoder
	// encConfig.EncodeCaller = zapcore.ShortCallerEncoder
	// encConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	// encConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	encConfig.EncodeTime = zapcore.TimeEncoderOfLayout(consts.LayoutTimeEncoder)
	encConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	// encConfig.EncodeCaller = zapcore.ShortCallerEncoder
	// encConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	// encConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	// encConfig.EncodeLevel = colorfulLevelEncoder
	if len(opt) > 0 {
		o := opt[0]
		if o.DisableMsg {
			encConfig.MessageKey = ""
		}
		if o.DisableLevel {
			encConfig.LevelKey = ""
		}
		if o.DisableCaller {
			encConfig.CallerKey = ""
		}
		if len(o.TSLayout) > 0 {
			encConfig.EncodeTime = zapcore.TimeEncoderOfLayout(o.TSLayout)
		}
	}
	switch strings.ToLower(logFormat) {
	case "json":
		return zapcore.NewJSONEncoder(encConfig)
	case "text", "console":
		// return newCustomConsoleEncoder(encConfig)
		return zapcore.NewConsoleEncoder(encConfig)
	default:
		return zapcore.NewJSONEncoder(encConfig)
	}
}

func readConf() {
	mode = config.App.Mode
	logFile = config.App.Logger.File
	logLevel = config.App.Logger.Level
	logFormat = config.App.Logger.Format
	logEncoder = config.App.Logger.Encoder
	logMaxAge = config.App.Logger.MaxAge
	logMaxSize = config.App.Logger.MaxSize
	logMaxBackups = config.App.Logger.MaxBackups
}
