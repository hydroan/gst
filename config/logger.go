package config

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
)

const (
	LOGGER_OUTPUT                  = "LOGGER_OUTPUT"
	LOGGER_DIR                     = "LOGGER_DIR"
	LOGGER_PREFIX                  = "LOGGER_PREFIX"
	LOGGER_FILE                    = "LOGGER_FILE"
	LOGGER_CONSOLE                 = "LOGGER_CONSOLE"
	LOGGER_LEVEL                   = "LOGGER_LEVEL"
	LOGGER_FORMAT                  = "LOGGER_FORMAT"
	LOGGER_ERROR_STACK_DISABLED    = "LOGGER_ERROR_STACK_DISABLED"
	LOGGER_MAX_AGE                 = "LOGGER_MAX_AGE"
	LOGGER_MAX_SIZE                = "LOGGER_MAX_SIZE"
	LOGGER_MAX_BACKUPS             = "LOGGER_MAX_BACKUPS"
	LOGGER_HTTP_BODY_ENABLED       = "LOGGER_HTTP_BODY_ENABLED"
	LOGGER_HTTP_BODY_LOG_REQUEST   = "LOGGER_HTTP_BODY_LOG_REQUEST"
	LOGGER_HTTP_BODY_LOG_RESPONSE  = "LOGGER_HTTP_BODY_LOG_RESPONSE"
	LOGGER_HTTP_BODY_MAX_BODY_SIZE = "LOGGER_HTTP_BODY_MAX_BODY_SIZE"
	LOGGER_HTTP_BODY_SKIP_ROUTES   = "LOGGER_HTTP_BODY_SKIP_ROUTES"

	LOGGER_SQL_CALLER_SKIP_PREFIXES = "LOGGER_SQL_CALLER_SKIP_PREFIXES"
)

// Logger represents section "logger" for client-side or server-side configuration,
// and there is only one copy during the application entire lifetime.
type Logger struct {
	// Output selects where every log stream writes (stdout|file), in any
	// letter case. Any other value is a configuration error at startup.
	//
	// With stdout, every stream — the global one, access, SQL, HTTP body, the
	// component streams such as cronjob and leader, and the rest — writes to
	// the process's stdout, one line per entry, and the entry's logger field
	// names the stream: its file name in file mode without ".log" ("access",
	// "gorm", "cronjob"), and for the global stream File without ".log", or
	// "global" when File names no file. No file is created or rotated, so
	// Dir, Console, MaxAge, MaxSize and MaxBackups do not apply.
	//
	// With file, every stream writes to a file of its own under Dir, rotated
	// by MaxAge, MaxSize and MaxBackups, and its entries carry no logger field.
	//
	// Stdout and the files buffer entries and write them out within a second,
	// and in full when the process stops. In file mode a stream naming no file,
	// or naming /dev/stdout or /dev/stderr, writes straight through.
	// Default: stdout
	Output LoggerOutput `json:"output" ini:"output" yaml:"output" mapstructure:"output"`

	// Dir specifies which directory log to.
	Dir string `json:"dir" ini:"dir" yaml:"dir" mapstructure:"dir"`

	// Prefix specifies the log prefix.
	// You can set the prefix name to your project name.
	Prefix string `json:"prefix" ini:"prefix" yaml:"prefix" mapstructure:"prefix"`

	// File specifies the which file the global logger logs to in file mode.
	// If value is "/dev/stdout", log to os.Stdout.
	// If value is "/dev/stderr", log to os.Stderr.
	// If value is empty(length is zero), log to os.Stdout.
	// In stdout mode it only names the global stream (see Output).
	File string `json:"file" ini:"file" yaml:"file" mapstructure:"file"`

	// Console additionally mirrors the global logger's output to os.Stdout
	// when File is set to a real file path in file mode. It has no effect when
	// File is empty or one of "/dev/stdout"/"/dev/stderr", since those already
	// log to console, nor in stdout mode, where every stream already does.
	// Only the global logger built by Init() reads this field; subsystem
	// loggers stay file-only unless the caller opts in explicitly via
	// zap.Option.Console.
	// Default: true
	Console bool `json:"console" ini:"console" yaml:"console" mapstructure:"console"`

	// Level is the least severe level the log streams write, one zap knows
	// (debug|info|warn|error|dpanic|panic|fatal, warning for warn), in any
	// letter case. Any other value is a configuration error at startup.
	// Default: info
	Level LogLevel `json:"level" ini:"level" yaml:"level" mapstructure:"level"`

	// Format lays out each log entry (json|text), in any letter case. Any
	// other value is a configuration error at startup.
	// Default: json
	Format LogFormat `json:"format" ini:"format" yaml:"format" mapstructure:"format"`

	// ErrorStackDisabled disables attaching the error_stack field to
	// error-level logs when a logged error carries a stack trace embedded at
	// its creation point. Stack traces are attached by default; set this to
	// true in environments where the extra payload is unwanted, such as local
	// development.
	// Default: false
	ErrorStackDisabled bool `json:"error_stack_disabled" ini:"error_stack_disabled" yaml:"error_stack_disabled" mapstructure:"error_stack_disabled"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.
	// uint is "day" and default to 30.
	MaxAge int `json:"max_age" ini:"max_age" yaml:"max_age" mapstructure:"max_age"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated, default to 100MB.
	MaxSize int `json:"max_size" ini:"max_size" yaml:"max_size" mapstructure:"max_size"`

	// MaxBackups is the maximum number of old log files to retain.
	// The value default to 1.
	MaxBackups int `json:"max_backups" ini:"max_backups" yaml:"max_backups" mapstructure:"max_backups"`

	// SQLCallerSkipPrefixes lists extra function-path prefixes to skip when
	// resolving the business caller of a SQL statement log, on top of the
	// built-in gorm and framework prefixes, which are always skipped and
	// cannot be removed. Configure it when a project routes database calls
	// through its own helper layer, such as a dao or repository package, so
	// SQL logs name the code that called the helper instead of the helper
	// body. Each entry is a function-path prefix matched at a package
	// boundary: "example.com/app/dao" covers that package and its
	// subpackages but not "example.com/app/daox". The environment form is
	// comma-separated, eg.
	// LOGGER_SQL_CALLER_SKIP_PREFIXES="example.com/app/dao,example.com/app/repo".
	SQLCallerSkipPrefixes []string `json:"sql_caller_skip_prefixes" ini:"sql_caller_skip_prefixes" yaml:"sql_caller_skip_prefixes" mapstructure:"sql_caller_skip_prefixes"`

	// HTTPBody contains HTTP request and response body logging configurations.
	HTTPBody HTTPBodyLogger `json:"http_body" ini:"http_body" yaml:"http_body" mapstructure:"http_body"`
}

// LoggerOutput selects where every log stream writes.
type LoggerOutput string

const (
	// LoggerOutputStdout writes every log stream to stdout, each entry naming
	// its stream, which is what a container platform collects.
	LoggerOutputStdout LoggerOutput = "stdout"
	// LoggerOutputFile writes every log stream to a rotated file of its own,
	// for a deployment whose log collector reads files.
	LoggerOutputFile LoggerOutput = "file"
)

// UnmarshalText reads text as one of the outputs, in any letter case; any
// other text is a configuration error naming the key.
func (o *LoggerOutput) UnmarshalText(text []byte) (err error) {
	*o, err = oneOf(text, LoggerOutputStdout, LoggerOutputFile)
	return err
}

// LogLevel names the least severe level the log streams write, one of the
// levels zap knows.
type LogLevel string

const (
	LogLevelDebug  LogLevel = "debug"
	LogLevelInfo   LogLevel = "info"
	LogLevelWarn   LogLevel = "warn"
	LogLevelError  LogLevel = "error"
	LogLevelDPanic LogLevel = "dpanic"
	LogLevelPanic  LogLevel = "panic"
	LogLevelFatal  LogLevel = "fatal"
)

// UnmarshalText reads text as a level zap parses, in any letter case, and
// keeps it as zap spells it, so warning reads as warn; any other text is a
// configuration error naming the key.
func (l *LogLevel) UnmarshalText(text []byte) error {
	value := strings.TrimSpace(string(text))
	if value == "" {
		*l = ""
		return nil
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return errors.Newf("want one of %s, %s, %s, %s, %s, %s, %s, got %q",
			LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelDPanic, LogLevelPanic, LogLevelFatal, text)
	}
	*l = LogLevel(level.String())
	return nil
}

// LogFormat names how a log entry is laid out: json, one object per line,
// or text, the console layout.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// UnmarshalText reads text as one of the formats, in any letter case; any
// other text is a configuration error naming the key.
func (f *LogFormat) UnmarshalText(text []byte) (err error) {
	*f, err = oneOf(text, LogFormatJSON, LogFormatText)
	return err
}

// HTTPBodyLogMode selects which finished requests get a captured HTTP body
// written to the log.
type HTTPBodyLogMode string

const (
	// HTTPBodyLogModeAll writes the captured body for every request.
	HTTPBodyLogModeAll HTTPBodyLogMode = "all"
	// HTTPBodyLogModeError writes the captured body only for failed requests:
	// HTTP status >= 400.
	HTTPBodyLogModeError HTTPBodyLogMode = "error"
	// HTTPBodyLogModeNone disables body logging for that side entirely.
	HTTPBodyLogModeNone HTTPBodyLogMode = "none"
)

// UnmarshalText reads text as one of the modes, in any letter case; any
// other text is a configuration error naming the key.
func (m *HTTPBodyLogMode) UnmarshalText(text []byte) (err error) {
	*m, err = oneOf(text, HTTPBodyLogModeAll, HTTPBodyLogModeError, HTTPBodyLogModeNone)
	return err
}

// HTTPBodyLogger represents HTTP body logging configuration.
type HTTPBodyLogger struct {
	// Enabled enables the HTTP request and response body logging middleware.
	// Default: true
	Enabled bool `json:"enabled" ini:"enabled" yaml:"enabled" mapstructure:"enabled"`

	// LogRequest selects which requests get their JSON body logged
	// (all|error|none), in any letter case. Empty is the default; any other
	// value is a configuration error at startup.
	// Default: all
	LogRequest HTTPBodyLogMode `json:"log_request" ini:"log_request" yaml:"log_request" mapstructure:"log_request"`

	// LogResponse selects which responses get their JSON body logged
	// (all|error|none), in any letter case. Empty is the default; any other
	// value is a configuration error at startup.
	// Default: error
	LogResponse HTTPBodyLogMode `json:"log_response" ini:"log_response" yaml:"log_response" mapstructure:"log_response"`

	// MaxBodySize limits how much request or response body data can be logged,
	// a size with its unit (see ByteSize); empty or 0 is the default.
	// Default: 64KB
	MaxBodySize ByteSize `json:"max_body_size" ini:"max_body_size" yaml:"max_body_size" mapstructure:"max_body_size"`

	// SkipRoutes lists routes whose bodies are never logged, for example login
	// or other credential-carrying endpoints. Bodies are logged verbatim and
	// are never redacted, so every route accepting a password, token, or other
	// secret belongs here. A pattern ending in "*" matches the route by prefix;
	// any other pattern must match the registered route pattern exactly, so a
	// route with path parameters is written with them ("/users/:id/password").
	SkipRoutes []string `json:"skip_routes" ini:"skip_routes" yaml:"skip_routes" mapstructure:"skip_routes"`
}

func (*Logger) setDefault(v *viper.Viper) {
	v.SetDefault("logger.output", LoggerOutputStdout)
	v.SetDefault("logger.dir", "./logs")
	v.SetDefault("logger.prefix", "")
	v.SetDefault("logger.file", "")
	v.SetDefault("logger.console", true)
	v.SetDefault("logger.level", LogLevelInfo)
	v.SetDefault("logger.format", LogFormatJSON)
	v.SetDefault("logger.error_stack_disabled", false)
	v.SetDefault("logger.max_age", 30)
	v.SetDefault("logger.max_size", 100)
	v.SetDefault("logger.max_backups", 1)
	v.SetDefault("logger.sql_caller_skip_prefixes", []string{})
	v.SetDefault("logger.http_body.enabled", true)
	v.SetDefault("logger.http_body.log_request", HTTPBodyLogModeAll)
	v.SetDefault("logger.http_body.log_response", HTTPBodyLogModeError)
	v.SetDefault("logger.http_body.max_body_size", "64KB")
	v.SetDefault("logger.http_body.skip_routes", []string{})
}
