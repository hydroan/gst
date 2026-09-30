package serviceregistry

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors/errbase"
)

const defaultErrorStatus = http.StatusInternalServerError

// FailureMsg is the message a failure carrying no Error is answered with on
// every transport, the server's own failure: 500 with it in the HTTP
// envelope (see response.Error), Internal with it over gRPC (see
// grpcserver.StatusError). What went wrong stays in the log.
const FailureMsg = "The server could not process the request."

var (
	_ error                      = (*Error)(nil)
	_ errbase.StackTraceProvider = (*Error)(nil)
)

// Error represents a service-layer error that can be converted to an API response.
//
// It is re-exported to application code through the public service package.
type Error struct {
	status int
	msg    string
	cause  error
	// violations are the fields a request the validator refused failed on,
	// set by NewInvalidFields alone and read through FieldViolations; the
	// gRPC answer lists them as its details, the HTTP answer carries them
	// joined in msg.
	violations []FieldViolation
	// stack holds the program counters captured at the construction site,
	// exposed through StackTrace so error-stack consumers such as
	// errors.GetReportableStackTrace can locate where the error was created.
	stack []uintptr
}

// FieldViolation is one field of a request the validator refused: the field
// by its JSON key path, address.city or items[1].name, and the client-safe
// description of what it failed, the shape of google.rpc.BadRequest's
// FieldViolation.
type FieldViolation struct {
	Field       string
	Description string
}

// NewInvalidFields creates the 400 of a request whose fields the validator
// refused: the message joins the description of each violation with a
// semicolon, the violations stay readable (see FieldViolations), and cause,
// the validator's error, is reported by Error for logs and available
// through Unwrap.
func NewInvalidFields(violations []FieldViolation, cause error) *Error {
	descriptions := make([]string, 0, len(violations))
	for _, v := range violations {
		descriptions = append(descriptions, v.Description)
	}
	err := newError(http.StatusBadRequest, strings.Join(descriptions, "; "), cause)
	err.violations = violations
	return err
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

// FieldViolations returns the fields the request err answers for failed on,
// which only an error NewInvalidFields created carries; nil for any other
// error, and for nil. It is a package function rather than a method so it
// stays out of the public alias of Error: only the framework's gRPC answer
// reads the fields, to list them as its details, and the type they come as
// is the framework's own.
func FieldViolations(err *Error) []FieldViolation {
	if err == nil {
		return nil
	}
	return err.violations
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
