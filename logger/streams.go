// Package logger holds the framework's log streams, one per component,
// and the loggers behind them: Init opens the streams the configuration
// names, and New builds a logger of its own for a component that has one.
package logger

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gormlogger "gorm.io/gorm/logger"
)

var (
	// App is the general-purpose business logger for application code (model
	// hooks, service methods, etc.) that wants WithContext-enriched logging
	// without going through zap.S()/zap.L(). Unlike those globals, App never
	// mirrors output to stdout.
	App types.Logger

	Controller types.Logger
	Service    types.Logger
	Database   types.Logger
	Cache      types.Logger
	Dcache     types.Logger
	Redis      types.Logger

	Authz     types.Logger
	OTEL      types.Logger
	Cassandra types.Logger
	Elastic   types.Logger
	Etcd      types.Logger
	Influxdb  types.Logger
	Kafka     types.Logger
	Ldap      types.Logger
	Minio     types.Logger
	Mongo     types.Logger
	Mqtt      types.Logger
	Nats      types.Logger
	RethinkDB types.Logger
	RocketMQ  types.Logger
	Scylla    types.Logger

	// Gin is the access log of the HTTP listener and GRPC that of the gRPC
	// listener, one entry per request or call; HTTPBody carries the request
	// and response bodies the HTTP listener logs.
	Gin      *zap.Logger
	HTTPBody *zap.Logger
	GRPC     *zap.Logger
	// Recovery records the panics the handlers of both listeners recover
	// from, with their stacks. One logger serves both so the file has one
	// writer: a second one on the same path would race it at rotation.
	Recovery *zap.Logger
	Gorm     gormlogger.Interface
)

// Init builds every stream from the configuration: the global zap logger,
// the component loggers, the fallbacks of the optional providers and the
// listeners' loggers. It fails when the configured output is neither stdout
// nor file.
func Init() error {
	cfg := readConf("")
	if cfg.output != config.LoggerOutputStdout && cfg.output != config.LoggerOutputFile {
		return errors.Newf("logger.output must be %q or %q, not %q", config.LoggerOutputStdout, config.LoggerOutputFile, cfg.output)
	}
	opt := Option{Console: config.App.Logger.Console}
	zap.ReplaceGlobals(named(cfg, zap.New(
		newLogCore(cfg, opt),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	)))

	App = New("app.log")

	Controller = New("controller.log")
	Service = New("service.log")
	Database = New("database.log")
	Cache = New("cache.log")
	Dcache = New("dcache.log")
	Redis = New("redis.log")

	Authz = New("authz.log", Option{DisableMsg: true, DisableCaller: true})
	OTEL = New("otel.log")

	// Optional provider loggers start on a fallback sharing the global core:
	// non-nil and safe to use, but owning no stream of their own. The
	// lifecycle registry replaces each with a dedicated logger for the
	// providers actually compiled in (see lifecycle.Component.SetLogger), so a
	// stream of its own — a log file in file mode — exists exactly for the
	// capabilities the binary carries.
	Cassandra = Fallback("cassandra")
	Elastic = Fallback("elastic")
	Etcd = Fallback("etcd")
	Influxdb = Fallback("influxdb")
	Kafka = Fallback("kafka")
	Ldap = Fallback("ldap")
	Minio = Fallback("minio")
	Mongo = Fallback("mongo")
	Mqtt = Fallback("mqtt")
	Nats = Fallback("nats")
	Scylla = Fallback("scylla")
	RethinkDB = Fallback("rethinkdb")
	RocketMQ = Fallback("rocketmq")

	Gin = NewGin("access.log")
	HTTPBody = NewGin("http_body.log")
	GRPC = NewGin("grpc.log")
	// A panic entry is its message — the request, the panic and the stack —
	// so the recovery log keeps the message and the level the access-log
	// encoder leaves out.
	Recovery = NewZap("recovery.log")
	Gorm = NewGorm("gorm.log")

	return nil
}

// Clean flushes the loggers built by Init and stops every buffered writer the
// constructors registered — the file writers and the shared stdout sink — so
// a process about to exit, or a test about to read a log back, sees everything
// that was logged.
func Clean() {
	_ = zap.L().Sync()
	// The component streams.
	logs := []types.Logger{
		App,

		Controller,
		Service,
		Database,
		Cache,
		Dcache,
		Redis,

		Authz,
		OTEL,
		Cassandra,
		Elastic,
		Etcd,
		Influxdb,
		Kafka,
		Ldap,
		Minio,
		Mongo,
		Mqtt,
		Nats,
		Scylla,
		RethinkDB,
		RocketMQ,
	}
	for _, log := range logs {
		if l, ok := log.(*Logger); ok {
			_ = l.zlog.Sync()
		}
	}

	// The listeners' loggers: the HTTP access and body logs, the gRPC
	// access log and the recovery log.
	for _, log := range []*zap.Logger{Gin, HTTPBody, GRPC, Recovery} {
		if log != nil {
			_ = log.Sync()
		}
	}

	// The SQL log.
	if gorm, ok := Gorm.(*GormLogger); ok {
		if l, ok := gorm.l.(*Logger); ok {
			_ = l.zlog.Sync()
		}
	}

	stopBufferedLogWriters()
}

// Fallback builds the logger a component's logger variable holds until the
// lifecycle registry binds its dedicated one (see
// lifecycle.Component.SetLogger): a provider's until the provider stage
// starts, a lock's for a try made before Run. It derives from the global zap
// logger installed by Init — no file, no extra sink, and in particular no
// second lumberjack instance on any path, which the binding would then race
// at rotation — so an entry written through it lands in the global log
// stream, tagged with the component name. In a process that never ran Init
// (unit tests), the global logger is zap's no-op and the entry is dropped,
// which matches how such processes behave for every other logger. The
// caller-skip mirrors New so callers are attributed identically through
// either logger.
func Fallback(component string) types.Logger {
	return (&Logger{zlog: zap.L().WithOptions(zap.AddCallerSkip(1))}).With("component", component)
}
