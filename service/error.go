package service

import "github.com/hydroan/gst/internal/serviceregistry"

// Error represents a service-layer error that can be converted to an API response.
type Error = serviceregistry.Error

// NewError creates a service-layer error with a client-safe message.
//
// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
// http.StatusInternalServerError and the provided message is discarded.
func NewError(status int, msg string) *Error {
	return serviceregistry.NewError(status, msg)
}

// NewErrorWithCause creates a service-layer error with an internal cause.
//
// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
// http.StatusInternalServerError and the provided message is discarded.
//
// The cause is available through Unwrap but is never exposed as the response message.
func NewErrorWithCause(status int, msg string, cause error) *Error {
	return serviceregistry.NewErrorWithCause(status, msg, cause)
}
