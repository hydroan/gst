package types

import (
	"context"

	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// StandardLogger provides plain and printf-style leveled logging methods.
// Fatal and Fatalf follow the underlying logger's fatal behavior and should
// terminate the process after writing the log entry.
type StandardLogger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Fatal(args ...any)

	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// StructuredLogger provides sugared structured logging with alternating
// key/value fields. Methods with suffix "w" mean "with fields".
type StructuredLogger interface {
	Debugw(msg string, keysAndValues ...any)
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
	Fatalw(msg string, keysAndValues ...any)
}

// ZapLogger provides structured logging with typed zap.Field values.
// Methods with suffix "z" are the low-allocation typed-field variants.
type ZapLogger interface {
	Debugz(msg string, fields ...zap.Field)
	Infoz(msg string, fields ...zap.Field)
	Warnz(msg string, fields ...zap.Field)
	Errorz(msg string, fields ...zap.Field)
	Fatalz(msg string, fields ...zap.Field)
}

// Logger combines plain, sugared structured, and typed zap logging methods.
// With attaches string key/value fields. WithObject, WithArray, and the context
// helpers return derived loggers with additional structured fields.
type Logger interface {
	With(fields ...string) Logger

	WithObject(name string, obj zapcore.ObjectMarshaler) Logger
	WithArray(name string, arr zapcore.ArrayMarshaler) Logger

	WithContext(context.Context, consts.Phase) Logger

	StandardLogger
	StructuredLogger
	ZapLogger
}
