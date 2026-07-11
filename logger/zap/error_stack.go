package zap

import (
	"github.com/hydroan/gst/internal/errorstack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// errorStackKey is the structured log field holding the stack trace embedded
// in a logged error at its creation point.
const errorStackKey = "error_stack"

// withErrorStack returns the underlying zap logger enriched with the
// error_stack field when args contain an error carrying an embedded stack
// trace. The first such error wins. It returns the logger unchanged when
// the level is disabled or no error carries a stack trace, so non-error
// paths pay nothing.
func (l *Logger) withErrorStack(level zapcore.Level, args []any) *zap.Logger {
	if !l.zlog.Core().Enabled(level) {
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
