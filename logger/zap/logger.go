package zap

import (
	"context"
	"strings"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.opentelemetry.io/otel/trace"
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
func (l *Logger) Error(args ...any) { l.zlog.Sugar().Error(args...) }
func (l *Logger) Fatal(args ...any) { l.zlog.Sugar().Fatal(args...) }

func (l *Logger) Debugf(format string, args ...any) { l.zlog.Sugar().Debugf(format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.zlog.Sugar().Infof(format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.zlog.Sugar().Warnf(format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.zlog.Sugar().Errorf(format, args...) }
func (l *Logger) Fatalf(format string, args ...any) { l.zlog.Sugar().Fatalf(format, args...) }

func (l *Logger) Debugw(msg string, keysValues ...any) { l.zlog.Sugar().Debugw(msg, keysValues...) }
func (l *Logger) Infow(msg string, keysValues ...any)  { l.zlog.Sugar().Infow(msg, keysValues...) }
func (l *Logger) Warnw(msg string, keysValues ...any)  { l.zlog.Sugar().Warnw(msg, keysValues...) }
func (l *Logger) Errorw(msg string, keysValues ...any) { l.zlog.Sugar().Errorw(msg, keysValues...) }
func (l *Logger) Fatalw(msg string, keysValues ...any) { l.zlog.Sugar().Fatalw(msg, keysValues...) }

func (l *Logger) Debugz(msg string, fields ...zap.Field) { l.zlog.Debug(msg, fields...) }
func (l *Logger) Infoz(msg string, fields ...zap.Field)  { l.zlog.Info(msg, fields...) }
func (l *Logger) Warnz(msg string, fields ...zap.Field)  { l.zlog.Warn(msg, fields...) }
func (l *Logger) Errorz(msg string, fields ...zap.Field) { l.zlog.Error(msg, fields...) }
func (l *Logger) Fatalz(msg string, fields ...zap.Field) { l.zlog.Fatal(msg, fields...) }

func (l *Logger) ZapLogger() *zap.Logger { return l.zlog }

func (l *Logger) WithObject(name string, obj zapcore.ObjectMarshaler) types.Logger {
	return &Logger{zlog: l.zlog.With(zap.Object(name, obj))}
}

func (l *Logger) WithArray(name string, arr zapcore.ArrayMarshaler) types.Logger {
	return &Logger{zlog: l.zlog.With(zap.Array(name, arr))}
}

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

func (l *Logger) withRequestMetadata(meta types.RequestMetadata, phase consts.Phase, traceID string) types.Logger {
	return l.With(
		consts.PHASE, string(phase),
		consts.CTX_ROUTE, meta.Route(),
		consts.CTX_USERNAME, meta.Username(),
		consts.CTX_USER_ID, meta.UserID(),
		consts.TRACE_ID, traceID,
	).
		WithObject(consts.PARAMS, paramsObject(meta.Params())).
		WithObject(consts.QUERY, queryObject(meta.Query()))
}

// WithContext creates a new logger with request metadata fields from ctx.
func (l *Logger) WithContext(ctx context.Context, phase consts.Phase) types.Logger {
	if ctx == nil {
		return l.With(consts.PHASE, string(phase))
	}

	meta := types.RequestMetadataFromContext(ctx)
	traceID := meta.TraceID()
	if len(traceID) == 0 {
		spanCtx := trace.SpanFromContext(ctx).SpanContext()
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}
	}

	return l.withRequestMetadata(meta, phase, traceID)
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

type queryObject map[string][]string

func (o queryObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if o == nil {
		return nil
	}
	for k, v := range o {
		enc.AddString(k, strings.Join(v, ","))
	}
	return nil
}
