package service

import (
	"net/http"
	"strings"

	"github.com/hydroan/gst/types"
)

const (
	defaultErrorCode   = -1
	defaultErrorStatus = http.StatusInternalServerError
)

var (
	_ error       = (*Error)(nil)
	_ types.Coder = (*Error)(nil)
)

// Error represents a service-layer error that can be converted to an API response.
type Error struct {
	status int
	msg    string
	cause  error
}

// NewError creates a service-layer error with a client-safe message.
func NewError(status int, msg string) *Error {
	return newError(status, msg, nil)
}

// NewErrorWithCause creates a service-layer error with an internal cause.
//
// The cause is available through Unwrap but is never exposed as the response message.
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
	}
}

func (e *Error) Error() string {
	return e.Msg()
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
