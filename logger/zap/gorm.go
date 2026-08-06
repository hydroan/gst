package zap

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gorml "gorm.io/gorm/logger"
)

// GormLogger implements gorm logger.Interface
type GormLogger struct{ l types.Logger }

var _ gorml.Interface = (*GormLogger)(nil)

func (g *GormLogger) LogMode(gorml.LogLevel) gorml.Interface { return g }

// Info, Warn and Error carry gorm's own non-statement messages (migrator
// progress, callback registration problems). gorm passes a printf format
// string with its arguments, so they forward to the formatting loggers.
func (g *GormLogger) Info(_ context.Context, str string, args ...any)  { g.l.Infof(str, args...) }
func (g *GormLogger) Warn(_ context.Context, str string, args ...any)  { g.l.Warnf(str, args...) }
func (g *GormLogger) Error(_ context.Context, str string, args ...any) { g.l.Errorf(str, args...) }

// sqlCallerSkipPrefixes are the function-path prefixes of frames that sit
// between a SQL log entry and the code that issued the SQL: gorm itself and
// the framework packages wrapping statement execution. The dao package is
// included so its helpers surface their business caller instead of the
// helper body.
var sqlCallerSkipPrefixes = []string{
	"gorm.io/",
	"github.com/hydroan/gst/logger",
	"github.com/hydroan/gst/database",
	"github.com/hydroan/gst/dao",
}

// isFrameworkSQLFrame reports whether the function belongs to gorm or to a
// framework package that wraps SQL execution.
func isFrameworkSQLFrame(function string) bool {
	for _, prefix := range sqlCallerSkipPrefixes {
		if strings.HasPrefix(function, prefix) {
			return true
		}
	}
	return false
}

// callerOutside walks the current goroutine stack and returns the trimmed
// file:line of the first frame whose function the skip predicate rejects,
// reporting false when every visible frame is skipped.
//
// SQL logs cannot name the code that issued the SQL through zap's
// AddCallerSkip: the wrapper depth between that code and the log call varies
// per operation (reads call gorm directly, writes run inside the transaction
// boundary closure), and a fixed skip count is only ever right for one of
// those depths. Walking the stack against a package predicate names the
// right frame at every depth, the same way gorm's own FileWithLineNum does.
func callerOutside(skip func(function string) bool) (string, bool) {
	var pcs [64]uintptr
	// Skip runtime.Callers and callerOutside itself; the predicate drops the
	// remaining framework frames, so the start offset needs no fine-tuning.
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if frame.Function != "" && !skip(frame.Function) {
			caller := zapcore.EntryCaller{File: frame.File, Line: frame.Line, Defined: true}
			return caller.TrimmedPath(), true
		}
		if !more {
			return "", false
		}
	}
}

// sqlCaller resolves the business frame a statement log should point at.
func sqlCaller() (string, bool) {
	return callerOutside(isFrameworkSQLFrame)
}

// Trace logs one executed statement with the request identity, timing, the
// SQL text, and the business caller that issued it.
//
// gorm.ErrRecordNotFound stays at info level with a record_not_found marker:
// a read matching no row is a documented outcome of First/Take, and logging
// it as an error buries real failures under normal traffic. Statements over
// the configured threshold log as slow queries; real errors log at error
// level with the same field set.
func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	meta := requestctx.FromContext(ctx)
	username := meta.Username()
	userID := meta.UserID()
	traceID := meta.TraceID()
	// Fallback to OTEL span context trace ID when request metadata has no trace ID.
	if len(traceID) == 0 {
		spanCtx := trace.SpanFromContext(ctx).SpanContext()
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}
	}
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := make([]zap.Field, 0, 10)
	if caller, ok := sqlCaller(); ok {
		fields = append(fields, zap.String("caller", caller))
	}
	fields = append(
		fields,
		zap.String(consts.CTX_ROUTE, meta.Route()),
		zap.String(consts.CTX_USERNAME, username),
		zap.String(consts.CTX_USER_ID, userID),
		zap.String(consts.TRACE_ID, traceID),
		zap.String("sql", sql),
		util.LogDuration(elapsed),
		zap.Int64("rows", rows),
	)
	notFound := errors.Is(err, gorml.ErrRecordNotFound)
	if notFound {
		fields = append(fields, zap.Bool("record_not_found", true))
	}

	switch {
	case err != nil && !notFound:
		g.l.Errorz("sql failed", append(fields, zap.Error(err))...)
	case elapsed > config.App.Database.SlowQueryThreshold:
		g.l.Warnz("slow sql detected", append(fields, zap.String("threshold", config.App.Database.SlowQueryThreshold.String()))...)
	default:
		g.l.Infoz("sql executed", fields...)
	}
}
