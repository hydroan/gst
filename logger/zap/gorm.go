package zap

import (
	"context"
	"encoding/binary"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
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

// sqlCallerSkipPrefixes are the built-in function-path prefixes of frames
// that sit between a SQL log entry and the code that issued the SQL: gorm
// itself and the framework packages wrapping statement execution. The dao
// package is included so its helpers surface their business caller instead
// of the helper body. Projects extend the list through
// logger.sql_caller_skip_prefixes; these built-in entries always stay in
// force.
//
// The controller package is deliberately absent: it holds the statements it
// issues, and the only frame beneath it is the router's handler dispatch
// loop, the same line for every route and action. Skipping it would trade an
// exact statement site for one constant frame; the skip predicate test pins
// that decision.
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
		if hasPackagePrefix(function, prefix) {
			return true
		}
	}
	return false
}

// hasPackagePrefix reports whether function lives at or under the package
// path prefix. A prefix already ending in "/" or "." matches by plain string
// prefix; any other prefix must be followed by "." (a symbol of that package)
// or "/" (a subpackage), so "example.com/app/dao" does not swallow
// "example.com/app/daox". An empty prefix never matches: a stray empty
// configuration entry must not skip every frame and erase the caller field.
func hasPackagePrefix(function, prefix string) bool {
	if len(prefix) == 0 || !strings.HasPrefix(function, prefix) {
		return false
	}
	if len(function) == len(prefix) {
		return true
	}
	if last := prefix[len(prefix)-1]; last == '/' || last == '.' {
		return true
	}
	next := function[len(prefix)]
	return next == '.' || next == '/'
}

// isSkippedSQLFrame reports whether a frame must not be named as the business
// caller of a statement log: it belongs to gorm, to a framework package
// wrapping SQL execution, or to a project-configured helper layer
// (logger.sql_caller_skip_prefixes). Built-in prefixes always apply; the
// configuration can only add more. Configured entries are trimmed because a
// comma-separated environment value keeps the whitespace around each comma
// after splitting.
func isSkippedSQLFrame(function string) bool {
	if isFrameworkSQLFrame(function) {
		return true
	}
	for _, prefix := range config.App.Logger.SQLCallerSkipPrefixes {
		if hasPackagePrefix(function, strings.TrimSpace(prefix)) {
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
	buffer, ok := sqlCallerBuffers.Get().(*sqlCallerBuffer)
	if !ok {
		buffer = new(sqlCallerBuffer)
	}
	defer sqlCallerBuffers.Put(buffer)

	// Skip runtime.Callers and callerOutside itself; the predicate drops the
	// remaining framework frames, so the start offset needs no fine-tuning.
	n := runtime.Callers(2, buffer.pcs[:])
	frames := sqlCallerFrames(buffer, n)
	// The predicate deliberately runs on every call rather than being baked
	// into the cache: an operator changing the configured skip prefixes keeps
	// moving the answer, the property the prefix test pins.
	for i := range frames {
		if frames[i].function != "" && !skip(frames[i].function) {
			return sqlCallerPath(frames[i].file, frames[i].line), true
		}
	}
	return "", false
}

// sqlCallerDepth bounds the stack walk that names a statement's caller.
const sqlCallerDepth = 64

// sqlCallerBuffer carries the scratch state of one caller resolution: the
// program-counter capture and the byte rendering of it that keys the stack
// cache.
type sqlCallerBuffer struct {
	pcs [sqlCallerDepth]uintptr
	key [sqlCallerDepth * 8]byte
}

// sqlCallerBuffers pools the capture buffers callerOutside works with.
// Without the pool every logged statement allocates one, and statement
// logging is on for every statement in every environment.
var sqlCallerBuffers = sync.Pool{
	New: func() any { return new(sqlCallerBuffer) },
}

// sqlCallerFrame is one resolved frame of a statement-issuing stack.
type sqlCallerFrame struct {
	function string
	file     string
	line     int
}

// sqlCallerStacks caches the resolved frames of each distinct
// statement-issuing stack, keyed by the raw program-counter bytes.
// runtime.CallersFrames re-runs the pc-to-source table walk on every
// statement, while that mapping is fixed for the life of the binary, so each
// distinct stack is resolved once. The key set cannot grow with traffic: it
// is bounded by the program's own statement-issuing call paths. Lookups go
// through a plain map under RWMutex because the compiler elides the
// byte-to-string conversion of a map index, which a sync.Map load cannot do.
var (
	sqlCallerStacksMu sync.RWMutex
	sqlCallerStacks   = make(map[string][]sqlCallerFrame)
)

// sqlCallerFrames returns the resolved frames of the buffer's captured stack,
// resolving each distinct stack only once.
func sqlCallerFrames(buffer *sqlCallerBuffer, n int) []sqlCallerFrame {
	for i := range n {
		binary.LittleEndian.PutUint64(buffer.key[i*8:], uint64(buffer.pcs[i]))
	}
	key := buffer.key[:n*8]

	sqlCallerStacksMu.RLock()
	frames, cached := sqlCallerStacks[string(key)]
	sqlCallerStacksMu.RUnlock()
	if cached {
		return frames
	}

	// The resolving slice is a copy: CallersFrames retains what it is handed,
	// and the pooled capture buffer goes back into circulation on return.
	resolved := make([]sqlCallerFrame, 0, n)
	iterator := runtime.CallersFrames(append([]uintptr(nil), buffer.pcs[:n]...))
	for {
		frame, more := iterator.Next()
		resolved = append(resolved, sqlCallerFrame{function: frame.Function, file: frame.File, line: frame.Line})
		if !more {
			break
		}
	}
	sqlCallerStacksMu.Lock()
	sqlCallerStacks[string(key)] = resolved
	sqlCallerStacksMu.Unlock()
	return resolved
}

// sqlCallerSite is one source position that issued a statement.
type sqlCallerSite struct {
	file string
	line int
}

// sqlCallerPaths caches the trimmed path of each site that issues statements.
//
// The trimmed path is pure formatting of a position that repeats on every
// statement the same code path runs, so it is computed once per site instead of
// once per statement. The cache cannot grow with traffic: its keys come from
// the program's own call sites, which are fixed at build time.
var sqlCallerPaths sync.Map

// sqlCallerPath returns the trimmed file:line of one caller, formatting it only
// the first time that site is seen.
func sqlCallerPath(file string, line int) string {
	site := sqlCallerSite{file: file, line: line}
	if cached, found := sqlCallerPaths.Load(site); found {
		if path, ok := cached.(string); ok {
			return path
		}
	}

	path := zapcore.EntryCaller{File: file, Line: line, Defined: true}.TrimmedPath()
	sqlCallerPaths.Store(site, path)
	return path
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

	// Sized to the exact worst case so the hot path never regrows: caller,
	// the eight base fields, db_role, record_not_found, and the one field the
	// error/slow branches append (mutually exclusive in the switch below).
	fields := make([]zap.Field, 0, 12)
	if caller, ok := callerOutside(isSkippedSQLFrame); ok {
		fields = append(fields, zap.String("caller", caller))
	}
	fields = append(
		fields,
		zap.String(consts.CTX_ROUTE, meta.Route()),
		zap.String(consts.CTX_METHOD, meta.Method()),
		zap.String(consts.CTX_USERNAME, username),
		zap.String(consts.CTX_USER_ID, userID),
		zap.String(consts.TRACE_ID, traceID),
		zap.String("sql", sql),
		util.LogDuration(elapsed),
		zap.Int64("rows", rows),
	)
	// Present only on handles with read replicas attached, where "which node
	// served this query" stops being answerable by assumption.
	if role := dbruntime.RoleFromContext(ctx); len(role) > 0 {
		fields = append(fields, zap.String("db_role", role))
	}
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
