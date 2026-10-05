package types

import (
	"context"

	"github.com/hydroan/gst/internal/consts"
	"go.uber.org/zap"
)

// Logger is the logger the framework hands to services and modules and keeps
// in the logger package's streams. It writes an entry three ways: plain and
// printf-style, sugared with alternating key/value fields (the methods with
// suffix "w", with fields), and with typed zap.Field values (the methods with
// suffix "z", the low-allocation variants). With attaches string key/value
// fields; WithContext derives a logger carrying request metadata fields.
// Fatal and its variants follow the underlying logger's fatal behavior and
// terminate the process after writing the entry.
type Logger interface {
	With(fields ...string) Logger

	WithContext(context.Context, consts.Phase) Logger

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

	Debugw(msg string, keysAndValues ...any)
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
	Fatalw(msg string, keysAndValues ...any)

	Debugz(msg string, fields ...zap.Field)
	Infoz(msg string, fields ...zap.Field)
	Warnz(msg string, fields ...zap.Field)
	Errorz(msg string, fields ...zap.Field)
	Fatalz(msg string, fields ...zap.Field)
}
