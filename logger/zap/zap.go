package zap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	logMaxAge     int
	logMaxSize    int
	logMaxBackups int

	bufferedLogWritersMu sync.Mutex
	bufferedLogWriters   []*zapcore.BufferedWriteSyncer
)

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

// Init initializes global loggers from config and wires subsystem loggers.
// Returns error on configuration or initialization failure.
func Init() error {
	readConf()
	opt := Option{Console: config.App.Logger.Console}
	zap.ReplaceGlobals(zap.New(
		zapcore.NewCore(newLogEncoder(opt), newLogWriter(opt), newLogLevel(opt)),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.FatalLevel),
	))

	logger.App = New("app.log")

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

	// Optional provider loggers start on a fallback sharing the global core:
	// non-nil and safe to use, but owning no file and no sink of their own.
	// Bootstrap's provider drain replaces each with a dedicated logger for
	// the providers actually compiled in (see provider.Provider.Logger), so
	// a log file exists exactly for the capabilities the binary carries.
	logger.Cassandra = newProviderFallback("cassandra")
	logger.Elastic = newProviderFallback("elastic")
	logger.Etcd = newProviderFallback("etcd")
	logger.Influxdb = newProviderFallback("influxdb")
	logger.Kafka = newProviderFallback("kafka")
	logger.Ldap = newProviderFallback("ldap")
	logger.Minio = newProviderFallback("minio")
	logger.Mongo = newProviderFallback("mongo")
	logger.Mqtt = newProviderFallback("mqtt")
	logger.Nats = newProviderFallback("nats")
	logger.Scylla = newProviderFallback("scylla")
	logger.RethinkDB = newProviderFallback("rethinkdb")
	logger.RocketMQ = newProviderFallback("rocketmq")

	logger.Gin = NewGin("access.log")
	logger.HTTPBody = NewGin("http_body.log")
	logger.Gorm = NewGorm("gorm.log")

	return nil
}

func Clean() {
	// types.Logger
	_ = zap.L().Sync()
	logs := []types.Logger{
		logger.App,

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
		logger.Influxdb,
		logger.Kafka,
		logger.Ldap,
		logger.Minio,
		logger.Mongo,
		logger.Mqtt,
		logger.Nats,
		logger.Scylla,
		logger.RethinkDB,
		logger.RocketMQ,
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

	// HTTP body logger
	if logger.HTTPBody != nil {
		_ = logger.HTTPBody.Sync()
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

	stopBufferedLogWriters()
}

// newProviderFallback builds the logger an optional provider variable holds
// until bootstrap's provider drain assigns its dedicated one. It derives from
// the global zap logger installed by Init — no file, no extra sink, and in
// particular no second lumberjack instance on any path — so an entry written
// through it lands in the global log stream, tagged with the component name.
// In a process that never ran Init (unit tests), the global logger is zap's
// no-op and the entry is dropped, which matches how such processes behave for
// every other logger. The caller-skip mirrors New so callers are attributed
// identically through either logger.
func newProviderFallback(component string) types.Logger {
	return (&Logger{zlog: zap.L().WithOptions(zap.AddCallerSkip(1))}).With("component", component)
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
//
// The logger deliberately has no zap caller annotation: the wrapper depth
// between business code and the log call varies per operation, so a fixed
// AddCallerSkip would misattribute most statements. GormLogger.Trace walks
// the stack itself and attaches the business caller as a plain field.
func NewGorm(filename string) gorml.Interface {
	readConf()
	if len(filename) > 0 {
		logFile = filename
	}
	logger := zap.New(
		zapcore.NewCore(newLogEncoder(), newLogWriter(), newLogLevel()),
		zap.AddStacktrace(zapcore.FatalLevel),
	)
	return &GormLogger{l: &Logger{zlog: logger}}
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
// opts: opts[0].Console additionally mirrors a file sink to os.Stdout.
func newLogWriter(opts ...Option) zapcore.WriteSyncer {
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

// newLogEncoder builds JSON/console encoder with optional field suppression.
// opt: encoder options
func newLogEncoder(opt ...Option) zapcore.Encoder {
	encConfig := zap.NewProductionEncoderConfig()
	// encConfig.EncodeCaller = zapcore.ShortCallerEncoder
	// encConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	encConfig.EncodeTime = utcTimeEncoder
	encConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	// Nanoseconds, matching util.LogDuration. The zap production default encodes
	// durations as floating point seconds, which rounds sub-millisecond work to
	// zero and leaves a log store guessing between integer and float. This
	// backstops any time.Duration that reaches a logger without going through
	// util.LogDuration, so no entry can carry a differently scaled duration.
	encConfig.EncodeDuration = zapcore.NanosDurationEncoder
	// Reflected values collapse into a single JSON string field instead of a
	// nested object; see stringifyReflectedEncoder for why.
	encConfig.NewReflectedEncoder = newStringifyReflectedEncoder
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

// utcTimeEncoder renders the entry timestamp in UTC using
// consts.LayoutTimeEncoder. The conversion happens here, the single point
// every logger's encoder is built at, so no host zone can leak into a log
// entry; see the layout constant for why the stream is UTC.
func utcTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.UTC().Format(consts.LayoutTimeEncoder))
}

// stringifyReflectedEncoder renders every reflected log value as a single
// JSON string instead of a nested JSON object. zap falls back to reflection
// for values no typed Field constructor understands (structs, maps, and
// slices or arrays of either), and the default reflected encoder inlines
// their JSON shape into the entry. A log store that indexes each key then
// grows its field mapping with every distinct shape logged anywhere in the
// codebase and eventually hits its per-index field cap, after which it drops
// entries. Collapsing the value into one string field keeps the mapping
// bounded no matter what gets logged, and the string still carries the
// value's JSON, so the content stays machine-readable. Typed fields and
// zapcore.ObjectMarshaler implementations are unaffected; the marshaler
// escape hatch is reserved for framework-internal objects whose key sets are
// fixed, never for open-ended shapes such as data models.
type stringifyReflectedEncoder struct{ w io.Writer }

// newStringifyReflectedEncoder is the zapcore.EncoderConfig.NewReflectedEncoder
// hook wired by newLogEncoder, the single point every logger's encoder is
// built at, so all loggers share the bounded-field behavior.
func newStringifyReflectedEncoder(w io.Writer) zapcore.ReflectedEncoder {
	return stringifyReflectedEncoder{w: w}
}

// reflectedValueBuffers pools the buffer holding a value's JSON between the
// two encoding passes below. Reflected values reach the encoder on every
// request that logs one, so the buffer would otherwise be allocated per entry.
var reflectedValueBuffers = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// maxPooledReflectedValueBytes caps the buffer a finished Encode hands back.
// sync.Pool assumes its entries cost about the same, so a single oversized
// value would otherwise pin its buffer for the life of the process. The limit
// matches the one the standard library's fmt package applies to its own pooled
// output buffer, and is far above any value worth reading back in a log entry.
// See https://go.dev/issue/23199.
const maxPooledReflectedValueBytes = 64 * 1024

// releaseReflectedValueBuffer returns buf to the pool unless it outgrew the
// pooled size, in which case it is dropped for the garbage collector.
func releaseReflectedValueBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledReflectedValueBytes {
		return
	}

	buf.Reset()
	reflectedValueBuffers.Put(buf)
}

// Encode implements zapcore.ReflectedEncoder. It writes the value's JSON into
// a pooled buffer, then writes that JSON as one JSON string, so the entry
// gains a single string field. A value json cannot handle (cycles, channels,
// functions) falls back to Go syntax rather than failing the whole entry.
//
// Both passes leave <, > and & as written. HTML escaping exists to keep JSON
// safe inside an HTML document, and a log entry is never rendered as one: it
// is consumed by log stores and by people reading them. The only effect here
// would be turning those three characters into six-character escapes, and they
// appear densely in exactly the payloads worth reading back — third-party
// error pages, URLs and query strings. Escaping has to be off on both passes:
// the second one re-escapes the characters the first one left alone, so
// disabling it on the value pass by itself changes nothing.
func (e stringifyReflectedEncoder) Encode(value any) error {
	buf, _ := reflectedValueBuffers.Get().(*bytes.Buffer)
	defer releaseReflectedValueBuffer(buf)

	if err := newVerbatimJSONEncoder(buf).Encode(value); err != nil {
		buf.Reset()
		fmt.Fprintf(buf, "%#v", value)
	}

	// Encode terminates its output with a newline, which must not travel into
	// the quoted string; zapcore trims the one this second pass appends.
	return newVerbatimJSONEncoder(e.w).Encode(string(bytes.TrimRight(buf.Bytes(), "\n")))
}

// newVerbatimJSONEncoder returns a json encoder that emits <, > and & as
// written instead of escaping them.
func newVerbatimJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	return enc
}

func readConf() {
	mode = config.App.Mode
	logFile = config.App.Logger.File
	logLevel = config.App.Logger.Level
	logFormat = config.App.Logger.Format
	logMaxAge = config.App.Logger.MaxAge
	logMaxSize = config.App.Logger.MaxSize
	logMaxBackups = config.App.Logger.MaxBackups
}
