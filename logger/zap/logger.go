package zap

import (
	"context"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger implements types.Logger interface.
type Logger struct {
	zlog *zap.Logger
}

var _ types.Logger = (*Logger)(nil)

func (l *Logger) Debug(args ...any) { l.zlog.Sugar().Debug(args...) }
func (l *Logger) Info(args ...any)  { l.zlog.Sugar().Info(args...) }
func (l *Logger) Warn(args ...any)  { l.zlog.Sugar().Warn(args...) }
func (l *Logger) Error(args ...any) {
	l.withErrorStack(zapcore.ErrorLevel, args).Sugar().Error(args...)
}
func (l *Logger) Fatal(args ...any) { l.zlog.Sugar().Fatal(args...) }

func (l *Logger) Debugf(format string, args ...any) { l.zlog.Sugar().Debugf(format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.zlog.Sugar().Infof(format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.zlog.Sugar().Warnf(format, args...) }
func (l *Logger) Errorf(format string, args ...any) {
	l.withErrorStack(zapcore.ErrorLevel, args).Sugar().Errorf(format, args...)
}
func (l *Logger) Fatalf(format string, args ...any) { l.zlog.Sugar().Fatalf(format, args...) }

func (l *Logger) Debugw(msg string, keysValues ...any) { l.zlog.Sugar().Debugw(msg, keysValues...) }
func (l *Logger) Infow(msg string, keysValues ...any)  { l.zlog.Sugar().Infow(msg, keysValues...) }
func (l *Logger) Warnw(msg string, keysValues ...any)  { l.zlog.Sugar().Warnw(msg, keysValues...) }
func (l *Logger) Errorw(msg string, keysValues ...any) {
	l.withErrorStack(zapcore.ErrorLevel, keysValues).Sugar().Errorw(msg, keysValues...)
}
func (l *Logger) Fatalw(msg string, keysValues ...any) { l.zlog.Sugar().Fatalw(msg, keysValues...) }

func (l *Logger) Debugz(msg string, fields ...zap.Field) { l.zlog.Debug(msg, fields...) }
func (l *Logger) Infoz(msg string, fields ...zap.Field)  { l.zlog.Info(msg, fields...) }
func (l *Logger) Warnz(msg string, fields ...zap.Field)  { l.zlog.Warn(msg, fields...) }
func (l *Logger) Errorz(msg string, fields ...zap.Field) {
	l.withErrorStackFields(zapcore.ErrorLevel, fields).Error(msg, fields...)
}
func (l *Logger) Fatalz(msg string, fields ...zap.Field) { l.zlog.Fatal(msg, fields...) }

func (l *Logger) ZapLogger() *zap.Logger { return l.zlog }

// With creates a new logger with additional string key-value pairs.
// Each pair of arguments must be a key(string) followed by its value(string).
// If an odd number of arguments is provided, an empty string will be appended as the last value.
//
// Example 1 - Multiple With calls:
//
//	logger.With("phase", "update").
//	      With("user", "admin").
//	      With("trace_id", "123")
//
// Example 2 - Single With call with multiple fields:
//
//	logger.With(
//	    "phase", "update",
//	    "user", "admin",
//	    "trace_id", "123",
//	)
//
// Returns the original logger if no fields are provided or if only an empty key is provided.
func (l *Logger) With(fields ...string) types.Logger {
	if len(fields) == 0 {
		return l
	}
	if len(fields) == 1 {
		if len(fields[0]) == 0 {
			return l
		}
	}
	if len(fields)%2 != 0 {
		fields = append(fields, "")
	}

	zapFields := make([]zap.Field, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		if len(fields[i]) == 0 {
			continue
		}
		zapFields = append(zapFields, zap.String(fields[i], fields[i+1]))
	}
	return &Logger{zlog: l.zlog.With(zapFields...)}
}

// withContextFields binds the fields derived from a context — the request
// metadata and the execution identity — to a derived logger. This runs for
// every context-scoped logger, so all fields go through one zap With call:
// each With call clones the logger core, and chaining several With calls here
// used to cost three clones per call. When adding metadata fields, extend this
// single call instead of chaining further With calls.
//
// Route params stay structured because their keys come from the registered
// routes and are therefore bounded; the query is logged as one raw string
// because its keys are not. See requestctx.Metadata.RawQuery.
//
// The cron job name is present only inside a round: a request's lines carry
// no empty field for a capability they do not use.
func (l *Logger) withContextFields(meta requestctx.Metadata, id execctx.Identity, phase consts.Phase) types.Logger {
	fields := make([]zap.Field, 0, 10)
	fields = append(fields,
		zap.String(consts.PHASE, string(phase)),
		zap.String(consts.CTX_ROUTE, meta.Route()),
		zap.String(consts.CTX_PATH, meta.Path()),
		zap.String(consts.CTX_METHOD, meta.Method()),
		zap.String(consts.CTX_USERNAME, meta.Username()),
		zap.String(consts.CTX_USER_ID, meta.UserID()),
		zap.String(consts.TRACE_ID, id.TraceID),
		zap.Object(consts.PARAMS, paramsObject(meta.Params())),
		zap.String(consts.QUERY, meta.RawQuery()),
	)
	if len(id.Cronjob) > 0 {
		fields = append(fields, zap.String(consts.CRONJOB, id.Cronjob))
	}
	return &Logger{zlog: l.zlog.With(fields...)}
}

// WithContext creates a new logger carrying the request metadata and the
// execution identity found on ctx.
func (l *Logger) WithContext(ctx context.Context, phase consts.Phase) types.Logger {
	if ctx == nil {
		return l.With(consts.PHASE, string(phase))
	}

	return l.withContextFields(requestctx.FromContext(ctx), execctx.FromContext(ctx), phase)
}

type paramsObject map[string]string

func (o paramsObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if o == nil {
		return nil
	}
	for k, v := range o {
		enc.AddString(k, v)
	}
	return nil
}
