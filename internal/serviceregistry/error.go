package serviceregistry

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors/errbase"
	"github.com/hydroan/gst/types"
)

const (
	defaultErrorCode   = -1
	defaultErrorStatus = http.StatusInternalServerError
)

var (
	_ error                      = (*Error)(nil)
	_ types.Coder                = (*Error)(nil)
	_ errbase.StackTraceProvider = (*Error)(nil)
)

// Error represents a service-layer error that can be converted to an API response.
//
// It is re-exported to application code through the public service package.
type Error struct {
	status int
	msg    string
	cause  error
	// stack holds the program counters captured at the construction site,
	// exposed through StackTrace so error-stack consumers such as
	// errors.GetReportableStackTrace can locate where the error was created.
	stack []uintptr
}

// NewError creates a service-layer error with a client-safe message.
//
// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
// http.StatusInternalServerError and the provided message is discarded.
func NewError(status int, msg string) *Error {
	return newError(status, msg, nil)
}

// NewErrorWithCause creates a service-layer error with an internal cause.
//
// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
// http.StatusInternalServerError and the provided message is discarded.
//
// The cause is reported by Error for logs and available through Unwrap, but
// is never exposed as the response message.
func NewErrorWithCause(status int, msg string, cause error) *Error {
	return newError(status, msg, cause)
}

func newError(status int, msg string, cause error) *Error {
	normalizedStatus, validStatus := normalizeErrorStatus(status)
	if !validStatus {
		msg = ""
	}

	return &Error{
		status: normalizedStatus,
		msg:    normalizeErrorMessage(normalizedStatus, msg),
		cause:  cause,
		stack:  callers(),
	}
}

// callers captures the current call stack, skipping runtime.Callers, callers
// itself, newError and its exported constructor wrapper, so the innermost
// recorded frame is the construction site in application code.
func callers() []uintptr {
	const maxDepth = 32
	var pcs [maxDepth]uintptr
	n := runtime.Callers(4, pcs[:])
	return pcs[:n]
}

// StackTrace implements errbase.StackTraceProvider, exposing the stack
// captured at the construction site in the github.com/pkg/errors format that
// errors.GetReportableStackTrace recognizes. When the error also wraps a
// cause carrying its own stack trace, consumers that pick the deepest stack
// in the unwrap chain keep preferring the cause's origin.
func (e *Error) StackTrace() errbase.StackTrace {
	if e == nil || len(e.stack) == 0 {
		return nil
	}
	frames := make(errbase.StackTrace, len(e.stack))
	for i, pc := range e.stack {
		frames[i] = errbase.StackFrame(pc)
	}
	return frames
}

// Error reports the client-safe message followed by the cause chain, so log
// consumers rendering err.Error() (zap's error field, sugared positional
// logging) capture the internal cause. The response envelope renders Msg
// instead, keeping the cause out of API responses.
func (e *Error) Error() string {
	if e == nil || e.cause == nil {
		return e.Msg()
	}
	return e.msg + ": " + e.cause.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Code() int {
	return defaultErrorCode
}

func (e *Error) Status() int {
	if e == nil {
		return defaultErrorStatus
	}
	return e.status
}

func (e *Error) Msg() string {
	if e == nil {
		return http.StatusText(defaultErrorStatus)
	}
	return e.msg
}

func normalizeErrorStatus(status int) (int, bool) {
	if status >= http.StatusBadRequest && status <= 599 {
		return status, true
	}
	return defaultErrorStatus, false
}

func normalizeErrorMessage(status int, msg string) string {
	msg = strings.TrimSpace(msg)
	if msg != "" {
		return msg
	}

	if text := http.StatusText(status); text != "" {
		return text
	}
	return http.StatusText(defaultErrorStatus)
}
