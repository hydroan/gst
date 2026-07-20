package zap

import (
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/errorstack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// errorStackKey is the structured log field holding the stack trace embedded
// in a logged error at its creation point.
const errorStackKey = "error_stack"

// withErrorStack returns the underlying zap logger enriched with the
// error_stack field when args contain an error carrying an embedded stack
// trace. The first such error wins. It serves Error, Errorf and Errorw,
// whose loosely typed arguments (positional args, format args or key-value
// pairs) all surface errors as plain values. It returns the logger unchanged
// when the level is disabled, the error_stack field is disabled via
// logger.error_stack_disabled, or no error carries a stack trace, so
// non-error paths pay nothing.
func (l *Logger) withErrorStack(level zapcore.Level, args []any) *zap.Logger {
	if !l.zlog.Core().Enabled(level) || config.App.Logger.ErrorStackDisabled {
		return l.zlog
	}
	for _, arg := range args {
		err, ok := arg.(error)
		if !ok || err == nil {
			continue
		}
		if stackTrace := errorstack.Origin(err); stackTrace != "" {
			return l.zlog.With(zap.String(errorStackKey, stackTrace))
		}
	}
	return l.zlog
}

// withErrorStackFields is the zap.Field counterpart of withErrorStack,
// serving Errorz: it scans strongly typed fields for the first error field
// (such as zap.Error) whose error carries an embedded stack trace. The same
// level and logger.error_stack_disabled short-circuits apply.
func (l *Logger) withErrorStackFields(level zapcore.Level, fields []zap.Field) *zap.Logger {
	if !l.zlog.Core().Enabled(level) || config.App.Logger.ErrorStackDisabled {
		return l.zlog
	}
	for _, field := range fields {
		if field.Type != zapcore.ErrorType {
			continue
		}
		err, ok := field.Interface.(error)
		if !ok || err == nil {
			continue
		}
		if stackTrace := errorstack.Origin(err); stackTrace != "" {
			return l.zlog.With(zap.String(errorStackKey, stackTrace))
		}
	}
	return l.zlog
}
