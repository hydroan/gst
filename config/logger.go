package config

const (
	LOGGER_DIR                     = "LOGGER_DIR"                     //nolint:staticcheck
	LOGGER_PREFIX                  = "LOGGER_PREFIX"                  //nolint:staticcheck
	LOGGER_FILE                    = "LOGGER_FILE"                    //nolint:staticcheck
	LOGGER_CONSOLE                 = "LOGGER_CONSOLE"                 //nolint:staticcheck
	LOGGER_LEVEL                   = "LOGGER_LEVEL"                   //nolint:staticcheck
	LOGGER_FORMAT                  = "LOGGER_FORMAT"                  //nolint:staticcheck
	LOGGER_ENCODER                 = "LOGGER_ENCODER"                 //nolint:staticcheck
	LOGGER_ERROR_STACK_DISABLED    = "LOGGER_ERROR_STACK_DISABLED"    //nolint:staticcheck
	LOGGER_MAX_AGE                 = "LOGGER_MAX_AGE"                 //nolint:staticcheck
	LOGGER_MAX_SIZE                = "LOGGER_MAX_SIZE"                //nolint:staticcheck
	LOGGER_MAX_BACKUPS             = "LOGGER_MAX_BACKUPS"             //nolint:staticcheck
	LOGGER_HTTP_BODY_ENABLED       = "LOGGER_HTTP_BODY_ENABLED"       //nolint:staticcheck
	LOGGER_HTTP_BODY_LOG_REQUEST   = "LOGGER_HTTP_BODY_LOG_REQUEST"   //nolint:staticcheck
	LOGGER_HTTP_BODY_LOG_RESPONSE  = "LOGGER_HTTP_BODY_LOG_RESPONSE"  //nolint:staticcheck
	LOGGER_HTTP_BODY_MAX_BODY_SIZE = "LOGGER_HTTP_BODY_MAX_BODY_SIZE" //nolint:staticcheck
)

// Logger represents section "logger" for client-side or server-side configuration,
// and there is only one copy during the application entire lifetime.
type Logger struct {
	// Dir specifies which direcotory log to.
	Dir string `json:"dir" ini:"dir" yaml:"dir" mapstructure:"dir"`

	// Prefix specifies the log prefix.
	// You can set the prefix name to your project name.
	Prefix string `json:"prefix" ini:"prefix" yaml:"prefix" mapstructure:"prefix"`

	// File specifies the which file log to.
	// If value is "/dev/stdout", log to os.Stdout.
	// If value is "/dev/stderr", log to os.Stderr.
	// If value is empty(length is zero), log to os.Stdout.
	File string `json:"file" ini:"file" yaml:"file" mapstructure:"file"`

	// Console additionally mirrors the global logger's output to os.Stdout
	// when File is set to a real file path. It has no effect when File is
	// empty or one of "/dev/stdout"/"/dev/stderr", since those already log
	// to console. Only the global logger built by Init() reads this field;
	// subsystem loggers stay file-only unless the caller opts in explicitly
	// via zap.Option.Console.
	// Default: true
	Console bool `json:"console" ini:"console" yaml:"console" mapstructure:"console"`

	// Level specifies the log level,  supported values are: (error|warn|warning|info|debug).
	// The value default to "info" and ignore case.
	Level string `json:"level" ini:"level" yaml:"level" mapstructure:"level"`

	// Format specifies the log format, supported values are: (json|text).
	// The Value default to "text" and ignore case.
	Format string `json:"format" ini:"format" yaml:"format" mapstructure:"format"`

	// Encoder is the same as LogFormat.
	Encoder string `json:"encoder" ini:"encoder" yaml:"encoder" mapstructure:"encoder"`

	// ErrorStackDisabled disables attaching the error_stack field to
	// error-level logs when a logged error carries a stack trace embedded at
	// its creation point. Stack traces are attached by default; set this to
	// true in environments where the extra payload is unwanted, such as local
	// development.
	// Default: false
	ErrorStackDisabled bool `json:"error_stack_disabled" ini:"error_stack_disabled" yaml:"error_stack_disabled" mapstructure:"error_stack_disabled"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.
	// uint is "day" and default to 7.
	MaxAge int `json:"max_age" ini:"max_age" yaml:"max_age" mapstructure:"max_age"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated, default to 1MB.
	MaxSize int `json:"max_size" ini:"max_size" yaml:"max_size" mapstructure:"max_size"`

	// MaxBackups is the maximum number of old log files to retain.
	// The value default to 3.
	MaxBackups int `json:"max_backups" ini:"max_backups" yaml:"max_backups" mapstructure:"max_backups"`

	// HTTPBody contains HTTP request and response body logging configurations.
	HTTPBody HTTPBodyLogger `json:"http_body" ini:"http_body" yaml:"http_body" mapstructure:"http_body"`
}

// HTTPBodyLogger represents HTTP body logging configuration.
type HTTPBodyLogger struct {
	// Enabled enables HTTP request and response body logging middleware.
	// Default: false
	Enabled bool `json:"enabled" ini:"enabled" yaml:"enabled" mapstructure:"enabled"`

	// LogRequest enables logging of JSON HTTP request bodies.
	// Default: false
	LogRequest bool `json:"log_request" ini:"log_request" yaml:"log_request" mapstructure:"log_request"`

	// LogResponse enables logging of JSON HTTP response bodies.
	// Default: false
	LogResponse bool `json:"log_response" ini:"log_response" yaml:"log_response" mapstructure:"log_response"`

	// MaxBodySize limits how much request or response body data can be logged.
	// Values are parsed with github.com/dustin/go-humanize, for example "64KB".
	// Default: 64KB
	MaxBodySize string `json:"max_body_size" ini:"max_body_size" yaml:"max_body_size" mapstructure:"max_body_size"`
}

func (*Logger) setDefault() {
	cv.SetDefault("logger.dir", "./logs")
	cv.SetDefault("logger.prefix", "")
	cv.SetDefault("logger.file", "")
	cv.SetDefault("logger.console", true)
	cv.SetDefault("logger.level", "info")
	cv.SetDefault("logger.format", "json")
	cv.SetDefault("logger.encoder", "json")
	cv.SetDefault("logger.error_stack_disabled", false)
	cv.SetDefault("logger.max_age", 30)
	cv.SetDefault("logger.max_size", 100)
	cv.SetDefault("logger.max_backups", 1)
	cv.SetDefault("logger.http_body.enabled", false)
	cv.SetDefault("logger.http_body.log_request", false)
	cv.SetDefault("logger.http_body.log_response", false)
	cv.SetDefault("logger.http_body.max_body_size", "64KB")
}
