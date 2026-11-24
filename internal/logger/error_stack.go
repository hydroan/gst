package logger

import (
	"slices"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/errorstack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// keyErrorStack is the structured log field holding the stack trace embedded
// in a logged error at its creation point.
const keyErrorStack = "error_stack"

// withErrorStack returns the underlying zap logger enriched with the
// error_stack field when args contain an error carrying an embedded stack
// trace. The first such error wins. It serves Error and Errorf, whose
// positional and format arguments surface errors as plain values the entry's
// message absorbs; the errors Errorw and Errorz carry reach the core as
// fields, and errorFieldCore handles those. It returns the logger unchanged
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
			return l.zlog.With(zap.String(keyErrorStack, stackTrace))
		}
	}
	return l.zlog
}

// errorFieldCore is the core every logger here writes through, the one
// newLogCore builds: it writes an error field as the error's text alone, with
// the error_stack field errorstack derives for the error, where zapcore on
// its own writes the text and a <key>Verbose field holding the error's %+v —
// for a cockroachdb error the stack of every wrap of the chain, the init
// stack of a package-level sentinel included — that no option of zapcore
// turns off. error_stack is written once per entry, from the first error
// field carrying a stack, context fields counted first;
// logger.error_stack_disabled leaves it out, as it does for the positional
// errors of Error and Errorf. It is held through a pointer: zapcore keeps
// the core of every checked entry as an interface value, and a struct there
// would be copied onto the heap once per entry.
type errorFieldCore struct {
	zapcore.Core
	// stacked records that the context fields carry error_stack already, so
	// an entry's own error field does not add a second one.
	stacked bool
}

// With rewrites the context's error fields; an error_stack among them, or
// derived from them, is every entry's.
func (c *errorFieldCore) With(fields []zapcore.Field) zapcore.Core {
	fields, stacked := c.errorFields(fields)
	return &errorFieldCore{Core: c.Core.With(fields), stacked: stacked}
}

// Check adds this core, not the wrapped one, so the entry is written through
// Write below.
func (c *errorFieldCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write rewrites the entry's error fields and writes it through the wrapped
// core.
func (c *errorFieldCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	fields, _ = c.errorFields(fields)
	return c.Core.Write(ent, fields)
}

// errorFields returns fields with every error field rewritten to its text,
// and error_stack appended for the first error carrying a stack unless the
// context has error_stack already, fields has it, or
// logger.error_stack_disabled is set; it reports whether the result carries
// error_stack. fields is returned as it is when it has no error field, so
// entries without one pay nothing.
func (c *errorFieldCore) errorFields(fields []zapcore.Field) ([]zapcore.Field, bool) {
	stacked := c.stacked || slices.ContainsFunc(fields, func(f zapcore.Field) bool { return f.Key == keyErrorStack })
	var out []zapcore.Field
	var stack string
	for i, f := range fields {
		err, ok := f.Interface.(error)
		if f.Type != zapcore.ErrorType || !ok || err == nil {
			continue
		}
		if out == nil {
			out = slices.Clone(fields)
		}
		if stack == "" && !stacked && !config.App.Logger.ErrorStackDisabled {
			stack = errorstack.Origin(err)
		}
		out[i].Interface = unformatted(err)
	}
	if out == nil {
		return fields, stacked
	}
	if stack != "" {
		out = append(out, zap.String(keyErrorStack, stack))
		stacked = true
	}
	return out, stacked
}

// unformatted returns err as an error without a Format method, so zapcore
// writes its text alone; the causes of an error group — the errors
// go.uber.org/multierr combines, which zapcore lists under <key>Causes — are
// returned the same way.
func unformatted(err error) error {
	group, ok := err.(interface{ Errors() []error })
	if !ok {
		return unformattedError{err}
	}
	causes := group.Errors()
	stripped := make([]error, len(causes))
	for i, cause := range causes {
		stripped[i] = unformatted(cause)
	}
	return unformattedGroupError{unformattedError{err}, stripped}
}

// unformattedError carries an error's Error method alone: the error it embeds
// is held as the error interface, whose method set has no Format.
type unformattedError struct{ error }

// unformattedGroupError is an unformattedError that is an error group as
// well, listing its causes.
type unformattedGroupError struct {
	unformattedError
	causes []error
}

// Errors returns the group's causes.
func (g unformattedGroupError) Errors() []error { return g.causes }
